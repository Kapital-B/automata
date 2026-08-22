package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaincontr "github.com/Kapital-B/automata/svc/internal/domain/contradictions"
	domaindec "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domaininterp "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
	"github.com/google/uuid"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrNothingToReconcile = errors.New("no pending interpretations to reconcile")
	ErrInvalidResolution  = errors.New("invalid resolution")
	ErrNotOpen            = errors.New("contradiction is not open")
)

// Service applies Stage B reconcile outcomes and manages contradictions.
type Service struct {
	Users           driven.UserRepository
	Projects        driven.ProjectRepository
	Interpretations driven.InterpretationRepository
	FactsRepo       driven.FactRepository
	Facts           *appfacts.Service
	Decisions       *appdecisions.Service
	Contradictions  driven.ContradictionRepository
	JobRuns         driven.JobRunRepository
}

type ReconcileInput struct {
	InterpretationIDs []uuid.UUID // empty = all pending
}

type 	CandidateOutcome struct {
	Kind            string `json:"kind"`
	Outcome         string `json:"outcome"`
	Subject         string `json:"subject_key,omitempty"`
	Reason          string `json:"reason"`
	FactID          string `json:"fact_id,omitempty"`
	VersionID       string `json:"version_id,omitempty"`
	DecisionID      string `json:"decision_id,omitempty"`
	ContradictionID string `json:"contradiction_id,omitempty"`
}

type ReconcileResult struct {
	ProcessedInterpretations int                `json:"processed_interpretations"`
	Outcomes                 []CandidateOutcome `json:"outcomes"`
	ContradictionsOpened     int                `json:"contradictions_opened"`
}

type ContradictionView struct {
	Contradiction driven.ContradictionRow
	Sides         []driven.ContradictionSideRow
}

type ResolveInput struct {
	Resolution          string
	KeepFactVersionID   *uuid.UUID
	RejectFactVersionID *uuid.UUID
	ResolutionNote      string
}

type candidatePayload struct {
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

type payloadRoot struct {
	Candidates []candidatePayload `json:"candidates"`
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

func (s *Service) requireMember(ctx context.Context, userID, projectID uuid.UUID) (uuid.UUID, error) {
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
	m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, ErrNotFound
	}
	return orgID, nil
}

func (s *Service) Run(ctx context.Context, userID, projectID uuid.UUID, in ReconcileInput) (*ReconcileResult, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	pending, err := s.Interpretations.ListPendingInterpretations(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	selected := make([]driven.InterpretationRow, 0)
	if len(in.InterpretationIDs) == 0 {
		selected = pending
	} else {
		want := map[uuid.UUID]struct{}{}
		for _, id := range in.InterpretationIDs {
			want[id] = struct{}{}
		}
		for _, row := range pending {
			if _, ok := want[row.ID]; ok {
				selected = append(selected, row)
			}
		}
	}
	if len(selected) == 0 {
		return nil, ErrNothingToReconcile
	}

	now := time.Now().UTC()
	runID := uuid.New()
	if s.JobRuns != nil {
		meta, _ := json.Marshal(map[string]any{"project_id": projectID.String(), "count": len(selected)})
		_ = s.JobRuns.InsertJobRun(ctx, runID, uuid.Nil, "reconcile_project", "api", "success", now, now, nil, string(meta))
	}

	result := &ReconcileResult{Outcomes: make([]CandidateOutcome, 0)}
	for _, interp := range selected {
		outs, opened, err := s.reconcileOne(ctx, userID, orgID, projectID, interp)
		if err != nil {
			return nil, err
		}
		result.Outcomes = append(result.Outcomes, outs...)
		result.ContradictionsOpened += opened
		_ = s.Interpretations.UpdateInterpretationStatus(ctx, orgID, interp.ID, string(domaininterp.StatusAccepted), now)
		result.ProcessedInterpretations++
	}
	return result, nil
}

func (s *Service) reconcileOne(ctx context.Context, userID, orgID, projectID uuid.UUID, interp driven.InterpretationRow) ([]CandidateOutcome, int, error) {
	var root payloadRoot
	_ = json.Unmarshal([]byte(interp.PayloadJSON), &root)
	outs := make([]CandidateOutcome, 0, len(root.Candidates))
	opened := 0
	for _, c := range root.Candidates {
		kind := strings.ToLower(strings.TrimSpace(c.Kind))
		switch kind {
		case string(domaininterp.KindFact):
			out, didOpen, err := s.reconcileFactCandidate(ctx, userID, orgID, projectID, interp.ID, c)
			if err != nil {
				return nil, 0, err
			}
			outs = append(outs, out)
			if didOpen {
				opened++
			}
		case string(domaininterp.KindDecision):
			out, err := s.reconcileDecisionCandidate(ctx, userID, projectID, c)
			if err != nil {
				return nil, 0, err
			}
			outs = append(outs, out)
		default:
			outs = append(outs, CandidateOutcome{Kind: kind, Outcome: "ignore", Reason: "unknown candidate kind"})
		}
	}
	return outs, opened, nil
}

func (s *Service) reconcileDecisionCandidate(ctx context.Context, userID, projectID uuid.UUID, c candidatePayload) (CandidateOutcome, error) {
	if s.Decisions == nil {
		return CandidateOutcome{Kind: "decision", Outcome: "ignore", Reason: "decisions not configured"}, nil
	}
	statement := strings.TrimSpace(c.Statement)
	if statement == "" {
		return CandidateOutcome{Kind: "decision", Outcome: "ignore", Reason: "empty statement"}, nil
	}
	norm := normalizeValue(statement)
	existing, err := s.Decisions.List(ctx, userID, projectID, string(domaindec.StatusAccepted))
	if err != nil {
		return CandidateOutcome{}, err
	}
	for _, v := range existing {
		if normalizeValue(v.Decision.Statement) == norm {
			for _, ref := range evidenceFromCandidate(c) {
				_, _ = s.Decisions.AddEvidence(ctx, userID, v.Decision.ID, appdecisions.EvidenceRef{
					MessageID: ref.MessageID, ManualItemID: ref.ManualItemID,
				})
			}
			return CandidateOutcome{
				Kind: "decision", Outcome: "reinforce", Reason: "compatible with accepted decision",
				DecisionID: v.Decision.ID.String(),
			}, nil
		}
	}
	evidence := make([]appdecisions.EvidenceRef, 0)
	for _, ref := range evidenceFromCandidate(c) {
		evidence = append(evidence, appdecisions.EvidenceRef{MessageID: ref.MessageID, ManualItemID: ref.ManualItemID})
	}
	view, err := s.Decisions.Create(ctx, userID, projectID, appdecisions.CreateInput{
		Statement: statement, Confirm: false, Evidence: evidence,
		Source: string(domaindec.SourceLLM), Confidence: &c.Confidence,
	})
	if err != nil {
		return CandidateOutcome{}, err
	}
	return CandidateOutcome{
		Kind: "decision", Outcome: "confirm_new", Reason: "proposed decision from interpretation",
		DecisionID: view.Decision.ID.String(),
	}, nil
}

func (s *Service) reconcileFactCandidate(ctx context.Context, userID, orgID, projectID, interpID uuid.UUID, c candidatePayload) (CandidateOutcome, bool, error) {
	subject := strings.TrimSpace(strings.ToLower(c.SubjectKey))
	label := strings.TrimSpace(c.Label)
	if !domainfacts.ValidSubjectKey(subject) || label == "" {
		return CandidateOutcome{Kind: "fact", Outcome: "ignore", Reason: "invalid subject/label"}, false, nil
	}
	var value any
	if len(c.Value) > 0 {
		_ = json.Unmarshal(c.Value, &value)
	}
	evidence := evidenceFromCandidate(c)
	conf := c.Confidence
	unit := strings.TrimSpace(c.Unit)
	var unitPtr *string
	if unit != "" {
		unitPtr = &unit
	}

	existing, err := s.FactsRepo.GetFactBySubject(ctx, orgID, projectID, subject)
	if err != nil {
		return CandidateOutcome{}, false, err
	}
	if existing == nil {
		view, err := s.Facts.Create(ctx, userID, projectID, appfacts.CreateInput{
			SubjectKey: subject, Label: label, Value: value, Unit: unitPtr,
			Confirm: true, Evidence: evidence, Source: string(domainfacts.SourceLLM),
			InterpretationID: &interpID, Confidence: &conf,
		})
		if err != nil {
			return CandidateOutcome{}, false, err
		}
		vid := ""
		if len(view.Versions) > 0 {
			vid = view.Versions[len(view.Versions)-1].Version.ID.String()
		}
		return CandidateOutcome{
			Kind: "fact", Outcome: "confirm_new", Subject: subject, Reason: "no prior subject",
			FactID: view.Fact.ID.String(), VersionID: vid,
		}, false, nil
	}

	active, err := s.FactsRepo.GetActiveFactVersion(ctx, existing.ID)
	if err != nil {
		return CandidateOutcome{}, false, err
	}
	if active == nil {
		view, err := s.Facts.Create(ctx, userID, projectID, appfacts.CreateInput{
			SubjectKey: subject, Label: label, Value: value, Unit: unitPtr,
			Confirm: true, Evidence: evidence, Source: string(domainfacts.SourceLLM),
			InterpretationID: &interpID, Confidence: &conf,
		})
		if err != nil {
			return CandidateOutcome{}, false, err
		}
		vid := ""
		if len(view.Versions) > 0 {
			vid = view.Versions[len(view.Versions)-1].Version.ID.String()
		}
		return CandidateOutcome{
			Kind: "fact", Outcome: "confirm_new", Subject: subject, Reason: "no active version",
			FactID: view.Fact.ID.String(), VersionID: vid,
		}, false, nil
	}

	candText := valueText(value, unit)
	if valuesCompatible(active.ValueText, candText) {
		for _, ref := range evidence {
			_, _ = s.Facts.AddEvidence(ctx, userID, active.ID, ref)
		}
		return CandidateOutcome{
			Kind: "fact", Outcome: "reinforce", Subject: subject, Reason: "compatible with active value",
			FactID: existing.ID.String(), VersionID: active.ID.String(),
		}, false, nil
	}

	// Incompatible: high confidence → propose supersede; low → contradiction.
	view, err := s.Facts.Create(ctx, userID, projectID, appfacts.CreateInput{
		SubjectKey: subject, Label: label, Value: value, Unit: unitPtr,
		Confirm: false, SupersedesVersionID: &active.ID, Evidence: evidence,
		Source: string(domainfacts.SourceLLM), InterpretationID: &interpID, Confidence: &conf,
	})
	if err != nil {
		return CandidateOutcome{}, false, err
	}
	var proposedID uuid.UUID
	for _, v := range view.Versions {
		if v.Version.Status == string(domainfacts.StatusProposed) {
			proposedID = v.Version.ID
		}
	}
	if conf >= 0.7 {
		return CandidateOutcome{
			Kind: "fact", Outcome: "supersede", Subject: subject,
			Reason: "incompatible value; proposed supersession",
			FactID: existing.ID.String(), VersionID: proposedID.String(),
		}, false, nil
	}

	now := time.Now().UTC()
	contr := driven.ContradictionRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID,
		Status: string(domaincontr.StatusOpen),
		Summary: fmt.Sprintf("%s: active %q vs proposed %q", subject, active.ValueText, candText),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Contradictions.CreateContradiction(ctx, contr); err != nil {
		return CandidateOutcome{}, false, err
	}
	_ = s.Contradictions.AddContradictionSide(ctx, driven.ContradictionSideRow{
		ID: uuid.New(), ContradictionID: contr.ID, FactVersionID: &active.ID,
	})
	_ = s.Contradictions.AddContradictionSide(ctx, driven.ContradictionSideRow{
		ID: uuid.New(), ContradictionID: contr.ID, FactVersionID: &proposedID,
	})
	return CandidateOutcome{
		Kind: "fact", Outcome: "contradiction", Subject: subject,
		Reason: "competing values without safe supersede",
		FactID: existing.ID.String(), VersionID: proposedID.String(),
		ContradictionID: contr.ID.String(),
	}, true, nil
}

func (s *Service) ListContradictions(ctx context.Context, userID, projectID uuid.UUID, status string) ([]ContradictionView, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Contradictions.ListContradictionsByProject(ctx, orgID, projectID, status)
	if err != nil {
		return nil, err
	}
	out := make([]ContradictionView, 0, len(rows))
	for _, row := range rows {
		sides, err := s.Contradictions.ListContradictionSides(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ContradictionView{Contradiction: row, Sides: sides})
	}
	return out, nil
}

func (s *Service) Resolve(ctx context.Context, userID, contradictionID uuid.UUID, in ResolveInput) (*ContradictionView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Contradictions.GetContradiction(ctx, orgID, contradictionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireMember(ctx, userID, row.ProjectID); err != nil {
		return nil, err
	}
	if row.Status != string(domaincontr.StatusOpen) {
		return nil, ErrNotOpen
	}
	res := domaincontr.Resolution(strings.TrimSpace(in.Resolution))
	if !res.Valid() {
		return nil, ErrInvalidResolution
	}
	sides, err := s.Contradictions.ListContradictionSides(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	switch res {
	case domaincontr.ResolutionSupersede:
		keep := in.KeepFactVersionID
		if keep == nil && len(sides) >= 2 {
			// Prefer second side (proposed) if present.
			if sides[1].FactVersionID != nil {
				keep = sides[1].FactVersionID
			} else if sides[0].FactVersionID != nil {
				keep = sides[0].FactVersionID
			}
		}
		if keep == nil {
			return nil, fmt.Errorf("keep_fact_version_id required")
		}
		var reject *uuid.UUID
		for _, side := range sides {
			if side.FactVersionID != nil && *side.FactVersionID != *keep {
				reject = side.FactVersionID
			}
		}
		_, err := s.Facts.Confirm(ctx, userID, *keep, appfacts.ConfirmInput{SupersedesVersionID: reject})
		if err != nil {
			return nil, err
		}
		if reject != nil {
			ver, _ := s.FactsRepo.GetFactVersion(ctx, orgID, *reject)
			if ver != nil && ver.Status == string(domainfacts.StatusProposed) {
				_, _ = s.Facts.Reject(ctx, userID, *reject)
			}
		}
	case domaincontr.ResolutionRejectA, domaincontr.ResolutionRejectB:
		idx := 0
		if res == domaincontr.ResolutionRejectB {
			idx = 1
		}
		if idx >= len(sides) || sides[idx].FactVersionID == nil {
			return nil, fmt.Errorf("side not found")
		}
		ver, err := s.FactsRepo.GetFactVersion(ctx, orgID, *sides[idx].FactVersionID)
		if err != nil {
			return nil, err
		}
		if ver != nil && ver.Status == string(domainfacts.StatusProposed) {
			_, _ = s.Facts.Reject(ctx, userID, ver.ID)
		}
	case domaincontr.ResolutionNote:
		// close with note only
	}

	note := strings.TrimSpace(in.ResolutionNote)
	row.Status = string(domaincontr.StatusResolved)
	row.ResolvedAt = &now
	uid := userID
	row.ResolvedByUserID = &uid
	if note != "" {
		row.ResolutionNote = &note
	} else {
		n := string(res)
		row.ResolutionNote = &n
	}
	row.UpdatedAt = now
	if err := s.Contradictions.UpdateContradiction(ctx, *row); err != nil {
		return nil, err
	}
	sides, _ = s.Contradictions.ListContradictionSides(ctx, row.ID)
	return &ContradictionView{Contradiction: *row, Sides: sides}, nil
}

func evidenceFromCandidate(c candidatePayload) []appfacts.EvidenceRef {
	out := make([]appfacts.EvidenceRef, 0)
	for _, idStr := range c.MessageIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			continue
		}
		mid := id
		out = append(out, appfacts.EvidenceRef{MessageID: &mid})
	}
	for _, idStr := range c.ManualItemIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			continue
		}
		mid := id
		out = append(out, appfacts.EvidenceRef{ManualItemID: &mid})
	}
	return out
}

func valueText(v any, unit string) string {
	text := ""
	switch t := v.(type) {
	case nil:
		text = ""
	case string:
		text = t
	case float64:
		text = strconv.FormatFloat(t, 'g', -1, 64)
	case json.Number:
		text = t.String()
	case map[string]any:
		if amount, ok := t["amount"]; ok {
			text = fmt.Sprintf("%v", amount)
			if u, ok := t["unit"].(string); ok && u != "" {
				unit = u
			}
		} else {
			b, _ := json.Marshal(t)
			text = string(b)
		}
	default:
		b, _ := json.Marshal(t)
		text = string(b)
	}
	text = strings.TrimSpace(text)
	if unit != "" && !strings.Contains(text, unit) {
		return text + " " + unit
	}
	return text
}

func valuesCompatible(a, b string) bool {
	na := normalizeValue(a)
	nb := normalizeValue(b)
	if na == nb {
		return true
	}
	fa, ea := strconv.ParseFloat(stripUnit(na), 64)
	fb, eb := strconv.ParseFloat(stripUnit(nb), 64)
	if ea == nil && eb == nil {
		return fa == fb
	}
	return false
}

func normalizeValue(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func stripUnit(s string) string {
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return s
	}
	return parts[0]
}
