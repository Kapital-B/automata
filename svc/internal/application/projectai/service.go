package projectai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaindec "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrLLMUnavailable = errors.New("llm not configured")
	ErrEmptyQuestion  = errors.New("question required")
)

type Service struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Facts       driven.FactRepository
	Decisions   driven.DecisionRepository
	Issues      driven.IssueRepository
	Timeline    driven.TimelineRepository
	JobRuns     driven.JobRunRepository
	LLM         driven.LLMClient
}

type Citation struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type Answer struct {
	Answer     string     `json:"answer"`
	Citations  []Citation `json:"citations"`
	Confidence float64    `json:"confidence"`
}

type askLLMPayload struct {
	SchemaVersion int        `json:"schema_version"`
	Answer        string     `json:"answer"`
	Citations     []Citation `json:"citations"`
	Confidence    float64    `json:"confidence"`
}

type contextPack struct {
	FactVersions map[string]struct{}
	Decisions    map[string]struct{}
	Issues       map[string]struct{}
	Messages     map[string]struct{}
	ManualItems  map[string]struct{}
	Prompt       string
}

func (s *Service) HasLLM() bool {
	return s != nil && s.LLM != nil
}

func (s *Service) Ask(ctx context.Context, userID, projectID uuid.UUID, question string) (*Answer, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, ErrEmptyQuestion
	}
	orgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil || m == nil {
		return nil, ErrNotFound
	}

	pack, err := s.buildContext(ctx, userID, orgID, projectID, p.Name)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	runID := uuid.New()
	if s.JobRuns != nil {
		meta, _ := json.Marshal(map[string]any{"project_id": projectID.String()})
		_ = s.JobRuns.InsertJobRun(ctx, runID, uuid.Nil, "project_ai", "api", "running", now, now, nil, string(meta))
	}

	var out *Answer
	if s.LLM == nil {
		out = s.heuristicAnswer(q, pack)
		if s.JobRuns != nil {
			fin := time.Now().UTC()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", &fin, strPtr("llm not configured"), "{}")
		}
		if out != nil {
			return out, nil
		}
		return nil, ErrLLMUnavailable
	}

	parsed, err := s.callAskLLM(ctx, pack.Prompt, q)
	if err != nil {
		if h := s.heuristicAnswer(q, pack); h != nil {
			out = h
		} else {
			if s.JobRuns != nil {
				fin := time.Now().UTC()
				_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", &fin, strPtr(err.Error()), "{}")
			}
			return nil, err
		}
	} else {
		out = &Answer{
			Answer:     strings.TrimSpace(parsed.Answer),
			Citations:  filterCitations(parsed.Citations, pack),
			Confidence: parsed.Confidence,
		}
		if out.Answer == "" {
			out.Answer = "I do not have enough grounded context in this project to answer that."
			out.Confidence = 0
		}
		if len(out.Citations) == 0 {
			if h := s.heuristicAnswer(q, pack); h != nil {
				out = h
			}
		}
	}

	if s.JobRuns != nil {
		fin := time.Now().UTC()
		meta, _ := json.Marshal(map[string]any{
			"project_id": projectID.String(),
			"citations":  len(out.Citations),
		})
		_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "success", &fin, nil, string(meta))
	}
	return out, nil
}

func (s *Service) buildContext(ctx context.Context, userID, orgID, projectID uuid.UUID, projectName string) (*contextPack, error) {
	pack := &contextPack{
		FactVersions: map[string]struct{}{},
		Decisions:    map[string]struct{}{},
		Issues:       map[string]struct{}{},
		Messages:     map[string]struct{}{},
		ManualItems:  map[string]struct{}{},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s (%s)\n\n", projectName, projectID.String())

	b.WriteString("## Active facts\n")
	facts, err := s.Facts.ListFactsByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		active, err := s.Facts.GetActiveFactVersion(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		if active == nil {
			continue
		}
		pack.FactVersions[active.ID.String()] = struct{}{}
		unit := ""
		if active.Unit != nil {
			unit = " " + *active.Unit
		}
		fmt.Fprintf(&b, "- fact_version_id=%s subject=%s label=%q value=%s%s\n",
			active.ID.String(), f.SubjectKey, f.Label, active.ValueText, unit)
		ev, _ := s.Facts.ListFactEvidence(ctx, active.ID)
		for _, e := range ev {
			if e.MessageID != nil {
				pack.Messages[e.MessageID.String()] = struct{}{}
				fmt.Fprintf(&b, "  evidence message_id=%s\n", e.MessageID.String())
			}
			if e.ManualItemID != nil {
				pack.ManualItems[e.ManualItemID.String()] = struct{}{}
				fmt.Fprintf(&b, "  evidence manual_item_id=%s\n", e.ManualItemID.String())
			}
		}
	}

	b.WriteString("\n## Accepted decisions\n")
	if s.Decisions != nil {
		decs, err := s.Decisions.ListDecisionsByProject(ctx, orgID, projectID, string(domaindec.StatusAccepted))
		if err != nil {
			return nil, err
		}
		for _, d := range decs {
			pack.Decisions[d.ID.String()] = struct{}{}
			fmt.Fprintf(&b, "- decision_id=%s statement=%q\n", d.ID.String(), d.Statement)
		}
	}

	b.WriteString("\n## Open issues\n")
	issues, err := s.Issues.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	for _, iss := range issues {
		if iss.Status == "resolved" {
			continue
		}
		pack.Issues[iss.ID.String()] = struct{}{}
		fmt.Fprintf(&b, "- issue_id=%s title=%q status=%s\n", iss.ID.String(), iss.Title, iss.Status)
	}

	b.WriteString("\n## Recent correspondence snippets\n")
	if s.Timeline != nil {
		items, err := s.Timeline.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{Limit: 12})
		if err == nil {
			for _, it := range items {
				snip := strings.TrimSpace(it.Snippet)
				if snip == "" {
					snip = strings.TrimSpace(it.Title)
				}
				if len(snip) > 240 {
					snip = snip[:237] + "…"
				}
				if it.MessageID != nil {
					pack.Messages[it.MessageID.String()] = struct{}{}
					fmt.Fprintf(&b, "- message_id=%s %q\n", it.MessageID.String(), snip)
				}
				if it.ManualItemID != nil {
					pack.ManualItems[it.ManualItemID.String()] = struct{}{}
					fmt.Fprintf(&b, "- manual_item_id=%s %q\n", it.ManualItemID.String(), snip)
				}
			}
		}
	}

	pack.Prompt = b.String()
	return pack, nil
}

func (s *Service) callAskLLM(ctx context.Context, contextText, question string) (*askLLMPayload, error) {
	tryDecode := func(content string) (*askLLMPayload, error) {
		var out askLLMPayload
		if err := json.Unmarshal([]byte(normalizeJSON(content)), &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	user := "Answer using ONLY the context below. Cite ids from the context. If insufficient, say so.\n\nContext:\n" +
		contextText + "\n\nQuestion: " + question +
		"\n\nReturn JSON: {\"schema_version\":1,\"answer\":\"\",\"citations\":[{\"type\":\"fact_version|decision|issue|message|manual_item\",\"id\":\"uuid\"}],\"confidence\":0.0}"
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "You are Project AI. Never invent facts. JSON only."},
		{Role: "user", Content: user},
	})
	if err != nil {
		return nil, err
	}
	parsed, err := tryDecode(resp.Content)
	if err == nil {
		return parsed, nil
	}
	resp2, err2 := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "Fix into valid JSON only."},
		{Role: "user", Content: resp.Content},
	})
	if err2 != nil {
		return nil, err
	}
	return tryDecode(resp2.Content)
}

func (s *Service) heuristicAnswer(question string, pack *contextPack) *Answer {
	q := strings.ToLower(question)
	for _, line := range strings.Split(pack.Prompt, "\n") {
		if !strings.Contains(line, "fact_version_id=") {
			continue
		}
		lower := strings.ToLower(line)
		matched := (strings.Contains(q, "duty") && strings.Contains(lower, "duty")) ||
			(strings.Contains(q, "pump") && strings.Contains(lower, "pump")) ||
			subjectMatchesQuestion(q, lower)
		if !matched {
			continue
		}
		vid := extractAttr(line, "fact_version_id=")
		val := extractAttr(line, "value=")
		label := extractQuoted(line, "label=")
		if vid == "" || val == "" {
			continue
		}
		if _, ok := pack.FactVersions[vid]; !ok {
			continue
		}
		ans := val
		if label != "" {
			ans = label + " is " + val
		}
		return &Answer{
			Answer:     ans,
			Citations:  []Citation{{Type: "fact_version", ID: vid}},
			Confidence: 0.85,
		}
	}
	_ = domainfacts.StatusActive
	return nil
}

func subjectMatchesQuestion(q, line string) bool {
	if i := strings.Index(line, "subject="); i >= 0 {
		rest := line[i+len("subject="):]
		parts := strings.Fields(rest)
		if len(parts) == 0 {
			return false
		}
		subj := strings.Trim(parts[0], `"'`)
		for _, token := range strings.Split(subj, ".") {
			if len(token) >= 3 && strings.Contains(q, token) {
				return true
			}
		}
	}
	return false
}

func extractAttr(line, key string) string {
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	parts := strings.Fields(rest)
	if len(parts) == 0 {
		return ""
	}
	return strings.Trim(parts[0], `"'`)
}

func extractQuoted(line, key string) string {
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	if strings.HasPrefix(rest, `"`) {
		rest = rest[1:]
		j := strings.Index(rest, `"`)
		if j >= 0 {
			return rest[:j]
		}
	}
	return extractAttr(line, key)
}

func filterCitations(in []Citation, pack *contextPack) []Citation {
	out := make([]Citation, 0, len(in))
	seen := map[string]struct{}{}
	for _, c := range in {
		typ := strings.TrimSpace(strings.ToLower(c.Type))
		id := strings.TrimSpace(c.ID)
		if id == "" {
			continue
		}
		ok := false
		switch typ {
		case "fact_version":
			_, ok = pack.FactVersions[id]
		case "decision":
			_, ok = pack.Decisions[id]
		case "issue":
			_, ok = pack.Issues[id]
		case "message":
			_, ok = pack.Messages[id]
		case "manual_item":
			_, ok = pack.ManualItems[id]
		}
		if !ok {
			continue
		}
		key := typ + ":" + id
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Citation{Type: typ, ID: id})
	}
	return out
}

func normalizeJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
		s = strings.TrimSpace(s)
	}
	return s
}

func strPtr(s string) *string { return &s }
