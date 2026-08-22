package interpret

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domaininterp "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
	"github.com/google/uuid"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrLLMUnavailable      = errors.New("llm not configured")
	ErrMixedAccounts       = errors.New("cannot mix mail account_id values in one interpret call")
	ErrNothingToInterpret  = errors.New("no correspondence to interpret")
	ErrNotPending          = errors.New("interpretation is not pending")
)

// Service runs Stage A interpret and manages pending candidates.
type Service struct {
	Users           driven.UserRepository
	Projects        driven.ProjectRepository
	Interpretations driven.InterpretationRepository
	Facts           driven.FactRepository
	Timeline        driven.TimelineRepository
	Assignments     driven.AssignmentRepository
	Manuals         driven.ManualItemRepository
	Messages        driven.MessageRepository
	JobRuns         driven.JobRunRepository
	LLM             driven.LLMClient
}

func (s *Service) HasLLM() bool {
	return s != nil && s.LLM != nil
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

type RunInput struct {
	AccountID     *uuid.UUID
	MessageIDs    []uuid.UUID
	ManualItemIDs []uuid.UUID
	Trigger       string // api | schedule (stored on job_runs)
}

type InterpretationView struct {
	Interpretation driven.InterpretationRow
	Sources        []driven.InterpretationSourceRow
	Candidates     []CandidateView
}

type CandidateView struct {
	Kind          string
	SubjectKey    string
	Label         string
	Value         any
	Unit          string
	Statement     string
	MessageIDs    []string
	ManualItemIDs []string
	Confidence    float64
	Reason        string
}

type interpretLLMPayload struct {
	SchemaVersion int                  `json:"schema_version"`
	ProjectID     string               `json:"project_id"`
	Candidates    []interpretCandidate `json:"candidates"`
}

type interpretCandidate struct {
	Kind          string          `json:"kind"`
	SubjectKey    string          `json:"subject_key"`
	Label         string          `json:"label"`
	Value         json.RawMessage `json:"value"`
	Unit          string          `json:"unit"`
	Statement     string          `json:"statement"`
	MessageIDs    []string        `json:"message_ids"`
	ManualItemIDs []string        `json:"manual_item_ids"`
	Confidence    float64         `json:"confidence"`
	Reason        string          `json:"reason"`
}

type interpretItem struct {
	Source       string
	Title        string
	Snippet      string
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AccountID    *uuid.UUID
}

func (s *Service) requireProjectMember(ctx context.Context, userID, projectID uuid.UUID) (uuid.UUID, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return uuid.Nil, err
	}
	if p == nil {
		return uuid.Nil, ErrNotFound
	}
	member, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if member == nil {
		return uuid.Nil, ErrNotFound
	}
	return orgID, nil
}

func (s *Service) ListPending(ctx context.Context, userID, projectID uuid.UUID) ([]InterpretationView, error) {
	orgID, err := s.requireProjectMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Interpretations.ListPendingInterpretations(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]InterpretationView, 0, len(rows))
	for _, row := range rows {
		view, err := s.buildView(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	return out, nil
}

func (s *Service) Dismiss(ctx context.Context, userID, interpretationID uuid.UUID) (*InterpretationView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Interpretations.GetInterpretation(ctx, orgID, interpretationID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireProjectMember(ctx, userID, row.ProjectID); err != nil {
		return nil, err
	}
	if row.Status != string(domaininterp.StatusPending) {
		return nil, ErrNotPending
	}
	now := time.Now().UTC()
	if err := s.Interpretations.UpdateInterpretationStatus(ctx, orgID, interpretationID, string(domaininterp.StatusDismissed), now); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	row.Status = string(domaininterp.StatusDismissed)
	row.UpdatedAt = now
	return s.buildView(ctx, *row)
}

// Run interprets project correspondence into a pending candidate payload. Never writes facts.
func (s *Service) Run(ctx context.Context, userID, projectID uuid.UUID, in RunInput) (*InterpretationView, error) {
	if s.LLM == nil {
		return nil, ErrLLMUnavailable
	}
	orgID, err := s.requireProjectMember(ctx, userID, projectID)
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

	items, accountID, err := s.resolveItems(ctx, userID, orgID, projectID, in)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNothingToInterpret
	}

	activeFacts, err := s.loadActiveFactContext(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}

	prompt := buildInterpretPrompt(projectID, p.Name, p.Code, items, activeFacts)
	parsed, err := s.callInterpretLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}

	allowedMsg := map[uuid.UUID]struct{}{}
	allowedManual := map[uuid.UUID]struct{}{}
	for _, it := range items {
		if it.MessageID != nil {
			allowedMsg[*it.MessageID] = struct{}{}
		}
		if it.ManualItemID != nil {
			allowedManual[*it.ManualItemID] = struct{}{}
		}
	}
	candidates := sanitizeCandidates(parsed.Candidates, allowedMsg, allowedManual, projectID)

	payload := interpretLLMPayload{
		SchemaVersion: 1,
		ProjectID:     projectID.String(),
		Candidates:    candidates,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	conf := averageConfidence(candidates)
	reason := summarizeReason(candidates)
	now := time.Now().UTC()
	runID := uuid.New()
	trigger := strings.TrimSpace(in.Trigger)
	if trigger == "" {
		trigger = "api"
	}
	accForJob := uuid.Nil
	if accountID != nil {
		accForJob = *accountID
	}
	meta, _ := json.Marshal(map[string]any{
		"project_id":       projectID.String(),
		"candidate_count":  len(candidates),
		"source_count":     len(items),
	})
	if s.JobRuns != nil {
		_ = s.JobRuns.InsertJobRun(ctx, runID, accForJob, "interpret_project", trigger, "success", now, now, nil, string(meta))
	}

	row := driven.InterpretationRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID,
		AccountID: accountID, RunID: &runID,
		Status: string(domaininterp.StatusPending), PayloadJSON: string(payloadBytes),
		Confidence: &conf, Reason: reason, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Interpretations.CreateInterpretation(ctx, row); err != nil {
		return nil, err
	}
	for _, it := range items {
		src := driven.InterpretationSourceRow{
			ID: uuid.New(), InterpretationID: row.ID,
			MessageID: it.MessageID, ManualItemID: it.ManualItemID,
		}
		if err := s.Interpretations.AddInterpretationSource(ctx, src); err != nil {
			return nil, err
		}
	}
	return s.buildView(ctx, row)
}

// TryRunBestEffort runs interpret when LLM is available; swallows all errors.
func (s *Service) TryRunBestEffort(ctx context.Context, userID, projectID uuid.UUID, in RunInput) {
	if s == nil || !s.HasLLM() {
		return
	}
	in.Trigger = "api"
	_, _ = s.Run(ctx, userID, projectID, in)
}

func (s *Service) resolveItems(ctx context.Context, userID, orgID, projectID uuid.UUID, in RunInput) ([]interpretItem, *uuid.UUID, error) {
	explicit := len(in.MessageIDs) > 0 || len(in.ManualItemIDs) > 0
	items := make([]interpretItem, 0)

	if explicit {
		var mailAccount *uuid.UUID
		for _, mid := range in.MessageIDs {
			msg, err := s.Messages.GetMessage(ctx, userID, mid)
			if err != nil {
				return nil, nil, err
			}
			if msg == nil {
				continue
			}
			eff, err := s.Assignments.EffectiveAssignment(ctx, userID, mid)
			if err != nil {
				return nil, nil, err
			}
			if eff == nil || eff.ProjectID == nil || *eff.ProjectID != projectID {
				continue
			}
			if mailAccount == nil {
				aid := msg.AccountID
				mailAccount = &aid
			} else if msg.AccountID != *mailAccount {
				return nil, nil, ErrMixedAccounts
			}
			if in.AccountID != nil && msg.AccountID != *in.AccountID {
				return nil, nil, ErrMixedAccounts
			}
			title := msg.Subject
			snippet := ""
			if msg.BodyText != nil {
				snippet = *msg.BodyText
			}
			midCopy := mid
			aid := msg.AccountID
			items = append(items, interpretItem{
				Source: "mail", Title: title, Snippet: clampText(snippet, 400),
				MessageID: &midCopy, AccountID: &aid,
			})
		}
		for _, manID := range in.ManualItemIDs {
			man, err := s.Manuals.GetManualItem(ctx, orgID, manID)
			if err != nil {
				return nil, nil, err
			}
			if man == nil || man.ProjectID == nil || *man.ProjectID != projectID {
				continue
			}
			manCopy := manID
			items = append(items, interpretItem{
				Source: "manual", Title: man.Title, Snippet: clampText(man.BodyText, 400),
				ManualItemID: &manCopy,
			})
		}
		if in.AccountID != nil && mailAccount != nil && *mailAccount != *in.AccountID {
			return nil, nil, ErrMixedAccounts
		}
		return items, mailAccount, nil
	}

	if s.Timeline == nil {
		return nil, nil, ErrNothingToInterpret
	}
	timeline, err := s.Timeline.ListProjectTimeline(ctx, userID, orgID, projectID, driven.TimelineFilter{
		Source: "all", Limit: 40,
	})
	if err != nil {
		return nil, nil, err
	}
	var mailAccount *uuid.UUID
	mailByAccount := map[uuid.UUID][]interpretItem{}
	manuals := make([]interpretItem, 0)
	for _, it := range timeline {
		if it.Source == "manual" && it.ManualItemID != nil {
			manuals = append(manuals, interpretItem{
				Source: "manual", Title: it.Title, Snippet: clampText(it.Snippet, 400),
				ManualItemID: it.ManualItemID,
			})
			continue
		}
		if it.Source == "mail" && it.MessageID != nil && it.AccountID != nil {
			if in.AccountID != nil && *it.AccountID != *in.AccountID {
				continue
			}
			c := interpretItem{
				Source: "mail", Title: it.Title, Snippet: clampText(it.Snippet, 400),
				MessageID: it.MessageID, AccountID: it.AccountID,
			}
			mailByAccount[*it.AccountID] = append(mailByAccount[*it.AccountID], c)
		}
	}
	if in.AccountID != nil {
		items = append(items, manuals...)
		items = append(items, mailByAccount[*in.AccountID]...)
		return items, in.AccountID, nil
	}
	// Single-account rule: pick the account with the most mail; reject only if caller
	// passed mixed ids (handled above). Auto-pick does not error on multi-account timeline.
	best := -1
	var chosenAcc *uuid.UUID
	var chosenMail []interpretItem
	for accID, mail := range mailByAccount {
		if len(mail) > best {
			best = len(mail)
			aid := accID
			chosenAcc = &aid
			chosenMail = mail
		}
	}
	items = append(items, manuals...)
	items = append(items, chosenMail...)
	mailAccount = chosenAcc
	return items, mailAccount, nil
}

func (s *Service) loadActiveFactContext(ctx context.Context, orgID, projectID uuid.UUID) ([]string, error) {
	if s.Facts == nil {
		return nil, nil
	}
	facts, err := s.Facts.ListFactsByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, f := range facts {
		ver, err := s.Facts.GetActiveFactVersion(ctx, f.ID)
		if err != nil || ver == nil {
			continue
		}
		unit := ""
		if ver.Unit != nil {
			unit = *ver.Unit
		}
		out = append(out, fmt.Sprintf("%s (%s) = %s %s", f.SubjectKey, f.Label, ver.ValueText, unit))
	}
	return out, nil
}

func (s *Service) buildView(ctx context.Context, row driven.InterpretationRow) (*InterpretationView, error) {
	sources, err := s.Interpretations.ListInterpretationSources(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	var payload interpretLLMPayload
	_ = json.Unmarshal([]byte(row.PayloadJSON), &payload)
	cands := make([]CandidateView, 0, len(payload.Candidates))
	for _, c := range payload.Candidates {
		var value any
		if len(c.Value) > 0 {
			_ = json.Unmarshal(c.Value, &value)
		}
		cands = append(cands, CandidateView{
			Kind: c.Kind, SubjectKey: c.SubjectKey, Label: c.Label, Value: value,
			Unit: c.Unit, Statement: c.Statement, MessageIDs: c.MessageIDs,
			ManualItemIDs: c.ManualItemIDs, Confidence: c.Confidence, Reason: c.Reason,
		})
	}
	return &InterpretationView{Interpretation: row, Sources: sources, Candidates: cands}, nil
}

func buildInterpretPrompt(projectID uuid.UUID, name, code string, items []interpretItem, activeFacts []string) string {
	var b strings.Builder
	b.WriteString("Extract durable project fact and decision candidates from the correspondence below. ")
	b.WriteString("Respond with a single JSON object only: ")
	b.WriteString(`{"schema_version":1,"project_id":"` + projectID.String() + `","candidates":[{"kind":"fact|decision","subject_key":"a.b.c","label":"...","value":...,"unit":"","statement":"","message_ids":[],"manual_item_ids":[],"confidence":0..1,"reason":"..."}]}. `)
	b.WriteString("subject_key must be lowercase dotted (e.g. pump.p03.duty_kw). Only use message_ids/manual_item_ids from the list. ")
	b.WriteString("Prefer facts for measurable assertions and decisions for approvals/proceed-with language. ")
	b.WriteString("If nothing durable, return an empty candidates array.\n\n")
	b.WriteString("Project: " + name + " (" + code + ")\n")
	if len(activeFacts) > 0 {
		b.WriteString("\nKnown active facts:\n")
		for _, f := range activeFacts {
			b.WriteString("- " + f + "\n")
		}
	}
	b.WriteString("\nCorrespondence:\n")
	for i, it := range items {
		id := ""
		if it.MessageID != nil {
			id = "message_id=" + it.MessageID.String()
		} else if it.ManualItemID != nil {
			id = "manual_item_id=" + it.ManualItemID.String()
		}
		fmt.Fprintf(&b, "%d. [%s] %s | %s | %s\n", i+1, it.Source, id, clampText(it.Title, 120), clampText(it.Snippet, 300))
	}
	return b.String()
}

func (s *Service) callInterpretLLM(ctx context.Context, prompt string) (*interpretLLMPayload, error) {
	tryDecode := func(content string) (*interpretLLMPayload, error) {
		var out interpretLLMPayload
		if err := json.Unmarshal([]byte(normalizeJSON(content)), &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "You extract project facts and decisions from correspondence. Return JSON only."},
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

func sanitizeCandidates(
	raw []interpretCandidate,
	allowedMsg, allowedManual map[uuid.UUID]struct{},
	projectID uuid.UUID,
) []interpretCandidate {
	out := make([]interpretCandidate, 0, len(raw))
	for _, c := range raw {
		kind := strings.TrimSpace(strings.ToLower(c.Kind))
		if !domaininterp.CandidateKind(kind).Valid() {
			continue
		}
		c.Kind = kind
		c.SubjectKey = strings.TrimSpace(strings.ToLower(c.SubjectKey))
		c.Label = strings.TrimSpace(c.Label)
		c.Statement = strings.TrimSpace(c.Statement)
		c.Unit = strings.TrimSpace(c.Unit)
		c.Reason = strings.TrimSpace(c.Reason)
		if c.Confidence < 0 {
			c.Confidence = 0
		}
		if c.Confidence > 1 {
			c.Confidence = 1
		}
		if kind == string(domaininterp.KindFact) {
			if !domainfacts.ValidSubjectKey(c.SubjectKey) || c.Label == "" {
				continue
			}
		}
		if kind == string(domaininterp.KindDecision) && c.Statement == "" {
			continue
		}
		msgs := make([]string, 0)
		for _, idStr := range c.MessageIDs {
			id, err := uuid.Parse(strings.TrimSpace(idStr))
			if err != nil {
				continue
			}
			if _, ok := allowedMsg[id]; ok {
				msgs = append(msgs, id.String())
			}
		}
		mans := make([]string, 0)
		for _, idStr := range c.ManualItemIDs {
			id, err := uuid.Parse(strings.TrimSpace(idStr))
			if err != nil {
				continue
			}
			if _, ok := allowedManual[id]; ok {
				mans = append(mans, id.String())
			}
		}
		c.MessageIDs = msgs
		c.ManualItemIDs = mans
		_ = projectID
		out = append(out, c)
	}
	return out
}

func averageConfidence(cands []interpretCandidate) float64 {
	if len(cands) == 0 {
		return 0
	}
	var sum float64
	for _, c := range cands {
		sum += c.Confidence
	}
	return sum / float64(len(cands))
}

func summarizeReason(cands []interpretCandidate) string {
	if len(cands) == 0 {
		return "no durable candidates"
	}
	if r := strings.TrimSpace(cands[0].Reason); r != "" {
		return clampText(r, 240)
	}
	return fmt.Sprintf("%d candidate(s)", len(cands))
}

func clampText(s string, maxChars int) string {
	trimmed := strings.TrimSpace(s)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "…"
}

func normalizeJSON(s string) string {
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
