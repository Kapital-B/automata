package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type SummarizeService struct {
	Messages  driven.MessageRepository
	Summaries driven.SummaryRepository
	LLM       driven.LLMClient
	JobRuns   driven.JobRunRepository
}

type SummarizeOptions struct {
	RunID     *uuid.UUID
	Trigger   string
	Since     *time.Time
	AccountID *uuid.UUID
}

type SummarizeResult struct {
	JobRunID    uuid.UUID
	ActionItems int
	FYIItems    int
}

type summarizePayload struct {
	GeneralSummary string `json:"general_summary"`
	ActionItems    []struct {
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
		DueAt     string `json:"due_at,omitempty"`
	} `json:"action_items"`
	FYI []struct {
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
	} `json:"fyi"`
}

const (
	defaultSummarizeChunkSize = 12
	minSummarizeChunkSize     = 3
	maxSummarizeChunkSize     = 30
	maxCombinedSummaryLen     = 2400
	maxSubjectCharsDefault    = 120
	maxFromCharsDefault       = 120
	maxBodyCharsDefault       = 320
	maxSubjectCharsTight      = 80
	maxFromCharsTight         = 80
	maxBodyCharsTight         = 120
)

func (s *SummarizeService) SummarizeAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, opts SummarizeOptions) (*SummarizeResult, error) {
	if s == nil || s.Messages == nil || s.Summaries == nil || s.LLM == nil || s.JobRuns == nil {
		return nil, fmt.Errorf("summarize service not configured")
	}
	runID := uuid.New()
	if opts.RunID != nil {
		runID = *opts.RunID
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	started := time.Now().UTC()
	_ = s.JobRuns.InsertJobRun(ctx, runID, accountID, "summarize", trigger, "running", started, time.Time{}, nil, `{}`)

	fail := func(err error) (*SummarizeResult, error) {
		msg := err.Error()
		_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", timePtrSummary(time.Now().UTC()), &msg, `{}`)
		return nil, err
	}

	filter := driven.MessageListFilter{AccountID: &accountID, Limit: 200}
	if opts.Since != nil {
		filter.Since = opts.Since
	}
	msgs, err := s.Messages.ListMessages(ctx, userID, filter)
	if err != nil {
		return fail(err)
	}
	settings, err := s.Summaries.GetSummarySettings(ctx, userID)
	if err != nil {
		return fail(err)
	}
	filtered := applySummarySettings(msgs, settings)
	chainMeta := summarizeChainMetaJSON(opts.Since)
	_ = s.JobRuns.UpdateJobRunMeta(ctx, runID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":0%s}`, len(filtered), chainMeta))

	chunkSize := defaultSummarizeChunkSize
	if settings != nil {
		chunkSize = normalizeChunkSize(settings.ChunkSize)
	}
	p, err := s.summarizeMessagesInChunks(ctx, filtered, runID, chunkSize)
	if err != nil {
		return fail(err)
	}

	now := time.Now().UTC()
	snapshot := driven.SummarySnapshotRow{
		ID:             uuid.New(),
		UserID:         userID,
		AccountID:      &accountID,
		RunID:          runID,
		WindowStart:    started.Add(-24 * time.Hour),
		WindowEnd:      started,
		GeneralSummary: p.GeneralSummary,
		CreatedAt:      now,
	}
	if err := s.Summaries.InsertSummarySnapshot(ctx, snapshot); err != nil {
		return fail(err)
	}
	openItems, err := s.Summaries.ListOpenActionItems(ctx, userID, &accountID)
	if err != nil {
		return fail(err)
	}
	openByMessage := make(map[string]bool, len(openItems))
	for _, it := range openItems {
		openByMessage[it.MessageID.String()] = true
	}
	toInsert := make([]driven.ActionItemRow, 0, len(p.ActionItems))
	for _, a := range p.ActionItems {
		mid, err := uuid.Parse(strings.TrimSpace(a.MessageID))
		if err != nil || openByMessage[mid.String()] {
			continue
		}
		var dueAt *time.Time
		if strings.TrimSpace(a.DueAt) != "" {
			if t, err := time.Parse(time.RFC3339, a.DueAt); err == nil {
				ut := t.UTC()
				dueAt = &ut
			}
		}
		toInsert = append(toInsert, driven.ActionItemRow{
			ID:        uuid.New(),
			UserID:    userID,
			AccountID: accountID,
			MessageID: mid,
			RunID:     runID,
			Text:      strings.TrimSpace(a.Text),
			DueAt:     dueAt,
			Status:    "open",
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	if len(toInsert) > 0 {
		if err := s.Summaries.InsertActionItems(ctx, toInsert); err != nil {
			return fail(err)
		}
	}
	fyiRows := make([]driven.FYIRow, 0, len(p.FYI))
	for _, f := range p.FYI {
		mid, err := uuid.Parse(strings.TrimSpace(f.MessageID))
		if err != nil {
			continue
		}
		fyiRows = append(fyiRows, driven.FYIRow{
			ID:        uuid.New(),
			UserID:    userID,
			AccountID: accountID,
			MessageID: mid,
			RunID:     runID,
			Text:      strings.TrimSpace(f.Text),
			CreatedAt: now,
		})
	}
	if len(fyiRows) > 0 {
		if err := s.Summaries.InsertFYI(ctx, fyiRows); err != nil {
			return fail(err)
		}
	}
	meta := fmt.Sprintf(`{"total_messages":%d,"processed_messages":%d,"action_items":%d,"fyi_items":%d%s}`, len(filtered), len(filtered), len(toInsert), len(fyiRows), chainMeta)
	_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "success", timePtrSummary(time.Now().UTC()), nil, meta)
	return &SummarizeResult{JobRunID: runID, ActionItems: len(toInsert), FYIItems: len(fyiRows)}, nil
}

func (s *SummarizeService) summarizeMessages(ctx context.Context, msgs []driven.MessageRow) (*summarizePayload, error) {
	out, err := s.summarizeMessagesWithClamp(ctx, msgs, maxSubjectCharsDefault, maxFromCharsDefault, maxBodyCharsDefault)
	if err == nil {
		return out, nil
	}
	if !isContextExceededError(err) {
		return nil, err
	}
	// Retry with tighter prompt budget for small-context local models.
	out, err = s.summarizeMessagesWithClamp(ctx, msgs, maxSubjectCharsTight, maxFromCharsTight, maxBodyCharsTight)
	if err == nil {
		return out, nil
	}
	if !isContextExceededError(err) {
		return nil, err
	}
	// Last-resort retry with zero body text.
	return s.summarizeMessagesWithClamp(ctx, msgs, maxSubjectCharsTight, maxFromCharsTight, 0)
}

func (s *SummarizeService) summarizeMessagesWithClamp(
	ctx context.Context,
	msgs []driven.MessageRow,
	subjectLimit int,
	fromLimit int,
	bodyLimit int,
) (*summarizePayload, error) {
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		body := ""
		if bodyLimit > 0 {
			body = clampText(derefStr(m.BodyText), bodyLimit)
		}
		lines = append(lines, fmt.Sprintf("- id=%s | subj=%s | from=%s | body=%s",
			m.ID.String(), clampText(m.Subject, subjectLimit), clampText(m.FromJSON, fromLimit), body))
	}
	prompt := "Return JSON only: {\"general_summary\":\"...\",\"action_items\":[{\"message_id\":\"uuid\",\"text\":\"...\",\"due_at\":\"RFC3339 optional\"}],\"fyi\":[{\"message_id\":\"uuid\",\"text\":\"...\"}]}. " +
		"Use only listed message ids.\n" + strings.Join(lines, "\n")
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "Email summarizer. JSON output only."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}
	var out summarizePayload
	if err := json.Unmarshal([]byte(normalizeJSONContent(resp.Content)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func isContextExceededError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context size") ||
		strings.Contains(msg, "context length") ||
		strings.Contains(msg, "context window") ||
		strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "n_tokens")
}

func (s *SummarizeService) summarizeMessagesInChunks(ctx context.Context, msgs []driven.MessageRow, runID uuid.UUID, chunkSize int) (*summarizePayload, error) {
	if len(msgs) == 0 {
		return &summarizePayload{
			GeneralSummary: "No matching emails in the selected window and category filters.",
			ActionItems:    nil,
			FYI:            nil,
		}, nil
	}
	combined := &summarizePayload{
		ActionItems: make([]struct {
			MessageID string `json:"message_id"`
			Text      string `json:"text"`
			DueAt     string `json:"due_at,omitempty"`
		}, 0),
		FYI: make([]struct {
			MessageID string `json:"message_id"`
			Text      string `json:"text"`
		}, 0),
	}
	chunkSummaries := make([]string, 0, (len(msgs)+chunkSize-1)/chunkSize)
	seenActionByMessage := make(map[string]struct{}, len(msgs))
	seenFYIByMessage := make(map[string]struct{}, len(msgs))
	processed := 0
	for i := 0; i < len(msgs); i += chunkSize {
		end := i + chunkSize
		if end > len(msgs) {
			end = len(msgs)
		}
		part, err := s.summarizeMessages(ctx, msgs[i:end])
		if err != nil {
			return nil, err
		}
		if text := strings.TrimSpace(part.GeneralSummary); text != "" {
			chunkSummaries = append(chunkSummaries, text)
		}
		for _, item := range part.ActionItems {
			key := strings.TrimSpace(item.MessageID)
			if key == "" {
				continue
			}
			if _, exists := seenActionByMessage[key]; exists {
				continue
			}
			seenActionByMessage[key] = struct{}{}
			combined.ActionItems = append(combined.ActionItems, item)
		}
		for _, item := range part.FYI {
			key := strings.TrimSpace(item.MessageID)
			if key == "" {
				continue
			}
			if _, exists := seenFYIByMessage[key]; exists {
				continue
			}
			seenFYIByMessage[key] = struct{}{}
			combined.FYI = append(combined.FYI, item)
		}
		processed = end
		_ = s.JobRuns.UpdateJobRunMeta(ctx, runID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":%d,"chunks_completed":%d,"chunk_size":%d}`, len(msgs), processed, (end+chunkSize-1)/chunkSize, chunkSize))
	}
	combined.GeneralSummary = combineChunkSummaries(chunkSummaries)
	return combined, nil
}

func normalizeChunkSize(v int) int {
	if v <= 0 {
		return defaultSummarizeChunkSize
	}
	if v < minSummarizeChunkSize {
		return minSummarizeChunkSize
	}
	if v > maxSummarizeChunkSize {
		return maxSummarizeChunkSize
	}
	return v
}

func combineChunkSummaries(chunkSummaries []string) string {
	if len(chunkSummaries) == 0 {
		return "No notable changes in the selected email set."
	}
	joined := strings.Join(chunkSummaries, " ")
	joined = strings.Join(strings.Fields(joined), " ")
	if len(joined) <= maxCombinedSummaryLen {
		return joined
	}
	return strings.TrimSpace(joined[:maxCombinedSummaryLen]) + "..."
}

func applySummarySettings(msgs []driven.MessageRow, settings *driven.SummarySettingsRow) []driven.MessageRow {
	if settings == nil {
		return msgs
	}
	include := make(map[string]struct{}, len(settings.IncludeCategorySlugs))
	for _, v := range settings.IncludeCategorySlugs {
		include[strings.TrimSpace(strings.ToLower(v))] = struct{}{}
	}
	exclude := make(map[string]struct{}, len(settings.ExcludeCategorySlugs))
	for _, v := range settings.ExcludeCategorySlugs {
		exclude[strings.TrimSpace(strings.ToLower(v))] = struct{}{}
	}
	out := make([]driven.MessageRow, 0, len(msgs))
	for _, m := range msgs {
		slug := ""
		if m.CategorySlug != nil {
			slug = strings.ToLower(strings.TrimSpace(*m.CategorySlug))
		}
		if len(include) > 0 {
			if _, ok := include[slug]; !ok {
				continue
			}
		}
		if _, ok := exclude[slug]; ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

func timePtrSummary(t time.Time) *time.Time {
	return &t
}

func summarizeChainMetaJSON(since *time.Time) string {
	if since == nil {
		return ""
	}
	return fmt.Sprintf(`,"chain_started_at":"%s"`, since.UTC().Format(time.RFC3339Nano))
}
