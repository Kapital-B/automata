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

type AutoDraftService struct {
	Messages   driven.MessageRepository
	Summaries  driven.SummaryRepository
	LLM        driven.LLMClient
	JobRuns    driven.JobRunRepository
	ModelLabel string
}

type AutoDraftOptions struct {
	RunID      *uuid.UUID
	Trigger    string
	OnlyUnseen bool
}

type AutoDraftResult struct {
	JobRunID        uuid.UUID
	ActionItemsSeen int
	DraftsGenerated int
}

type autoDraftPayload struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (s *AutoDraftService) GenerateForAccount(ctx context.Context, userID, accountID uuid.UUID, opts AutoDraftOptions) (*AutoDraftResult, error) {
	if s == nil || s.Messages == nil || s.Summaries == nil || s.LLM == nil || s.JobRuns == nil {
		return nil, fmt.Errorf("auto-draft service not configured")
	}
	runID := uuid.New()
	if opts.RunID != nil {
		runID = *opts.RunID
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	onlyUnseen := true
	if !opts.OnlyUnseen && opts.OnlyUnseen == false {
		onlyUnseen = false
	}
	started := time.Now().UTC()
	_ = s.JobRuns.InsertJobRun(ctx, runID, accountID, "draft_suggest", trigger, "running", started, time.Time{}, nil, `{}`)

	fail := func(err error) (*AutoDraftResult, error) {
		msg := err.Error()
		_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", timePtrAutoDraft(time.Now().UTC()), &msg, `{}`)
		return nil, err
	}

	items, err := s.Summaries.ListActionItemsForAutoDraft(ctx, userID, accountID, onlyUnseen, 100)
	if err != nil {
		return fail(err)
	}
	_ = s.JobRuns.UpdateJobRunMeta(ctx, runID, fmt.Sprintf(`{"total_action_items":%d,"processed_action_items":0,"drafts_generated":0,"only_unseen":%t}`, len(items), onlyUnseen))

	now := time.Now().UTC()
	seenIDs := make([]uuid.UUID, 0, len(items))
	drafts := make([]driven.DraftSuggestionRow, 0, len(items))
	replyCandidates := 0
	for idx, item := range items {
		seenIDs = append(seenIDs, item.ID)
		if !actionItemNeedsReply(item.Text) {
			_ = s.JobRuns.UpdateJobRunMeta(ctx, runID, fmt.Sprintf(`{"total_action_items":%d,"processed_action_items":%d,"reply_candidates":%d,"drafts_generated":%d,"only_unseen":%t}`, len(items), idx+1, replyCandidates, len(drafts), onlyUnseen))
			continue
		}
		replyCandidates++
		msg, err := s.Messages.GetMessage(ctx, userID, item.MessageID)
		if err != nil || msg == nil {
			continue
		}
		draft, err := s.generateDraft(ctx, item, *msg)
		if err != nil {
			continue
		}
		drafts = append(drafts, driven.DraftSuggestionRow{
			ID:           uuid.New(),
			UserID:       userID,
			AccountID:    accountID,
			MessageID:    item.MessageID,
			ActionItemID: item.ID,
			RunID:        runID,
			Subject:      draft.Subject,
			Body:         draft.Body,
			Model:        s.ModelLabel,
			CreatedAt:    now,
		})
		_ = s.JobRuns.UpdateJobRunMeta(ctx, runID, fmt.Sprintf(`{"total_action_items":%d,"processed_action_items":%d,"reply_candidates":%d,"drafts_generated":%d,"only_unseen":%t}`, len(items), idx+1, replyCandidates, len(drafts), onlyUnseen))
	}
	if err := s.Summaries.MarkActionItemsAutoDraftSeen(ctx, userID, seenIDs, now); err != nil {
		return fail(err)
	}
	if err := s.Summaries.InsertDraftSuggestions(ctx, drafts); err != nil {
		return fail(err)
	}
	meta := fmt.Sprintf(`{"total_action_items":%d,"processed_action_items":%d,"reply_candidates":%d,"drafts_generated":%d,"action_items_seen":%d,"only_unseen":%t}`, len(items), len(items), replyCandidates, len(drafts), len(seenIDs), onlyUnseen)
	_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "success", timePtrAutoDraft(time.Now().UTC()), nil, meta)
	return &AutoDraftResult{JobRunID: runID, ActionItemsSeen: len(seenIDs), DraftsGenerated: len(drafts)}, nil
}

func (s *AutoDraftService) generateDraft(ctx context.Context, item driven.ActionItemRow, msg driven.MessageRow) (*autoDraftPayload, error) {
	body := derefStr(msg.BodyText)
	if looksLikeHTML(body) {
		body = stripHTML(body)
	}
	prompt := "Draft a concise professional email reply in JSON only: {\"subject\":\"...\",\"body\":\"...\"}. " +
		"Use the action item and email context.\n" +
		"Action item: " + clampText(item.Text, 300) + "\n" +
		"Original subject: " + clampText(msg.Subject, 180) + "\n" +
		"Original body: " + clampText(body, 1200)
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "You write helpful short email replies. Return JSON only."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}
	var out autoDraftPayload
	if err := json.Unmarshal([]byte(normalizeJSONContent(resp.Content)), &out); err != nil {
		return nil, err
	}
	out.Subject = strings.TrimSpace(out.Subject)
	out.Body = strings.TrimSpace(out.Body)
	if out.Subject == "" {
		out.Subject = "Re: " + strings.TrimSpace(msg.Subject)
	}
	return &out, nil
}

func timePtrAutoDraft(t time.Time) *time.Time {
	return &t
}

func actionItemNeedsReply(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	keywords := []string{"reply", "respond", "response", "follow up", "follow-up", "answer", "get back"}
	for _, kw := range keywords {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}
