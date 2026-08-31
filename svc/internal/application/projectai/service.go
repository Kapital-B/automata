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
	Users     driven.UserRepository
	Projects  driven.ProjectRepository
	Facts     driven.FactRepository
	Decisions driven.DecisionRepository
	Issues    driven.IssueRepository
	Timeline  driven.TimelineRepository
	JobRuns   driven.JobRunRepository
	LLM       driven.LLMClient
	Attention AttentionProjects
}

// AttentionProjects reports home-org projects that currently need operator input.
type AttentionProjects interface {
	ProjectIDsNeedingInput(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]struct{}, error)
}

const maxAskAcrossProjects = 8

type contextLimits struct {
	Facts     int
	Decisions int
	Issues    int
	Timeline  int
}

func singleAskLimits() contextLimits {
	return contextLimits{Facts: 40, Decisions: 24, Issues: 24, Timeline: 12}
}

func acrossAskLimits() contextLimits {
	return contextLimits{Facts: 16, Decisions: 8, Issues: 8, Timeline: 6}
}

type Citation struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectCode string `json:"project_code,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
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
	CiteMeta     map[string]Citation
	Prompt       string
}

func newContextPack() *contextPack {
	return &contextPack{
		FactVersions: map[string]struct{}{},
		Decisions:    map[string]struct{}{},
		Issues:       map[string]struct{}{},
		Messages:     map[string]struct{}{},
		ManualItems:  map[string]struct{}{},
		CiteMeta:     map[string]Citation{},
	}
}

func (p *contextPack) noteCite(typ, id string, project driven.ProjectRow) {
	if p == nil || id == "" {
		return
	}
	key := typ + ":" + id
	p.CiteMeta[key] = Citation{
		Type:        typ,
		ID:          id,
		ProjectID:   project.ID.String(),
		ProjectCode: project.Code,
		ProjectName: project.Name,
	}
	switch typ {
	case "fact_version":
		p.FactVersions[id] = struct{}{}
	case "decision":
		p.Decisions[id] = struct{}{}
	case "issue":
		p.Issues[id] = struct{}{}
	case "message":
		p.Messages[id] = struct{}{}
	case "manual_item":
		p.ManualItems[id] = struct{}{}
	}
}

func (p *contextPack) mergeFrom(other *contextPack) {
	if other == nil {
		return
	}
	for k, v := range other.FactVersions {
		p.FactVersions[k] = v
	}
	for k, v := range other.Decisions {
		p.Decisions[k] = v
	}
	for k, v := range other.Issues {
		p.Issues[k] = v
	}
	for k, v := range other.Messages {
		p.Messages[k] = v
	}
	for k, v := range other.ManualItems {
		p.ManualItems[k] = v
	}
	for k, v := range other.CiteMeta {
		p.CiteMeta[k] = v
	}
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

	pack, err := s.buildContext(ctx, userID, orgID, *p, singleAskLimits())
	if err != nil {
		return nil, err
	}

	return s.runAsk(ctx, q, pack, map[string]any{"project_id": projectID.String()}, false)
}

// AskAcross answers a question over the caller's accessible home-org projects.
func (s *Service) AskAcross(ctx context.Context, userID uuid.UUID, question string) (*Answer, error) {
	q := strings.TrimSpace(question)
	if q == "" {
		return nil, ErrEmptyQuestion
	}
	orgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return nil, err
	}
	projects, err := s.Projects.ListProjects(ctx, orgID, driven.ProjectListFilter{Limit: 200})
	if err != nil {
		return nil, err
	}
	accessible := make([]driven.ProjectRow, 0, len(projects))
	for _, p := range projects {
		m, err := s.Projects.GetProjectMember(ctx, p.ID, userID)
		if err != nil || m == nil {
			continue
		}
		accessible = append(accessible, p)
	}
	prefer := map[uuid.UUID]struct{}{}
	if s.Attention != nil {
		ids, err := s.Attention.ProjectIDsNeedingInput(ctx, userID)
		if err != nil {
			return nil, err
		}
		prefer = ids
	}
	selected := SelectAskAcrossProjects(accessible, prefer, maxAskAcrossProjects)

	combined := newContextPack()
	var b strings.Builder
	b.WriteString("MULTI-PROJECT CONTEXT\n")
	b.WriteString("Each PROJECT block is isolated. Never blend evidence across projects without citing which project.\n\n")
	if len(selected) == 0 {
		b.WriteString("(no accessible projects)\n")
	}
	projectIDs := make([]string, 0, len(selected))
	for _, p := range selected {
		pack, err := s.buildContext(ctx, userID, orgID, p, acrossAskLimits())
		if err != nil {
			return nil, err
		}
		combined.mergeFrom(pack)
		fmt.Fprintf(&b, "==== PROJECT code=%s id=%s name=%q ====\n%s\n", p.Code, p.ID.String(), p.Name, pack.Prompt)
		projectIDs = append(projectIDs, p.ID.String())
	}
	combined.Prompt = b.String()

	return s.runAsk(ctx, q, combined, map[string]any{
		"scope":         "across",
		"project_count": len(selected),
		"project_ids":   projectIDs,
		"capped":        len(accessible) > len(selected),
	}, true)
}

func (s *Service) runAsk(ctx context.Context, question string, pack *contextPack, jobMeta map[string]any, multi bool) (*Answer, error) {
	now := time.Now().UTC()
	runID := uuid.New()
	jobType := "project_ai"
	if s.JobRuns != nil {
		meta, _ := json.Marshal(jobMeta)
		_ = s.JobRuns.InsertJobRun(ctx, runID, uuid.Nil, jobType, "api", "running", now, now, nil, string(meta))
	}

	var out *Answer
	if s.LLM == nil {
		out = s.heuristicAnswer(question, pack)
		if s.JobRuns != nil {
			fin := time.Now().UTC()
			_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "failed", &fin, strPtr("llm not configured"), "{}")
		}
		if out != nil {
			return out, nil
		}
		return nil, ErrLLMUnavailable
	}

	parsed, err := s.callAskLLM(ctx, pack.Prompt, question, multi)
	if err != nil {
		if h := s.heuristicAnswer(question, pack); h != nil {
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
			if multi {
				out.Answer = "I do not have enough grounded context across your projects to answer that."
			} else {
				out.Answer = "I do not have enough grounded context in this project to answer that."
			}
			out.Confidence = 0
		}
		if len(out.Citations) == 0 {
			if h := s.heuristicAnswer(question, pack); h != nil {
				out = h
			}
		}
	}

	if s.JobRuns != nil {
		fin := time.Now().UTC()
		meta := map[string]any{}
		for k, v := range jobMeta {
			meta[k] = v
		}
		meta["citations"] = len(out.Citations)
		raw, _ := json.Marshal(meta)
		_ = s.JobRuns.UpdateJobRunStatus(ctx, runID, "success", &fin, nil, string(raw))
	}
	return out, nil
}

func (s *Service) buildContext(ctx context.Context, userID, orgID uuid.UUID, project driven.ProjectRow, limits contextLimits) (*contextPack, error) {
	pack := newContextPack()
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s code=%s (%s)\n\n", project.Name, project.Code, project.ID.String())

	b.WriteString("## Active facts\n")
	facts, err := s.Facts.ListFactsByProject(ctx, orgID, project.ID)
	if err != nil {
		return nil, err
	}
	factCount := 0
	for _, f := range facts {
		active, err := s.Facts.GetActiveFactVersion(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		if active == nil {
			continue
		}
		if limits.Facts > 0 && factCount >= limits.Facts {
			break
		}
		factCount++
		pack.noteCite("fact_version", active.ID.String(), project)
		unit := ""
		if active.Unit != nil {
			unit = " " + *active.Unit
		}
		fmt.Fprintf(&b, "- fact_version_id=%s subject=%s label=%q value=%s%s\n",
			active.ID.String(), f.SubjectKey, f.Label, active.ValueText, unit)
		ev, _ := s.Facts.ListFactEvidence(ctx, active.ID)
		for _, e := range ev {
			if e.MessageID != nil {
				pack.noteCite("message", e.MessageID.String(), project)
				fmt.Fprintf(&b, "  evidence message_id=%s\n", e.MessageID.String())
			}
			if e.ManualItemID != nil {
				pack.noteCite("manual_item", e.ManualItemID.String(), project)
				fmt.Fprintf(&b, "  evidence manual_item_id=%s\n", e.ManualItemID.String())
			}
		}
	}

	b.WriteString("\n## Accepted decisions\n")
	if s.Decisions != nil {
		decs, err := s.Decisions.ListDecisionsByProject(ctx, orgID, project.ID, string(domaindec.StatusAccepted))
		if err != nil {
			return nil, err
		}
		for i, d := range decs {
			if limits.Decisions > 0 && i >= limits.Decisions {
				break
			}
			pack.noteCite("decision", d.ID.String(), project)
			fmt.Fprintf(&b, "- decision_id=%s statement=%q\n", d.ID.String(), d.Statement)
		}
	}

	b.WriteString("\n## Open issues\n")
	issues, err := s.Issues.ListIssuesByProject(ctx, orgID, project.ID)
	if err != nil {
		return nil, err
	}
	issueCount := 0
	for _, iss := range issues {
		if iss.Status == "resolved" {
			continue
		}
		if limits.Issues > 0 && issueCount >= limits.Issues {
			break
		}
		issueCount++
		pack.noteCite("issue", iss.ID.String(), project)
		fmt.Fprintf(&b, "- issue_id=%s title=%q status=%s\n", iss.ID.String(), iss.Title, iss.Status)
	}

	b.WriteString("\n## Recent correspondence snippets\n")
	if s.Timeline != nil {
		timelineLimit := limits.Timeline
		if timelineLimit <= 0 {
			timelineLimit = 12
		}
		items, err := s.Timeline.ListProjectTimeline(ctx, userID, orgID, project.ID, driven.TimelineFilter{Limit: timelineLimit})
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
					pack.noteCite("message", it.MessageID.String(), project)
					fmt.Fprintf(&b, "- message_id=%s %q\n", it.MessageID.String(), snip)
				}
				if it.ManualItemID != nil {
					pack.noteCite("manual_item", it.ManualItemID.String(), project)
					fmt.Fprintf(&b, "- manual_item_id=%s %q\n", it.ManualItemID.String(), snip)
				}
			}
		}
	}

	pack.Prompt = b.String()
	return pack, nil
}

func (s *Service) callAskLLM(ctx context.Context, contextText, question string, multi bool) (*askLLMPayload, error) {
	tryDecode := func(content string) (*askLLMPayload, error) {
		var out askLLMPayload
		if err := json.Unmarshal([]byte(normalizeJSON(content)), &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	system := "You are Project AI. Never invent facts. JSON only."
	if multi {
		system = "You are Automata Project AI answering across multiple projects. Never invent facts. Never blend evidence across PROJECT blocks without naming the project. JSON only."
	}
	user := "Answer using ONLY the context below. Cite ids from the context. If insufficient, say so.\n\nContext:\n" +
		contextText + "\n\nQuestion: " + question +
		"\n\nReturn JSON: {\"schema_version\":1,\"answer\":\"\",\"citations\":[{\"type\":\"fact_version|decision|issue|message|manual_item\",\"id\":\"uuid\"}],\"confidence\":0.0}"
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: system},
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
		cite := Citation{Type: "fact_version", ID: vid}
		if meta, ok := pack.CiteMeta["fact_version:"+vid]; ok {
			cite = meta
		}
		return &Answer{
			Answer:     ans,
			Citations:  []Citation{cite},
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
		cite := Citation{Type: typ, ID: id}
		if meta, found := pack.CiteMeta[key]; found {
			cite = meta
		}
		out = append(out, cite)
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
