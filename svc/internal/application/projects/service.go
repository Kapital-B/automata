package projects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Kapital-B/automata/svc/internal/application/jobkit"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/contacts"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidCode   = errors.New("invalid project code")
	ErrCodeTaken     = errors.New("project code already exists")
	ErrCodeImmutable = errors.New("project code is immutable")
)

// Service handles project CRUD and manual assignment.
type Service struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Assignments driven.AssignmentRepository
	Manuals     driven.ManualItemRepository
	Timeline    driven.TimelineRepository
	Contacts    driven.ContactRepository
	Messages    driven.MessageRepository
	// AfterProjectCorrespondence is optional best-effort hook (e.g. interpret) after
	// committed paste/assign. Must not fail the caller.
	AfterProjectCorrespondence func(ctx context.Context, userID, projectID uuid.UUID, messageID, manualItemID *uuid.UUID)
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, filter driven.ProjectListFilter) ([]driven.ProjectRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.Projects.ListProjects(ctx, orgID, filter)
}

type ProjectDetail struct {
	Project driven.ProjectRow
	Member  *driven.ProjectMemberRow
}

func (s *Service) Get(ctx context.Context, userID, projectID uuid.UUID) (*ProjectDetail, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	return &ProjectDetail{Project: *p, Member: m}, nil
}

type CreateProjectInput struct {
	Name              string
	Code              string
	Description       *string
	Client            *string
	Keywords          []string
	MemberRole        string
	Discipline        *string
	Responsibilities  *string
	CurrentScope      *string
	ApprovalAuthority *string
	OutOfScope        *string
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateProjectInput) (*driven.ProjectRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	code := domainprojects.NormalizeCode(in.Code)
	if !domainprojects.ValidCode(code) {
		return nil, ErrInvalidCode
	}
	if existing, err := s.Projects.GetProjectByCode(ctx, orgID, code); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrCodeTaken
	}
	now := time.Now().UTC()
	id := uuid.New()
	project := driven.ProjectRow{
		ID: id, OrganisationID: orgID, Name: strings.TrimSpace(in.Name), Code: code,
		Description: in.Description, Client: in.Client, Keywords: in.Keywords,
		CreatedAt: now, UpdatedAt: now,
	}
	if project.Name == "" {
		project.Name = code
	}
	if project.Keywords == nil {
		project.Keywords = []string{}
	}
	member := driven.ProjectMemberRow{
		ID: uuid.New(), ProjectID: id, UserID: userID,
		Role:       strings.TrimSpace(in.MemberRole),
		Discipline: in.Discipline, Responsibilities: in.Responsibilities,
		CurrentScope: in.CurrentScope, ApprovalAuthority: in.ApprovalAuthority,
		OutOfScope: in.OutOfScope, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Projects.CreateProject(ctx, project, member); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrCodeTaken
		}
		return nil, err
	}
	return &project, nil
}

type UpdateProjectInput struct {
	Name        *string
	Description *string
	Client      *string
	Keywords    *[]string
	Archive     *bool
}

func (s *Service) Update(ctx context.Context, userID, projectID uuid.UUID, in UpdateProjectInput) (*driven.ProjectRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
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
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		p.Description = in.Description
	}
	if in.Client != nil {
		p.Client = in.Client
	}
	if in.Keywords != nil {
		p.Keywords = *in.Keywords
	}
	if in.Archive != nil {
		if *in.Archive {
			now := time.Now().UTC()
			p.ArchivedAt = &now
		} else {
			p.ArchivedAt = nil
		}
	}
	p.UpdatedAt = time.Now().UTC()
	if err := s.Projects.UpdateProject(ctx, *p); err != nil {
		return nil, err
	}
	return p, nil
}

type UpdateMemberInput struct {
	Role              *string
	Discipline        *string
	Responsibilities  *string
	CurrentScope      *string
	ApprovalAuthority *string
	OutOfScope        *string
}

func (s *Service) UpdateMember(ctx context.Context, userID, projectID uuid.UUID, in UpdateMemberInput) (*driven.ProjectMemberRow, error) {
	orgID, err := s.homeOrg(ctx, userID)
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
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	if in.Role != nil {
		m.Role = strings.TrimSpace(*in.Role)
	}
	if in.Discipline != nil {
		m.Discipline = in.Discipline
	}
	if in.Responsibilities != nil {
		m.Responsibilities = in.Responsibilities
	}
	if in.CurrentScope != nil {
		m.CurrentScope = in.CurrentScope
	}
	if in.ApprovalAuthority != nil {
		m.ApprovalAuthority = in.ApprovalAuthority
	}
	if in.OutOfScope != nil {
		m.OutOfScope = in.OutOfScope
	}
	m.UpdatedAt = time.Now().UTC()
	if err := s.Projects.UpdateProjectMember(ctx, *m); err != nil {
		return nil, err
	}
	return m, nil
}

type AssignInput struct {
	ProjectID *uuid.UUID
	Scope     domainprojects.AssignScope
	Status    domainprojects.AssignmentStatus
}

func (s *Service) AssignMessage(ctx context.Context, userID, messageID uuid.UUID, in AssignInput) (*driven.EffectiveAssignment, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	msg, err := s.Messages.GetMessage(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNotFound
	}
	scope := in.Scope
	if scope == "" {
		scope = domainprojects.ScopeThread
	}
	if !scope.Valid() {
		return nil, fmt.Errorf("invalid scope")
	}
	status := in.Status
	if status == "" {
		status = domainprojects.StatusCommitted
	}
	if !status.Valid() {
		return nil, fmt.Errorf("invalid status")
	}
	if in.ProjectID != nil {
		p, err := s.Projects.GetProject(ctx, orgID, *in.ProjectID)
		if err != nil {
			return nil, err
		}
		if p == nil || p.ArchivedAt != nil {
			return nil, ErrNotFound
		}
	}
	now := time.Now().UTC()
	uid := userID

	// A message with no conversation has no thread to file; fall back to the
	// only scope that can express the request rather than refusing it.
	if scope == domainprojects.ScopeThread && !hasConversation(msg) {
		scope = domainprojects.ScopeMessage
	}

	switch scope {
	case domainprojects.ScopeThread:
		if in.ProjectID == nil {
			if err := s.Assignments.DeleteThreadAssignment(ctx, msg.AccountID, *msg.ConversationID); err != nil {
				return nil, err
			}
		} else {
			row := driven.AssignmentRow{
				ID: uuid.New(), OrganisationID: orgID, AccountID: msg.AccountID,
				ConversationID: *msg.ConversationID, ProjectID: in.ProjectID,
				Status: string(status), Reason: "user_assign", Source: string(domainprojects.SourceUser),
				AssignedByUserID: &uid, CreatedAt: now, UpdatedAt: now,
			}
			if err := s.Assignments.UpsertThreadAssignment(ctx, row); err != nil {
				return nil, err
			}
		}
	case domainprojects.ScopeMessage:
		row := driven.AssignmentRow{
			OrganisationID: orgID, AccountID: msg.AccountID, MessageID: &messageID,
			ProjectID: in.ProjectID, Status: string(status), Reason: "user_assign",
			Source: string(domainprojects.SourceUser), AssignedByUserID: &uid,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.Assignments.UpsertMessageOverride(ctx, row); err != nil {
			return nil, err
		}
	}

	if status == domainprojects.StatusCommitted && in.ProjectID != nil {
		_ = s.upsertParticipantsFromMessage(ctx, orgID, *in.ProjectID, msg)
		if s.AfterProjectCorrespondence != nil {
			mid := messageID
			pid := *in.ProjectID
			s.AfterProjectCorrespondence(ctx, userID, pid, &mid, nil)
		}
	}
	return s.Assignments.EffectiveAssignment(ctx, userID, messageID)
}

func (s *Service) ClearOverride(ctx context.Context, userID, messageID uuid.UUID) (*driven.EffectiveAssignment, error) {
	msg, err := s.Messages.GetMessage(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNotFound
	}
	if err := s.Assignments.DeleteMessageOverride(ctx, messageID); err != nil {
		return nil, err
	}
	return s.Assignments.EffectiveAssignment(ctx, userID, messageID)
}

func (s *Service) ListUnassigned(ctx context.Context, userID uuid.UUID, filter driven.UnassignedListFilter) ([]driven.UnassignedItem, error) {
	return s.Assignments.ListUnassigned(ctx, userID, filter)
}

func (s *Service) UnassignedSummary(ctx context.Context, userID uuid.UUID) (driven.UnassignedSummary, error) {
	return s.Assignments.CountUnassignedSummary(ctx, userID)
}

func (s *Service) EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*driven.EffectiveAssignment, error) {
	return s.Assignments.EffectiveAssignment(ctx, userID, messageID)
}

func (s *Service) upsertParticipantsFromMessage(ctx context.Context, orgID, projectID uuid.UUID, msg *driven.MessageRow) error {
	now := time.Now().UTC()
	contactIDs, err := s.Contacts.ListContactIDsForMessage(ctx, orgID, msg.ID)
	if err != nil {
		return err
	}
	if msg.ConversationID != nil && strings.TrimSpace(*msg.ConversationID) != "" {
		threadIDs, err := s.Contacts.ListContactIDsForThread(ctx, orgID, msg.AccountID, *msg.ConversationID)
		if err == nil {
			contactIDs = append(contactIDs, threadIDs...)
		}
	}
	seen := map[uuid.UUID]struct{}{}
	for _, cid := range contactIDs {
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		_ = s.Projects.UpsertProjectParticipant(ctx, projectID, cid, now)
	}
	return nil
}

var (
	ErrInvalidChannel = errors.New("invalid channel")
	ErrInvalidBody    = errors.New("body_text required")
)

type CreateManualInput struct {
	Channel               string
	OccurredAt            time.Time
	Title                 string
	BodyText              string
	ProjectID             *uuid.UUID
	ParticipantContactIDs []uuid.UUID
}

func (s *Service) CreateManualItem(ctx context.Context, userID uuid.UUID, in CreateManualInput) (*driven.ManualItemRow, error) {
	if s.Manuals == nil {
		return nil, fmt.Errorf("manuals not configured")
	}
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	ch := domainprojects.ManualChannel(strings.TrimSpace(strings.ToLower(in.Channel)))
	if !ch.Valid() {
		return nil, ErrInvalidChannel
	}
	body := strings.TrimSpace(in.BodyText)
	if body == "" {
		return nil, ErrInvalidBody
	}
	if in.OccurredAt.IsZero() {
		return nil, fmt.Errorf("occurred_at required")
	}
	if in.ProjectID != nil {
		p, err := s.Projects.GetProject(ctx, orgID, *in.ProjectID)
		if err != nil {
			return nil, err
		}
		if p == nil || p.ArchivedAt != nil {
			return nil, ErrNotFound
		}
	}
	now := time.Now().UTC()
	status := "unassigned"
	src := string(domainprojects.SourceUser)
	reason := "user_paste"
	if in.ProjectID != nil {
		status = string(domainprojects.StatusCommitted)
	}
	row := driven.ManualItemRow{
		ID: uuid.New(), OrganisationID: orgID, Channel: string(ch),
		OccurredAt: in.OccurredAt.UTC(), Title: strings.TrimSpace(in.Title), BodyText: body,
		ProjectID: in.ProjectID, AssignmentStatus: status,
		AssignmentReason: &reason, AssignmentSource: &src,
		CreatedByUserID: userID, CreatedAt: now,
	}
	if err := s.Manuals.CreateManualItem(ctx, row); err != nil {
		return nil, err
	}
	for _, cid := range in.ParticipantContactIDs {
		c, err := s.Contacts.GetContact(ctx, orgID, cid)
		if err != nil || c == nil {
			continue
		}
		_ = s.Contacts.UpsertParticipant(ctx, driven.CorrespondenceParticipantRow{
			ID: uuid.New(), OrganisationID: orgID, ContactID: cid,
			Role: string(contacts.RoleParticipant), ManualItemID: &row.ID,
		})
		if status == string(domainprojects.StatusCommitted) && in.ProjectID != nil {
			_ = s.Projects.UpsertProjectParticipant(ctx, *in.ProjectID, cid, now)
		}
	}
	if status == string(domainprojects.StatusCommitted) && in.ProjectID != nil && s.AfterProjectCorrespondence != nil {
		manID := row.ID
		pid := *in.ProjectID
		s.AfterProjectCorrespondence(ctx, userID, pid, nil, &manID)
	}
	return &row, nil
}

func (s *Service) AssignManualItem(ctx context.Context, userID, manualItemID uuid.UUID, projectID *uuid.UUID) (*driven.ManualItemRow, error) {
	if s.Manuals == nil {
		return nil, fmt.Errorf("manuals not configured")
	}
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	item, err := s.Manuals.GetManualItem(ctx, orgID, manualItemID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound
	}
	status := "unassigned"
	reason := "user_assign"
	source := string(domainprojects.SourceUser)
	if projectID != nil {
		p, err := s.Projects.GetProject(ctx, orgID, *projectID)
		if err != nil {
			return nil, err
		}
		if p == nil || p.ArchivedAt != nil {
			return nil, ErrNotFound
		}
		status = string(domainprojects.StatusCommitted)
	}
	// Assigning a project clears any prior dismissal: an item cannot be both
	// filed and not-project-related.
	if err := s.Manuals.UpdateManualItemAssignment(ctx, orgID, manualItemID, projectID, status, reason, source, nil); err != nil {
		return nil, err
	}
	if status == string(domainprojects.StatusCommitted) && projectID != nil {
		now := time.Now().UTC()
		ids, _ := s.Contacts.ListContactIDsForManualItem(ctx, orgID, manualItemID)
		for _, cid := range ids {
			_ = s.Projects.UpsertProjectParticipant(ctx, *projectID, cid, now)
		}
		if s.AfterProjectCorrespondence != nil {
			manID := manualItemID
			pid := *projectID
			s.AfterProjectCorrespondence(ctx, userID, pid, nil, &manID)
		}
	}
	return s.Manuals.GetManualItem(ctx, orgID, manualItemID)
}

func (s *Service) GetTimeline(ctx context.Context, userID, projectID uuid.UUID, filter driven.TimelineFilter) ([]driven.TimelineItem, error) {
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
	if p == nil {
		return nil, ErrNotFound
	}
	return s.Timeline.ListProjectTimeline(ctx, userID, orgID, projectID, filter)
}

// AssignService auto-assigns projects after sync.
//
// LLM is optional. With it nil the service runs the deterministic tiers only,
// which is the behaviour when no model is configured.
type AssignService struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Assignments driven.AssignmentRepository
	Contacts    driven.ContactRepository
	Messages    driven.MessageRepository
	JobRuns     driven.JobRunRepository
	LLM         driven.LLMClient
}

type AssignChunkResult struct {
	MessagesProcessed int
	MessagesAssigned  int
	NextCursor        *driven.JobCursor
	Done              bool
	Tally             assignTally
}

// assignTally breaks a run down by outcome so a run that scored nothing is
// distinguishable from a run that failed.
type assignTally struct {
	CommittedRule  int
	ProvisionalLLM int
	Unscored       int
	Errors         int
}

func (t *assignTally) record(o assignOutcome) {
	switch o {
	case outcomeCommittedRule:
		t.CommittedRule++
	case outcomeProvisionalLLM:
		t.ProvisionalLLM++
	default:
		t.Unscored++
	}
}

func (t assignTally) assigned() int {
	return t.CommittedRule + t.ProvisionalLLM
}

func (t assignTally) meta(considered int) map[string]any {
	return map[string]any{
		"messages_considered": considered,
		"assigned":            t.assigned(),
		"committed_rule":      t.CommittedRule,
		"provisional_llm":     t.ProvisionalLLM,
		"unscored":            t.Unscored,
		"errors":              t.Errors,
	}
}

func (s *AssignService) AssignAfterSync(ctx context.Context, userID, accountID uuid.UUID) error {
	orgID, err := s.Users.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return err
	}
	projects, err := s.Projects.ListProjects(ctx, orgID, driven.ProjectListFilter{Limit: 500})
	if err != nil {
		return err
	}
	msgs, err := s.Assignments.ListMessagesNeedingAssign(ctx, userID, accountID, driven.AssignCandidateFilter{Limit: 500})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var runID *uuid.UUID
	if s.JobRuns != nil && len(msgs) > 0 {
		id := uuid.New()
		runID = &id
		_ = s.JobRuns.InsertJobRun(ctx, id, accountID, "assign_projects", "api", "running", now, time.Time{}, nil, `{}`)
	}
	tally := assignTally{}
	var firstErr error
	var unscored []driven.MessageRow
	for _, msg := range msgs {
		outcome, err := s.tryAssignOne(ctx, orgID, userID, accountID, msg, projects, runID, now)
		if err != nil {
			// Keep going so one bad message does not strand the rest, but record
			// the failure: reporting success on a failed pass is what made a
			// broken assignment run indistinguishable from "nothing matched".
			tally.Errors++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		tally.record(outcome)
		if outcome == outcomeUnscored {
			unscored = append(unscored, msg)
		}
	}
	// The model only sees what deterministic rules could not place.
	if n, err := s.scoreWithLLM(ctx, orgID, accountID, unscored, projects, runID, now); err != nil {
		tally.Errors++
		if firstErr == nil {
			firstErr = err
		}
		tally.ProvisionalLLM += n
		tally.Unscored -= n
	} else {
		tally.ProvisionalLLM += n
		tally.Unscored -= n
	}
	if s.JobRuns != nil && runID != nil {
		meta, _ := json.Marshal(tally.meta(len(msgs)))
		finished := time.Now().UTC()
		status := "success"
		var errMsg *string
		if tally.Errors > 0 {
			status = "failed"
			m := fmt.Sprintf("%d of %d messages failed to assign: %v", tally.Errors, len(msgs), firstErr)
			errMsg = &m
		}
		_ = s.JobRuns.UpdateJobRunStatus(ctx, *runID, status, &finished, errMsg, string(meta))
	}
	return firstErr
}

func (s *AssignService) AssignAccountChunk(ctx context.Context, run driven.RunContext) (*AssignChunkResult, error) {
	if s == nil || s.Users == nil || s.Projects == nil || s.Assignments == nil || s.Contacts == nil || s.Messages == nil {
		return nil, fmt.Errorf("assign service not configured")
	}
	if run.AccountID == nil || *run.AccountID == uuid.Nil {
		return nil, fmt.Errorf("account_id is required")
	}
	orgID, err := s.Users.GetHomeOrganisationID(ctx, run.UserID)
	if err != nil {
		return nil, err
	}
	projects, err := s.Projects.ListProjects(ctx, orgID, driven.ProjectListFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	// Only what nobody has decided: walking every message here re-scored
	// threads the operator had already filed or dismissed, and the model's
	// provisional row replaced their decision and put the thread back in
	// triage.
	filter := driven.AssignCandidateFilter{Limit: 26}
	if at, id, ok := jobkit.DecodeKeysetCursor(run.Cursor); ok {
		filter.BeforeReceivedAt = &at
		filter.BeforeID = &id
	}
	msgs, err := s.Assignments.ListMessagesNeedingAssign(ctx, run.UserID, *run.AccountID, filter)
	if err != nil {
		return nil, err
	}
	done := len(msgs) <= 25
	if len(msgs) > 25 {
		msgs = msgs[:25]
	}
	now := time.Now().UTC()
	tally := assignTally{}
	var unscored []driven.MessageRow
	for _, msg := range msgs {
		outcome, err := s.tryAssignOne(ctx, orgID, run.UserID, *run.AccountID, msg, projects, &run.RunID, now)
		if err != nil {
			return nil, err
		}
		tally.record(outcome)
		if outcome == outcomeUnscored {
			unscored = append(unscored, msg)
		}
	}
	n, err := s.scoreWithLLM(ctx, orgID, *run.AccountID, unscored, projects, &run.RunID, now)
	if err != nil {
		return nil, err
	}
	tally.ProvisionalLLM += n
	tally.Unscored -= n
	out := &AssignChunkResult{
		MessagesProcessed: len(msgs),
		MessagesAssigned:  tally.assigned(),
		Done:              done,
		Tally:             tally,
	}
	if !done && len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		out.NextCursor = jobkit.EncodeKeysetCursor(last.ReceivedAt, last.ID)
	}
	return out, nil
}

// assignOutcome records what the scorer decided, for run metadata.
type assignOutcome int

const (
	outcomeUnscored assignOutcome = iota
	outcomeCommittedRule
	outcomeProvisionalLLM
)

// tryAssignOne resolves one message against the project set.
//
// Order follows Wave 1 §9: a committed sibling in the same thread wins, then a
// single project code token commits, then the ranked scorer supplies a
// provisional suggestion. Only a code token may commit without the operator.
func (s *AssignService) tryAssignOne(ctx context.Context, orgID, userID, accountID uuid.UUID, msg driven.MessageRow, projects []driven.ProjectRow, runID *uuid.UUID, now time.Time) (assignOutcome, error) {
	// 1. Sibling committed project.
	if msg.ConversationID != nil && strings.TrimSpace(*msg.ConversationID) != "" {
		sib, err := s.Assignments.FindCommittedSiblingProject(ctx, userID, accountID, *msg.ConversationID, msg.ID)
		if err != nil {
			return outcomeUnscored, err
		}
		if sib != nil {
			row := driven.AssignmentRow{
				ID: uuid.New(), OrganisationID: orgID, AccountID: accountID,
				ConversationID: *msg.ConversationID, ProjectID: sib,
				Status: string(domainprojects.StatusCommitted), Reason: "thread_sibling",
				Source: string(domainprojects.SourceRule), RunID: runID, CreatedAt: now, UpdatedAt: now,
			}
			if err := s.Assignments.UpsertThreadAssignment(ctx, row); err != nil {
				return outcomeUnscored, err
			}
			_ = s.upsertParticipants(ctx, orgID, *sib, msg)
			return outcomeCommittedRule, nil
		}
	}

	// 2. Exactly one project code token commits (Wave 1 §7: confidence >= 0.9
	//    AND a code match).
	if p, ok := codeTokenHit(msg, projects); ok {
		if err := s.writeAssignment(ctx, orgID, accountID, msg, p.ID,
			domainprojects.StatusCommitted, "code:"+p.Code, domainprojects.SourceRule,
			ptrFloat64(confidenceCodeToken), runID, now); err != nil {
			return outcomeUnscored, err
		}
		_ = s.upsertParticipants(ctx, orgID, p.ID, msg)
		return outcomeCommittedRule, nil
	}

	// Anything else is the model's to place. Guessing from a sender domain or
	// a name keyword produced more wrong suggestions than it saved attention,
	// so a message no certain rule matched is left unscored here.
	return outcomeUnscored, nil
}

// writeAssignment upserts at thread scope when the message has a conversation,
// and as a per-message override otherwise (Wave 1 §7).
func (s *AssignService) writeAssignment(ctx context.Context, orgID, accountID uuid.UUID, msg driven.MessageRow, projectID uuid.UUID, status domainprojects.AssignmentStatus, reason string, source domainprojects.AssignmentSource, confidence *float64, runID *uuid.UUID, now time.Time) error {
	pid := projectID
	if msg.ConversationID == nil || strings.TrimSpace(*msg.ConversationID) == "" {
		mid := msg.ID
		return s.Assignments.UpsertMessageOverride(ctx, driven.AssignmentRow{
			OrganisationID: orgID, AccountID: accountID, MessageID: &mid, ProjectID: &pid,
			Status: string(status), Confidence: confidence, Reason: reason,
			Source: string(source), RunID: runID, CreatedAt: now, UpdatedAt: now,
		})
	}
	return s.Assignments.UpsertThreadAssignment(ctx, driven.AssignmentRow{
		ID: uuid.New(), OrganisationID: orgID, AccountID: accountID,
		ConversationID: *msg.ConversationID, ProjectID: &pid,
		Status: string(status), Confidence: confidence, Reason: reason,
		Source: string(source), RunID: runID, CreatedAt: now, UpdatedAt: now,
	})
}

// hasConversation reports whether a message belongs to a thread. Mail synced
// from a delta tombstone lost its conversation id, so this is not hypothetical.
func hasConversation(msg *driven.MessageRow) bool {
	return msg != nil && msg.ConversationID != nil && strings.TrimSpace(*msg.ConversationID) != ""
}

func ptrFloat64(v float64) *float64 { return &v }

func (s *AssignService) upsertParticipants(ctx context.Context, orgID, projectID uuid.UUID, msg driven.MessageRow) error {
	now := time.Now().UTC()
	ids, err := s.Contacts.ListContactIDsForMessage(ctx, orgID, msg.ID)
	if err != nil {
		return err
	}
	for _, cid := range ids {
		_ = s.Projects.UpsertProjectParticipant(ctx, projectID, cid, now)
	}
	return nil
}

var tokenSplitter = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func matchProjectCodes(text string, projects []driven.ProjectRow) []driven.ProjectRow {
	upper := strings.ToUpper(text)
	tokens := tokenizeAlnum(upper)
	tokenSet := map[string]struct{}{}
	for _, t := range tokens {
		tokenSet[t] = struct{}{}
	}
	hits := make([]driven.ProjectRow, 0)
	for _, p := range projects {
		if p.ArchivedAt != nil {
			continue
		}
		if _, ok := tokenSet[p.Code]; ok {
			hits = append(hits, p)
		}
	}
	return hits
}

func matchProjectNamesKeywords(text string, projects []driven.ProjectRow) []driven.ProjectRow {
	lower := strings.ToLower(text)
	hits := make([]driven.ProjectRow, 0)
	for _, p := range projects {
		if p.ArchivedAt != nil {
			continue
		}
		matched := false
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if name != "" && strings.Contains(lower, name) {
			matched = true
		}
		if !matched {
			for _, kw := range p.Keywords {
				k := strings.ToLower(strings.TrimSpace(kw))
				if k != "" && strings.Contains(lower, k) {
					matched = true
					break
				}
			}
		}
		if matched {
			hits = append(hits, p)
		}
	}
	return hits
}

func tokenizeAlnum(s string) []string {
	parts := tokenSplitter.Split(s, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Keep tokens that look like project codes (start with letter).
		r := []rune(p)
		if len(r) == 0 || !unicode.IsLetter(r[0]) {
			continue
		}
		out = append(out, p)
	}
	return out
}
