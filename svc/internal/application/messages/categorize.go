package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
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
	RunID        *uuid.UUID
	Trigger      string
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
	if opts.RunID != nil {
		jobID = *opts.RunID
	}
	trigger := strings.TrimSpace(opts.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	started := time.Now().UTC()
	fail := func(err error) (*CategorizeResult, error) {
		if s.JobRuns != nil {
			msg := err.Error()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "failed", timePtr(time.Now().UTC()), &msg, `{}`)
		}
		return nil, err
	}
	if s.JobRuns != nil {
		if opts.RunID != nil {
			if err := s.JobRuns.PromoteJobRunToRunning(ctx, jobID, started); err != nil {
				return fail(err)
			}
		} else {
			_ = s.JobRuns.InsertJobRun(ctx, jobID, accountID, "categorize", trigger, "running", started, time.Time{}, nil, `{}`)
		}
	}

	categories, err := s.Messages.ListCategoryDefinitions(ctx, userID)
	if err != nil {
		return fail(err)
	}
	if len(categories) == 0 {
		return fail(fmt.Errorf("no categories defined for user"))
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
	if s.JobRuns != nil {
		_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":0,"messages_categorized":0,"recategorize":%t}`, len(rows), opts.Recategorize))
	}

	count := 0
	for _, m := range rows {
		p, err := s.classifyMessage(ctx, m, categories)
		if err != nil {
			return fail(err)
		}
		if p.CategorySlug == "" || !hasCategorySlug(categories, p.CategorySlug) {
			p.CategorySlug = fallbackCategorySlug(categories)
		}
		def, err := s.Messages.GetCategoryDefinitionBySlug(ctx, userID, p.CategorySlug)
		if err != nil {
			return fail(err)
		}
		if def == nil {
			def, err = s.Messages.GetCategoryDefinitionBySlug(ctx, userID, fallbackCategorySlug(categories))
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
		if s.JobRuns != nil {
			_ = s.JobRuns.UpdateJobRunMeta(ctx, jobID, fmt.Sprintf(`{"total_messages":%d,"processed_messages":%d,"messages_categorized":%d,"recategorize":%t}`, len(rows), count, count, opts.Recategorize))
		}
	}

	if s.JobRuns != nil {
		meta := fmt.Sprintf(`{"messages_categorized":%d,"recategorize":%t}`, count, opts.Recategorize)
		_ = s.JobRuns.UpdateJobRunStatus(ctx, jobID, "success", timePtr(time.Now().UTC()), nil, meta)
	}
	return &CategorizeResult{JobRunID: jobID, MessagesCategorized: count}, nil
}

func (s *CategorizeService) classifyMessage(ctx context.Context, m driven.MessageRow, categories []driven.CategoryDefinitionRow) (*categorizePayload, error) {
	const (
		maxSubjectChars = 220
		maxFromChars    = 280
		maxBodyChars    = 1200
	)
	body := derefStr(m.BodyText)
	if looksLikeHTML(body) {
		body = stripHTML(body)
	}
	prompt := "Classify this email into one category slug from: " + strings.Join(categorySlugList(categories), ", ") + ". " +
		"Respond with a single JSON object only: {\"schema_version\":1,\"category_slug\":\"...\",\"confidence\":0..1}. " +
		"Use category_slug as lowercase.\n\n" +
		"Category definitions:\n" + categoryDefinitionsBlock(categories) + "\n\n" +
		"Subject: " + clampText(m.Subject, maxSubjectChars) + "\n" +
		"From: " + clampText(m.FromJSON, maxFromChars) + "\n" +
		"Body: " + clampText(body, maxBodyChars)

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

func categorySlugList(categories []driven.CategoryDefinitionRow) []string {
	out := make([]string, 0, len(categories))
	for _, c := range categories {
		out = append(out, c.Slug)
	}
	return out
}

func categoryDefinitionsBlock(categories []driven.CategoryDefinitionRow) string {
	lines := make([]string, 0, len(categories))
	for _, c := range categories {
		def := strings.TrimSpace(c.Definition)
		if def == "" {
			def = "(no extra definition)"
		}
		lines = append(lines, "- "+c.Slug+": "+def)
	}
	return strings.Join(lines, "\n")
}

func hasCategorySlug(categories []driven.CategoryDefinitionRow, slug string) bool {
	for _, c := range categories {
		if c.Slug == slug {
			return true
		}
	}
	return false
}

func fallbackCategorySlug(categories []driven.CategoryDefinitionRow) string {
	if len(categories) == 0 {
		return ""
	}
	for _, c := range categories {
		if c.Slug == "other" {
			return "other"
		}
	}
	sorted := append([]driven.CategoryDefinitionRow(nil), categories...)
	slices.SortFunc(sorted, func(a, b driven.CategoryDefinitionRow) int {
		if a.SortOrder != b.SortOrder {
			if a.SortOrder < b.SortOrder {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	return sorted[0].Slug
}

func timePtr(t time.Time) *time.Time {
	return &t
}
