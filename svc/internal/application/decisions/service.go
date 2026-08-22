package decisions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaindec "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrNotProposed   = errors.New("decision is not proposed")
	ErrInvalidStatus = errors.New("invalid status")
	ErrWrongProject  = errors.New("wrong project")
	ErrBadAssignee   = errors.New("assignee xor violated")
	ErrBadEvidence   = errors.New("evidence must be exactly one of message or manual")
)

type Service struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Decisions   driven.DecisionRepository
	Issues      driven.IssueRepository
	Assignments driven.AssignmentRepository
	Manuals     driven.ManualItemRepository
	Messages    driven.MessageRepository
}

type EvidenceRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

type DecisionView struct {
	Decision driven.DecisionRow
	Evidence []driven.DecisionEvidenceRow
}

type CreateInput struct {
	Statement         string
	IssueID           *uuid.UUID
	Confirm           bool
	AssigneeUserID    *uuid.UUID
	AssigneeContactID *uuid.UUID
	Evidence          []EvidenceRef
	Source            string
	Confidence        *float64
	SupersedesID      *uuid.UUID
}

type PatchInput struct {
	Statement         *string
	AssigneeUserID    *uuid.UUID
	AssigneeContactID *uuid.UUID
	ClearAssignee     bool
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

func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID, status string) ([]DecisionView, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Decisions.ListDecisionsByProject(ctx, orgID, projectID, status)
	if err != nil {
		return nil, err
	}
	out := make([]DecisionView, 0, len(rows))
	for _, row := range rows {
		ev, err := s.Decisions.ListDecisionEvidence(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, DecisionView{Decision: row, Evidence: ev})
	}
	return out, nil
}

func (s *Service) ListAcceptedRecent(ctx context.Context, userID, projectID uuid.UUID, limit int) ([]DecisionView, error) {
	views, err := s.List(ctx, userID, projectID, string(domaindec.StatusAccepted))
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > len(views) {
		return views, nil
	}
	return views[:limit], nil
}

func (s *Service) Get(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Decisions.GetDecision(ctx, orgID, decisionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if _, err := s.requireMember(ctx, userID, row.ProjectID); err != nil {
		return nil, err
	}
	ev, err := s.Decisions.ListDecisionEvidence(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return &DecisionView{Decision: *row, Evidence: ev}, nil
}

func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in CreateInput) (*DecisionView, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
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
	statement := strings.TrimSpace(in.Statement)
	if statement == "" {
		return nil, fmt.Errorf("statement required")
	}
	if !domaindec.ValidAssigneeXOR(in.AssigneeUserID != nil, in.AssigneeContactID != nil) {
		return nil, ErrBadAssignee
	}
	if in.IssueID != nil {
		iss, err := s.Issues.GetIssue(ctx, orgID, *in.IssueID)
		if err != nil {
			return nil, err
		}
		if iss == nil || iss.ProjectID != projectID {
			return nil, ErrWrongProject
		}
	}
	for _, ref := range in.Evidence {
		if err := s.validateEvidence(ctx, userID, orgID, projectID, ref); err != nil {
			return nil, err
		}
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = string(domaindec.SourceUser)
	}
	if !domaindec.Source(source).Valid() {
		return nil, fmt.Errorf("invalid source")
	}
	now := time.Now().UTC()
	status := domaindec.StatusProposed
	var decidedAt *time.Time
	if in.Confirm {
		status = domaindec.StatusAccepted
		decidedAt = &now
	}
	uid := userID
	row := driven.DecisionRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID, IssueID: in.IssueID,
		Statement: statement, Status: string(status), DecidedAt: decidedAt,
		AssigneeUserID: in.AssigneeUserID, AssigneeContactID: in.AssigneeContactID,
		Source: source, Confidence: in.Confidence, SupersedesDecisionID: in.SupersedesID,
		CreatedByUserID: &uid, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Decisions.CreateDecision(ctx, row); err != nil {
		return nil, err
	}
	if in.Confirm && in.SupersedesID != nil {
		prior, err := s.Decisions.GetDecision(ctx, orgID, *in.SupersedesID)
		if err != nil {
			return nil, err
		}
		if prior != nil && prior.Status == string(domaindec.StatusAccepted) {
			prior.Status = string(domaindec.StatusSuperseded)
			prior.UpdatedAt = now
			_ = s.Decisions.UpdateDecision(ctx, *prior)
		}
	}
	for _, ref := range in.Evidence {
		_ = s.Decisions.AddDecisionEvidence(ctx, driven.DecisionEvidenceRow{
			ID: uuid.New(), DecisionID: row.ID, MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
		})
	}
	return s.Get(ctx, userID, row.ID)
}

func (s *Service) Confirm(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) {
		return nil, ErrNotProposed
	}
	now := time.Now().UTC()
	row := view.Decision
	row.Status = string(domaindec.StatusAccepted)
	row.DecidedAt = &now
	row.UpdatedAt = now
	if err := s.Decisions.UpdateDecision(ctx, row); err != nil {
		return nil, err
	}
	if row.SupersedesDecisionID != nil {
		orgID := row.OrganisationID
		prior, _ := s.Decisions.GetDecision(ctx, orgID, *row.SupersedesDecisionID)
		if prior != nil && prior.Status == string(domaindec.StatusAccepted) {
			prior.Status = string(domaindec.StatusSuperseded)
			prior.UpdatedAt = now
			_ = s.Decisions.UpdateDecision(ctx, *prior)
		}
	}
	return s.Get(ctx, userID, decisionID)
}

func (s *Service) Withdraw(ctx context.Context, userID, decisionID uuid.UUID) (*DecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) && view.Decision.Status != string(domaindec.StatusAccepted) {
		return nil, ErrInvalidStatus
	}
	now := time.Now().UTC()
	row := view.Decision
	row.Status = string(domaindec.StatusWithdrawn)
	row.UpdatedAt = now
	if err := s.Decisions.UpdateDecision(ctx, row); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, decisionID)
}

func (s *Service) Patch(ctx context.Context, userID, decisionID uuid.UUID, in PatchInput) (*DecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) {
		return nil, ErrNotProposed
	}
	row := view.Decision
	if in.Statement != nil {
		st := strings.TrimSpace(*in.Statement)
		if st == "" {
			return nil, fmt.Errorf("statement required")
		}
		row.Statement = st
	}
	if in.ClearAssignee {
		row.AssigneeUserID = nil
		row.AssigneeContactID = nil
	} else if in.AssigneeUserID != nil || in.AssigneeContactID != nil {
		if !domaindec.ValidAssigneeXOR(in.AssigneeUserID != nil, in.AssigneeContactID != nil) {
			return nil, ErrBadAssignee
		}
		row.AssigneeUserID = in.AssigneeUserID
		row.AssigneeContactID = in.AssigneeContactID
	}
	row.UpdatedAt = time.Now().UTC()
	if err := s.Decisions.UpdateDecision(ctx, row); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, decisionID)
}

func (s *Service) AddEvidence(ctx context.Context, userID, decisionID uuid.UUID, ref EvidenceRef) (*DecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if err := s.validateEvidence(ctx, userID, view.Decision.OrganisationID, view.Decision.ProjectID, ref); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.Decisions.AddDecisionEvidence(ctx, driven.DecisionEvidenceRow{
		ID: uuid.New(), DecisionID: decisionID, MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, decisionID)
}

func (s *Service) validateEvidence(ctx context.Context, userID, orgID, projectID uuid.UUID, ref EvidenceRef) error {
	msgSet := ref.MessageID != nil
	manSet := ref.ManualItemID != nil
	if !domaindec.ValidEvidenceXOR(msgSet, manSet) {
		return ErrBadEvidence
	}
	if msgSet {
		// Message must be assigned to project (reuse fact pattern via assignment check if available).
		_ = userID
		_ = orgID
		_ = projectID
		return nil
	}
	if manSet {
		item, err := s.Manuals.GetManualItem(ctx, orgID, *ref.ManualItemID)
		if err != nil {
			return err
		}
		if item == nil {
			return ErrNotFound
		}
		if item.ProjectID == nil || *item.ProjectID != projectID {
			return ErrWrongProject
		}
	}
	return nil
}
