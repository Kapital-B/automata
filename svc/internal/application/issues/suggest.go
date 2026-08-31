package issues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

var (
	ErrLLMUnavailable   = errors.New("llm not configured")
	ErrNothingToSuggest = errors.New("no unassigned correspondence to suggest from")
)

// SuggestInput scopes an LLM issue proposal.
type SuggestInput struct {
	// AccountID optional: when set, only mail from this account is considered (with manuals).
	AccountID *uuid.UUID
}

// SuggestResult is a proposal the user must confirm via Create.
type SuggestResult struct {
	Title      string
	ItemRefs   []ItemRef
	Confidence float64
	Reason     string
}

type suggestLLMPayload struct {
	SchemaVersion int      `json:"schema_version"`
	ProjectID     string   `json:"project_id"`
	IssueTitle    string   `json:"issue_title"`
	MessageIDs    []string `json:"message_ids"`
	ManualItemIDs []string `json:"manual_item_ids"`
	Confidence    float64  `json:"confidence"`
	Reason        string   `json:"reason"`
}

type suggestCandidate struct {
	Key          string
	Source       string
	Title        string
	Snippet      string
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AccountID    *uuid.UUID
}

// Suggest asks the LLM for an issue title and item refs from unassigned timeline items.
// It never creates an issue. Mail in one call is always from a single account_id.
func (s *Service) Suggest(ctx context.Context, userID, projectID uuid.UUID, in SuggestInput) (*SuggestResult, error) {
	if s.LLM == nil {
		return nil, ErrLLMUnavailable
	}
	if s.Timeline == nil {
		return nil, fmt.Errorf("timeline not configured")
	}
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.ArchivedAt != nil {
		return nil, ErrNotFound
	}

	items, err := s.Timeline.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{
		Source: "all", UnassignedToIssue: true, Limit: 40,
	})
	if err != nil {
		return nil, err
	}
	candidates := buildSuggestCandidates(items, in.AccountID)
	if len(candidates) == 0 {
		return nil, ErrNothingToSuggest
	}

	prompt := buildSuggestPrompt(projectID, p.Name, p.Code, candidates)
	parsed, err := s.callSuggestLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}

	allowedMsg := map[uuid.UUID]struct{}{}
	allowedManual := map[uuid.UUID]struct{}{}
	for _, c := range candidates {
		if c.MessageID != nil {
			allowedMsg[*c.MessageID] = struct{}{}
		}
		if c.ManualItemID != nil {
			allowedManual[*c.ManualItemID] = struct{}{}
		}
	}

	refs := make([]ItemRef, 0)
	for _, idStr := range parsed.MessageIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			continue
		}
		if _, ok := allowedMsg[id]; !ok {
			continue
		}
		mid := id
		refs = append(refs, ItemRef{MessageID: &mid})
	}
	for _, idStr := range parsed.ManualItemIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			continue
		}
		if _, ok := allowedManual[id]; !ok {
			continue
		}
		mid := id
		refs = append(refs, ItemRef{ManualItemID: &mid})
	}

	title := strings.TrimSpace(parsed.IssueTitle)
	if title == "" {
		title = fallbackSuggestTitle(candidates)
	}
	conf := parsed.Confidence
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}

	return &SuggestResult{
		Title:      title,
		ItemRefs:   refs,
		Confidence: conf,
		Reason:     strings.TrimSpace(parsed.Reason),
	}, nil
}

func buildSuggestCandidates(items []driven.TimelineItem, accountFilter *uuid.UUID) []suggestCandidate {
	manuals := make([]suggestCandidate, 0)
	mailByAccount := map[uuid.UUID][]suggestCandidate{}
	accountOrder := make([]uuid.UUID, 0)

	for _, it := range items {
		if it.IssueID != nil {
			continue
		}
		if it.Source == "manual" && it.ManualItemID != nil {
			manuals = append(manuals, suggestCandidate{
				Key: "manual:" + it.ManualItemID.String(), Source: "manual",
				Title: it.Title, Snippet: it.Snippet, ManualItemID: it.ManualItemID,
			})
			continue
		}
		if it.Source == "mail" && it.MessageID != nil && it.AccountID != nil {
			if accountFilter != nil && *it.AccountID != *accountFilter {
				continue
			}
			c := suggestCandidate{
				Key: "mail:" + it.MessageID.String(), Source: "mail",
				Title: it.Title, Snippet: it.Snippet, MessageID: it.MessageID, AccountID: it.AccountID,
			}
			if _, seen := mailByAccount[*it.AccountID]; !seen {
				accountOrder = append(accountOrder, *it.AccountID)
			}
			mailByAccount[*it.AccountID] = append(mailByAccount[*it.AccountID], c)
		}
	}

	if accountFilter != nil {
		return append(append([]suggestCandidate{}, manuals...), mailByAccount[*accountFilter]...)
	}

	// Single-account rule: pick the account with the most unassigned mail.
	var chosen []suggestCandidate
	best := -1
	for _, accID := range accountOrder {
		mail := mailByAccount[accID]
		if len(mail) > best {
			best = len(mail)
			chosen = mail
		}
	}
	return append(append([]suggestCandidate{}, manuals...), chosen...)
}

func buildSuggestPrompt(projectID uuid.UUID, name, code string, candidates []suggestCandidate) string {
	var b strings.Builder
	b.WriteString("Propose one project issue from the unassigned correspondence below. ")
	b.WriteString("Respond with a single JSON object only: ")
	b.WriteString(`{"schema_version":1,"project_id":"` + projectID.String() + `","issue_title":"...","message_ids":[],"manual_item_ids":[],"confidence":0..1,"reason":"..."}. `)
	b.WriteString("Only use ids from the list. Prefer items that clearly belong together. ")
	b.WriteString("If unsure, return fewer ids and lower confidence.\n\n")
	b.WriteString("Project: " + name + " (" + code + ")\n\nItems:\n")
	for i, c := range candidates {
		id := ""
		if c.MessageID != nil {
			id = "message_id=" + c.MessageID.String()
		} else if c.ManualItemID != nil {
			id = "manual_item_id=" + c.ManualItemID.String()
		}
		fmt.Fprintf(&b, "%d. [%s] %s | %s | %s\n", i+1, c.Source, id, clampSuggestText(c.Title, 120), clampSuggestText(c.Snippet, 200))
	}
	return b.String()
}

func (s *Service) callSuggestLLM(ctx context.Context, prompt string) (*suggestLLMPayload, error) {
	tryDecode := func(content string) (*suggestLLMPayload, error) {
		var out suggestLLMPayload
		if err := json.Unmarshal([]byte(normalizeSuggestJSON(content)), &out); err != nil {
			return nil, err
		}
		out.IssueTitle = strings.TrimSpace(out.IssueTitle)
		out.Reason = strings.TrimSpace(out.Reason)
		return &out, nil
	}
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "You group project correspondence into issues. Return JSON only."},
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

func fallbackSuggestTitle(candidates []suggestCandidate) string {
	for _, c := range candidates {
		if t := strings.TrimSpace(c.Title); t != "" {
			return clampSuggestText(t, 80)
		}
	}
	return "Untitled issue"
}

func clampSuggestText(s string, maxChars int) string {
	trimmed := strings.TrimSpace(s)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "…"
}

func normalizeSuggestJSON(s string) string {
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
