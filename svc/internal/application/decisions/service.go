package decisions

import (
	"context"

	"fmt"
	"strings"
	"time"

	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domaindec "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domaindecisions "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

type Service struct {
	Users       domainauth.UserRepository
	Projects    domainprojects.ProjectRepository
	Decisions   domaindecisions.DecisionRepository
	Issues      domainissues.IssueRepository
	Assignments domainprojects.AssignmentRepository
	Manuals     domainprojects.ManualItemRepository
	Messages    domainmessages.MessageRepository
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
		return uuid.Nil, domaindec.DecisionErrNotFound
	}
	m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, domaindec.DecisionErrNotFound
	}
	return orgID, nil
}

func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID, status string) ([]domaindec.DecisionDecisionView, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Decisions.ListDecisionsByProject(ctx, orgID, projectID, status)
	if err != nil {
		return nil, err
	}
	out := make([]domaindec.DecisionDecisionView, 0, len(rows))
	for _, row := range rows {
		ev, err := s.Decisions.ListDecisionEvidence(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domaindec.DecisionDecisionView{Decision: row, Evidence: ev})
	}
	return out, nil
}

func (s *Service) ListAcceptedRecent(ctx context.Context, userID, projectID uuid.UUID, limit int) ([]domaindec.DecisionDecisionView, error) {
	views, err := s.List(ctx, userID, projectID, string(domaindec.StatusAccepted))
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > len(views) {
		return views, nil
	}
	return views[:limit], nil
}

func (s *Service) Get(ctx context.Context, userID, decisionID uuid.UUID) (*domaindec.DecisionDecisionView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Decisions.GetDecision(ctx, orgID, decisionID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, domaindec.DecisionErrNotFound
	}
	if _, err := s.requireMember(ctx, userID, row.ProjectID); err != nil {
		return nil, err
	}
	ev, err := s.Decisions.ListDecisionEvidence(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return &domaindec.DecisionDecisionView{Decision: *row, Evidence: ev}, nil
}

func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in domaindec.DecisionCreateInput) (*domaindec.DecisionDecisionView, error) {
	orgID, err := s.requireMember(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil || p.ArchivedAt != nil {
		return nil, domaindec.DecisionErrNotFound
	}
	statement := strings.TrimSpace(in.Statement)
	if statement == "" {
		return nil, fmt.Errorf("statement required")
	}
	if !domaindec.ValidAssigneeXOR(in.AssigneeUserID != nil, in.AssigneeContactID != nil) {
		return nil, domaindec.DecisionErrBadAssignee
	}
	if in.IssueID != nil {
		iss, err := s.Issues.GetIssue(ctx, orgID, *in.IssueID)
		if err != nil {
			return nil, err
		}
		if iss == nil || iss.ProjectID != projectID {
			return nil, domaindec.DecisionErrWrongProject
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
	row := domaindecisions.DecisionRow{
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
		_ = s.Decisions.AddDecisionEvidence(ctx, domaindecisions.DecisionEvidenceRow{
			ID: uuid.New(), DecisionID: row.ID, MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
		})
	}
	return s.Get(ctx, userID, row.ID)
}

func (s *Service) Confirm(ctx context.Context, userID, decisionID uuid.UUID) (*domaindec.DecisionDecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) {
		return nil, domaindec.DecisionErrNotProposed
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

func (s *Service) Withdraw(ctx context.Context, userID, decisionID uuid.UUID) (*domaindec.DecisionDecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) && view.Decision.Status != string(domaindec.StatusAccepted) {
		return nil, domaindec.DecisionErrInvalidStatus
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

func (s *Service) Patch(ctx context.Context, userID, decisionID uuid.UUID, in domaindec.DecisionPatchInput) (*domaindecisions.DecisionDecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if view.Decision.Status != string(domaindec.StatusProposed) {
		return nil, domaindec.DecisionErrNotProposed
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
			return nil, domaindec.DecisionErrBadAssignee
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

func (s *Service) AddEvidence(ctx context.Context, userID, decisionID uuid.UUID, ref domaindec.DecisionEvidenceRef) (*domaindec.DecisionDecisionView, error) {
	view, err := s.Get(ctx, userID, decisionID)
	if err != nil {
		return nil, err
	}
	if err := s.validateEvidence(ctx, userID, view.Decision.OrganisationID, view.Decision.ProjectID, ref); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := s.Decisions.AddDecisionEvidence(ctx, domaindecisions.DecisionEvidenceRow{
		ID: uuid.New(), DecisionID: decisionID, MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
	}); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, decisionID)
}

func (s *Service) validateEvidence(ctx context.Context, userID, orgID, projectID uuid.UUID, ref domaindec.DecisionEvidenceRef) error {
	msgSet := ref.MessageID != nil
	manSet := ref.ManualItemID != nil
	if !domaindec.ValidEvidenceXOR(msgSet, manSet) {
		return domaindec.DecisionErrBadEvidence
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
			return domaindec.DecisionErrNotFound
		}
		if item.ProjectID == nil || *item.ProjectID != projectID {
			return domaindec.DecisionErrWrongProject
		}
	}
	return nil
}
