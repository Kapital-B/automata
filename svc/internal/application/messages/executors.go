package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/jobkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type SyncExecutor struct {
	Service *SyncService
}

func (e *SyncExecutor) JobType() string { return "sync" }

func (e *SyncExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.SyncChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: driven.JobProgress{
			Processed: res.MessagesUpserted,
			Detail: map[string]interface{}{
				"fetched":            res.Fetched,
				"messages_upserted":  res.MessagesUpserted,
				"delta_reused":       res.DeltaReused,
				"delta_reset_reason": res.DeltaResetReason,
			},
		},
		Done: res.Done,
	}, nil
}

type CategorizeExecutor struct {
	Service *CategorizeService
}

func (e *CategorizeExecutor) JobType() string { return "categorize" }

func (e *CategorizeExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.CategorizeChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		NextCursor: res.NextCursor,
		ProgressDelta: driven.JobProgress{
			Processed: res.MessagesScanned,
			Detail: map[string]interface{}{
				"messages_categorized": res.MessagesCategorized,
			},
		},
		Done: res.Done,
	}, nil
}

type DraftSuggestExecutor struct {
	Service *AutoDraftService
}

func (e *DraftSuggestExecutor) JobType() string { return "draft_suggest" }

func (e *DraftSuggestExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	res, err := e.Service.GenerateChunk(ctx, run)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		ProgressDelta: driven.JobProgress{
			Processed: res.ActionItemsSeen,
			Detail: map[string]interface{}{
				"drafts_generated":  res.DraftsGenerated,
				"action_items_seen": res.ActionItemsSeen,
			},
		},
		Done: res.Done,
	}, nil
}

type SummarizeExecutor struct {
	Service *SummarizeService
}

func (e *SummarizeExecutor) JobType() string { return "summarize" }

func (e *SummarizeExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	if e == nil || e.Service == nil || e.Service.Messages == nil || e.Service.Summaries == nil || e.Service.LLM == nil {
		return driven.ChunkResult{}, fmt.Errorf("summarize service not configured")
	}
	if run.AccountID == nil || *run.AccountID == uuid.Nil {
		return driven.ChunkResult{}, fmt.Errorf("account_id is required")
	}
	offset := jobkit.DecodeOffsetCursor(run.Cursor)
	rows, err := e.Service.Messages.ListMessages(ctx, run.UserID, driven.MessageListFilter{
		AccountID: run.AccountID,
		Limit:     241,
		Offset:    offset,
	})
	if err != nil {
		return driven.ChunkResult{}, err
	}
	done := len(rows) <= 240
	if len(rows) > 240 {
		rows = rows[:240]
	}
	candidates := make([]driven.MessageRow, 0, len(rows))
	for _, row := range rows {
		if row.SummarySeenAt == nil {
			candidates = append(candidates, row)
		}
	}
	settings, err := e.Service.Summaries.GetSummarySettings(ctx, run.UserID)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	filtered := applySummarySettings(candidates, settings)
	if len(filtered) == 0 {
		result := driven.ChunkResult{
			ProgressDelta: driven.JobProgress{Processed: len(rows)},
			Done:          done,
		}
		if !done {
			result.NextCursor = jobkit.EncodeOffsetCursor(offset + len(rows))
		}
		return result, nil
	}
	chunkSize := defaultSummarizeChunkSize
	if settings != nil {
		chunkSize = normalizeChunkSize(settings.ChunkSize)
	}
	partials := make([]summarizePayload, 0, (len(filtered)+chunkSize-1)/chunkSize)
	chunkSummaries := make([]string, 0, len(partials))
	for i := 0; i < len(filtered); i += chunkSize {
		end := i + chunkSize
		if end > len(filtered) {
			end = len(filtered)
		}
		part, err := e.Service.summarizeMessages(ctx, filtered[i:end])
		if err != nil {
			return driven.ChunkResult{}, err
		}
		raw, _ := json.Marshal(part)
		messageIDs := make([]uuid.UUID, 0, end-i)
		for _, msg := range filtered[i:end] {
			messageIDs = append(messageIDs, msg.ID)
		}
		now := time.Now().UTC()
		if err := e.Service.Summaries.UpsertSummaryJobChunk(ctx, driven.SummaryJobChunkRow{
			ID:          jobkit.DeterministicID(run.RunID, "summary_job_chunk", strconv.Itoa(i/chunkSize)),
			RunID:       run.RunID,
			AccountID:   *run.AccountID,
			ChunkIndex:  i / chunkSize,
			Phase:       "map",
			MessageIDs:  messageIDs,
			PayloadJSON: string(raw),
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			return driven.ChunkResult{}, err
		}
		partials = append(partials, *part)
		if text := strings.TrimSpace(part.GeneralSummary); text != "" {
			chunkSummaries = append(chunkSummaries, text)
		}
	}
	reduced := reduceSummaryPartials(partials)
	if len(partials) > 1 {
		combined, err := e.reducePartials(ctx, partials)
		if err == nil && combined != nil {
			reduced = *combined
		}
	}
	now := time.Now().UTC()
	if err := e.Service.Summaries.InsertSummarySnapshot(ctx, driven.SummarySnapshotRow{
		ID:             jobkit.DeterministicID(run.RunID, "summary_snapshot"),
		UserID:         run.UserID,
		AccountID:      run.AccountID,
		RunID:          run.RunID,
		WindowStart:    now.Add(-24 * time.Hour),
		WindowEnd:      now,
		GeneralSummary: firstNonEmpty(strings.TrimSpace(reduced.GeneralSummary), combineChunkSummaries(chunkSummaries)),
		CreatedAt:      now,
	}); err != nil {
		return driven.ChunkResult{}, err
	}
	openItems, err := e.Service.Summaries.ListOpenActionItems(ctx, run.UserID, run.AccountID)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	openByMessage := make(map[string]bool, len(openItems))
	for _, item := range openItems {
		openByMessage[item.MessageID.String()] = true
	}
	toInsert := make([]driven.ActionItemRow, 0, len(reduced.ActionItems))
	for _, item := range reduced.ActionItems {
		mid, err := uuid.Parse(strings.TrimSpace(item.MessageID))
		if err != nil || openByMessage[mid.String()] {
			continue
		}
		var dueAt *time.Time
		if strings.TrimSpace(item.DueAt) != "" {
			if t, err := time.Parse(time.RFC3339, item.DueAt); err == nil {
				ut := t.UTC()
				dueAt = &ut
			}
		}
		toInsert = append(toInsert, driven.ActionItemRow{
			ID:        jobkit.DeterministicID(run.RunID, "action_item", mid.String()),
			UserID:    run.UserID,
			AccountID: *run.AccountID,
			MessageID: mid,
			RunID:     run.RunID,
			Text:      strings.TrimSpace(item.Text),
			DueAt:     dueAt,
			Status:    "open",
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	if err := e.Service.Summaries.InsertActionItems(ctx, toInsert); err != nil {
		return driven.ChunkResult{}, err
	}
	existingFYI, err := e.Service.Summaries.ListFYIByRun(ctx, run.UserID, run.RunID)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	existingFYIByMessage := map[string]struct{}{}
	for _, item := range existingFYI {
		existingFYIByMessage[item.MessageID.String()] = struct{}{}
	}
	fyiRows := make([]driven.FYIRow, 0, len(reduced.FYI))
	for _, item := range reduced.FYI {
		mid, err := uuid.Parse(strings.TrimSpace(item.MessageID))
		if err != nil {
			continue
		}
		if _, ok := existingFYIByMessage[mid.String()]; ok {
			continue
		}
		fyiRows = append(fyiRows, driven.FYIRow{
			ID:        jobkit.DeterministicID(run.RunID, "fyi", mid.String()),
			UserID:    run.UserID,
			AccountID: *run.AccountID,
			MessageID: mid,
			RunID:     run.RunID,
			Text:      strings.TrimSpace(item.Text),
			CreatedAt: now,
		})
	}
	if err := e.Service.Summaries.InsertFYI(ctx, fyiRows); err != nil {
		return driven.ChunkResult{}, err
	}
	markSeen := make([]uuid.UUID, 0, len(filtered))
	for _, msg := range filtered {
		markSeen = append(markSeen, msg.ID)
	}
	if err := e.Service.Messages.MarkMessagesSummarySeen(ctx, run.UserID, markSeen, now); err != nil {
		return driven.ChunkResult{}, err
	}
	result := driven.ChunkResult{
		ProgressDelta: driven.JobProgress{
			Processed: len(rows),
			Detail: map[string]interface{}{
				"summarize_candidates": len(filtered),
				"action_items":         len(toInsert),
				"fyi_items":            len(fyiRows),
			},
		},
		Done: done,
	}
	if !done {
		result.NextCursor = jobkit.EncodeOffsetCursor(offset + len(rows))
	}
	return result, nil
}

func (e *SummarizeExecutor) reducePartials(ctx context.Context, partials []summarizePayload) (*summarizePayload, error) {
	lines := make([]string, 0, len(partials))
	for i, part := range partials {
		raw, _ := json.Marshal(part)
		lines = append(lines, fmt.Sprintf("partial_%d=%s", i+1, string(raw)))
	}
	resp, err := driven.ChatCompletion(ctx, e.Service.LLM, []driven.LLMMessage{
		{Role: "system", Content: "You reduce summary partials into one JSON object. Return JSON only."},
		{Role: "user", Content: "Reduce these summary partials into one JSON payload matching the existing schema.\n" + strings.Join(lines, "\n")},
	}, driven.LLMRequestOptions{MaxOutputTokens: 1200, ReasoningHint: "low"})
	if err != nil {
		return nil, err
	}
	var out summarizePayload
	if err := json.Unmarshal([]byte(normalizeJSONContent(resp.Content)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func reduceSummaryPartials(partials []summarizePayload) summarizePayload {
	combined := summarizePayload{
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
	summaries := make([]string, 0, len(partials))
	seenAction := map[string]struct{}{}
	seenFYI := map[string]struct{}{}
	for _, part := range partials {
		if text := strings.TrimSpace(part.GeneralSummary); text != "" {
			summaries = append(summaries, text)
		}
		for _, item := range part.ActionItems {
			key := strings.TrimSpace(item.MessageID)
			if key == "" {
				continue
			}
			if _, ok := seenAction[key]; ok {
				continue
			}
			seenAction[key] = struct{}{}
			combined.ActionItems = append(combined.ActionItems, item)
		}
		for _, item := range part.FYI {
			key := strings.TrimSpace(item.MessageID)
			if key == "" {
				continue
			}
			if _, ok := seenFYI[key]; ok {
				continue
			}
			seenFYI[key] = struct{}{}
			combined.FYI = append(combined.FYI, item)
		}
	}
	combined.GeneralSummary = combineChunkSummaries(summaries)
	return combined
}

type ForwardRulesExecutor struct {
	Service *ForwardRulesService
	Store   driven.JobStore
}

func (e *ForwardRulesExecutor) JobType() string { return "forward_rules" }

func (e *ForwardRulesExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	if e == nil || e.Service == nil || e.Store == nil {
		return driven.ChunkResult{}, fmt.Errorf("forward rules executor not configured")
	}
	if run.AccountID == nil || *run.AccountID == uuid.Nil {
		return driven.ChunkResult{}, fmt.Errorf("account_id is required")
	}
	offset := jobkit.DecodeOffsetCursor(run.Cursor)
	rows, err := e.Service.Messages.ListMessages(ctx, run.UserID, driven.MessageListFilter{
		AccountID: run.AccountID,
		Limit:     11,
		Offset:    offset,
	})
	if err != nil {
		return driven.ChunkResult{}, err
	}
	done := len(rows) <= 10
	if len(rows) > 10 {
		rows = rows[:10]
	}
	allowlistRows, err := e.Service.Forwards.ListForwardAllowlist(ctx, run.UserID)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	allowed := map[string]struct{}{}
	for _, row := range allowlistRows {
		allowed[strings.ToLower(strings.TrimSpace(row.Email))] = struct{}{}
	}
	rules, err := e.Service.Forwards.ListForwardRules(ctx, run.UserID, *run.AccountID)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	var accessToken string
	needToken := false
	for _, row := range rows {
		if row.ForwardSeenAt == nil {
			needToken = true
			break
		}
	}
	if needToken {
		account, cipher, err := e.Service.Accounts.GetAccount(ctx, run.UserID, *run.AccountID)
		if err != nil || account == nil {
			if err == nil {
				err = fmt.Errorf("account not found")
			}
			return driven.ChunkResult{}, err
		}
		accessToken, err = e.Service.refreshToken(ctx, run.UserID, *run.AccountID, account, cipher)
		if err != nil {
			return driven.ChunkResult{}, err
		}
	}
	forwarded, skipped, failed := 0, 0, 0
	seenMessageIDs := make([]uuid.UUID, 0, len(rows))
	for _, msg := range rows {
		if msg.ForwardSeenAt != nil {
			continue
		}
		messageForwarded := false
		messageHadFailure := false
		var graphMsgID string
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			if _, ok := allowed[strings.ToLower(strings.TrimSpace(rule.ForwardTo))]; !ok {
				reason := "forward_to not in allowlist"
				_ = e.insertAudit(ctx, run, msg.ID, rule.ID, "failed", &reason)
				failed++
				messageHadFailure = true
				continue
			}
			match, reason, err := e.Service.ruleMatches(ctx, rule, msg)
			if err != nil {
				msgErr := err.Error()
				_ = e.insertAudit(ctx, run, msg.ID, rule.ID, "failed", &msgErr)
				failed++
				messageHadFailure = true
				continue
			}
			if !match {
				_ = e.insertAudit(ctx, run, msg.ID, rule.ID, "skipped", &reason)
				skipped++
				continue
			}
			effectKey := fmt.Sprintf("forward:%s:%s:%s", msg.ID.String(), rule.ID.String(), strings.ToLower(strings.TrimSpace(rule.ForwardTo)))
			effect, err := e.Store.ClaimEffect(ctx, driven.ClaimEffectInput{
				AccountID: *run.AccountID,
				EffectKey: effectKey,
				JobID:     run.RunID,
				AttemptID: run.AttemptID,
				Now:       time.Now().UTC(),
			})
			if err != nil {
				if err == driven.ErrEffectAlreadyClaimed {
					existing, getErr := e.Store.GetEffect(ctx, *run.AccountID, effectKey)
					if getErr == nil && existing != nil && existing.State != driven.EffectUnknown {
						messageForwarded = true
					}
					continue
				}
				return driven.ChunkResult{}, err
			}
			if graphMsgID == "" {
				graphMsgID, err = e.Service.Graph.ResolveGraphMessageID(ctx, accessToken, msg.ProviderMessageID)
				if err != nil {
					raw, _ := json.Marshal(map[string]any{"status": "resolve_failed", "error": err.Error()})
					_, _ = e.Store.UpdateEffect(ctx, *run.AccountID, effectKey, effect.Revision, driven.EffectRetryable, string(raw), time.Now().UTC())
					return driven.ChunkResult{Retryable: true, ErrorMessage: err.Error()}, err
				}
			}
			if err := e.Service.Graph.ForwardMessage(ctx, accessToken, graphMsgID, rule.ForwardTo, ""); err != nil {
				raw, _ := json.Marshal(map[string]any{"status": "unknown", "error": err.Error()})
				_, _ = e.Store.UpdateEffect(ctx, *run.AccountID, effectKey, effect.Revision, driven.EffectUnknown, string(raw), time.Now().UTC())
				msgErr := err.Error()
				_ = e.insertAudit(ctx, run, msg.ID, rule.ID, "failed", &msgErr)
				failed++
				messageHadFailure = true
				continue
			}
			raw, _ := json.Marshal(map[string]any{"status": "forwarded", "forward_to": rule.ForwardTo})
			_, _ = e.Store.UpdateEffect(ctx, *run.AccountID, effectKey, effect.Revision, driven.EffectSucceededPendingAudit, string(raw), time.Now().UTC())
			okReason := "rule matched and message forwarded"
			_ = e.insertAudit(ctx, run, msg.ID, rule.ID, "forwarded", &okReason)
			forwarded++
			messageForwarded = true
		}
		if messageForwarded || !messageHadFailure {
			seenMessageIDs = append(seenMessageIDs, msg.ID)
		}
	}
	if err := e.Service.Messages.MarkMessagesForwardSeen(ctx, run.UserID, seenMessageIDs, time.Now().UTC()); err != nil {
		return driven.ChunkResult{}, err
	}
	result := driven.ChunkResult{
		ProgressDelta: driven.JobProgress{
			Processed: len(rows),
			Failed:    failed,
			Detail: map[string]interface{}{
				"forwarded":   forwarded,
				"skipped":     skipped,
				"marked_seen": len(seenMessageIDs),
			},
		},
		Done: done,
	}
	if !done {
		result.NextCursor = jobkit.EncodeOffsetCursor(offset + len(rows))
	}
	return result, nil
}

func (e *ForwardRulesExecutor) insertAudit(ctx context.Context, run driven.RunContext, messageID, ruleID uuid.UUID, status string, reason *string) error {
	return e.Service.Forwards.InsertForwardAudit(ctx, driven.ForwardAuditRow{
		ID:        jobkit.DeterministicID(run.RunID, "forward_audit", messageID.String(), ruleID.String()),
		UserID:    run.UserID,
		AccountID: *run.AccountID,
		MessageID: messageID,
		RuleID:    ruleID,
		RunID:     run.RunID,
		Status:    status,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
