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

type CategorizeService struct {
	Messages driven.MessageRepository
	LLM      driven.LLMClient
	JobRuns  driven.JobRunRepository
}

type CategorizeResult struct {
	JobRunID            uuid.UUID
	MessagesCategorized int
}

type CategorizeOptions struct {
	Recategorize bool
}

type categorizePayload struct {
	SchemaVersion int      `json:"schema_version"`
	CategorySlug  string   `json:"category_slug"`
	Confidence    *float64 `json:"confidence"`
}

func (s *CategorizeService) CategorizeAccount(ctx context.Context, userID, accountID uuid.UUID, opts CategorizeOptions) (*CategorizeResult, error) {
	if s == nil || s.Messages == nil || s.LLM == nil {
		return nil, fmt.Errorf("categorize service not configured")
	}
	jobID := uuid.New()
	started := time.Now().UTC()
	fail := func(err error) (*CategorizeResult, error) {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "categorize", "api", "failed", started, time.Now().UTC(), &msg, `{}`)
		}
		return nil, err
	}

	rows, err := s.Messages.ListMessages(ctx, userID, driven.MessageListFilter{
		AccountID: &accountID,
		Limit:     200,
	})
	if err != nil {
		return fail(err)
	}

	if !opts.Recategorize {
		uncategorized := make([]driven.MessageRow, 0, len(rows))
		for _, m := range rows {
			if m.CategorySlug == nil || *m.CategorySlug == "" {
				uncategorized = append(uncategorized, m)
			}
		}
		rows = uncategorized
	}

	count := 0
	for _, m := range rows {
		p, err := s.classifyMessage(ctx, m)
		if err != nil {
			return fail(err)
		}
		if p.CategorySlug == "" {
			p.CategorySlug = "other"
		}
		def, err := s.Messages.GetCategoryDefinitionBySlug(ctx, p.CategorySlug)
		if err != nil {
			return fail(err)
		}
		if def == nil {
			def, err = s.Messages.GetCategoryDefinitionBySlug(ctx, "other")
			if err != nil || def == nil {
				if err == nil {
					err = fmt.Errorf("missing category definition for %q", p.CategorySlug)
				}
				return fail(err)
			}
		}
		now := time.Now().UTC()
		err = s.Messages.UpsertMessageCategory(ctx, driven.MessageCategoryRow{
			ID:         uuid.New(),
			MessageID:  m.ID,
			AccountID:  accountID,
			CategoryID: def.ID,
			Source:     "llm",
			Confidence: p.Confidence,
			RunID:      jobID,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
		if err != nil {
			return fail(err)
		}
		count++
	}

	if s.JobRuns != nil {
		meta := fmt.Sprintf(`{"messages_categorized":%d,"recategorize":%t}`, count, opts.Recategorize)
		_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "categorize", "api", "success", started, time.Now().UTC(), nil, meta)
	}
	return &CategorizeResult{JobRunID: jobID, MessagesCategorized: count}, nil
}

func (s *CategorizeService) classifyMessage(ctx context.Context, m driven.MessageRow) (*categorizePayload, error) {
	const (
		maxSubjectChars = 220
		maxFromChars    = 280
		maxBodyChars    = 1200
	)
	prompt := "Classify this email into one category slug from: important, finance, personal, newsletter, spam, other. " +
		"Respond with a single JSON object only: {\"schema_version\":1,\"category_slug\":\"...\",\"confidence\":0..1}. " +
		"Use category_slug as lowercase.\n\n" +
		"Subject: " + clampText(m.Subject, maxSubjectChars) + "\n" +
		"From: " + clampText(m.FromJSON, maxFromChars) + "\n" +
		"Body: " + clampText(derefStr(m.BodyText), maxBodyChars)

	tryDecode := func(content string) (*categorizePayload, error) {
		var out categorizePayload
		normalized := normalizeJSONContent(content)
		if err := json.Unmarshal([]byte(normalized), &out); err != nil {
			return nil, err
		}
		out.CategorySlug = strings.ToLower(strings.TrimSpace(out.CategorySlug))
		return &out, nil
	}

	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "You are an email classifier. Return JSON only."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, err
	}
	parsed, err := tryDecode(resp.Content)
	if err == nil {
		return parsed, nil
	}

	resp2, err2 := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "Fix the following into valid JSON only. Keep semantic meaning."},
		{Role: "user", Content: resp.Content},
	})
	if err2 != nil {
		return nil, err
	}
	return tryDecode(resp2.Content)
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func clampText(s string, maxChars int) string {
	trimmed := strings.TrimSpace(s)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "...[truncated]"
}

func normalizeJSONContent(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(trimmed[start : end+1])
	}
	return trimmed
}
