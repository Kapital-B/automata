package issues

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	apptodos "github.com/Kapital-B/automata/svc/internal/application/todos"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidStatus  = errors.New("invalid status")
	ErrDualAssignee   = errors.New("cannot set both assignee_user_id and assignee_contact_id")
	ErrItemConflict   = errors.New("item already linked to an issue")
	ErrWrongProject   = errors.New("item not on this project")
	ErrInvalidItemRef = errors.New("invalid item ref")
)

// Service handles issue CRUD and trail links.
type Service struct {
	Users       driven.UserRepository
	Projects    driven.ProjectRepository
	Issues      driven.IssueRepository
	Assignments driven.AssignmentRepository
	Manuals     driven.ManualItemRepository
	Contacts    driven.ContactRepository
	Messages    driven.MessageRepository
	Timeline    driven.TimelineRepository
	LLM         driven.LLMClient
	// Todos supplies the project's to-dos from mail on an issue's trail.
	// Optional: without it an issue shows no to-dos.
	Todos *apptodos.Service
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

// HasLLM reports whether issue suggest can run.
func (s *Service) HasLLM() bool {
	return s != nil && s.LLM != nil
}

type IssueView struct {
	Issue         driven.IssueRow
	AwaitingMe    bool
	AssigneeLabel string
	Items         []TrailItem
	// ItemCount is the evidence count. On a listing it is resolved in one
	// query for the whole project rather than per issue; on a single issue
	// it is len(Items).
	ItemCount int
	// Todos are the project's open to-dos from mail on this issue's trail,
	// whoever's mailbox they came from. Only a single-issue view carries them.
	Todos []apptodos.Todo
}

type TrailItem struct {
	Item        driven.IssueItemRow
	Source      string // mail | manual
	Title       string
	OccurredAt  *time.Time
	AccountID   *uuid.UUID
	Channel     string
	BodySnippet string
}

type CreateInput struct {
	Title               string
	CurrentPositionNote string
	AssigneeUserID      *uuid.UUID
	AssigneeContactID   *uuid.UUID
	ItemRefs            []ItemRef
	// Source is "llm" when extraction raised the issue. Empty means a person
	// did, which is the default for every operator-facing path.
	Source string
}

type ItemRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

type UpdateInput struct {
	Title               *string
	CurrentPositionNote *string
	Status              *string
	// ClearAssignee clears both assignee fields when true.
	ClearAssignee     bool
	AssigneeUserID    *uuid.UUID
	AssigneeContactID *uuid.UUID
	SetAssignee       bool // true when client sent assignee fields
	// CompleteTodos marks the to-dos on the trail done, the team's as well as
	// the caller's, when this update resolves the issue.
	CompleteTodos bool
}

func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID) ([]IssueView, error) {
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
	rows, err := s.Issues.ListIssuesByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	counts, err := s.Issues.CountIssueItemsByProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]IssueView, 0, len(rows))
	for _, row := range rows {
		out = append(out, IssueView{
			Issue: row, AwaitingMe: awaitingMe(row, userID),
			AssigneeLabel: s.assigneeLabel(ctx, orgID, userID, row),
			ItemCount:     counts[row.ID],
		})
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, issueID uuid.UUID) (*IssueView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Issues.GetIssue(ctx, orgID, issueID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	items, err := s.Issues.ListIssueItems(ctx, issueID)
	if err != nil {
		return nil, err
	}
	trail := make([]TrailItem, 0, len(items))
	for _, it := range items {
		trail = append(trail, s.enrichTrailItem(ctx, userID, orgID, it))
	}
	sort.SliceStable(trail, func(i, j int) bool {
		ai, aj := trail[i].OccurredAt, trail[j].OccurredAt
		if ai == nil && aj == nil {
			return trail[i].Item.AddedAt.Before(trail[j].Item.AddedAt)
		}
		if ai == nil {
			return false
		}
		if aj == nil {
			return true
		}
		if ai.Equal(*aj) {
			return trail[i].Item.AddedAt.Before(trail[j].Item.AddedAt)
		}
		return ai.Before(*aj)
	})
	var todos []apptodos.Todo
	if s.Todos != nil {
		todos, err = s.Todos.ForIssue(ctx, userID, row.ProjectID, issueID)
		if err != nil {
			return nil, err
		}
	}
	return &IssueView{
		Issue: *row, AwaitingMe: awaitingMe(*row, userID),
		AssigneeLabel: s.assigneeLabel(ctx, orgID, userID, *row),
		Items:         trail,
		ItemCount:     len(trail),
		Todos:         todos,
	}, nil
}

// sourceOrHuman defaults an unset source to human: every operator-facing path
// leaves it empty, and only extraction sets it.
func sourceOrHuman(src string) string {
	if strings.TrimSpace(src) == "" {
		return string(domainissues.SourceHuman)
	}
	return src
}

func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in CreateInput) (*IssueView, error) {
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
	member, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, ErrNotFound
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("title required")
	}
	assigneeUser := in.AssigneeUserID
	assigneeContact := in.AssigneeContactID
	if assigneeUser == nil && assigneeContact == nil {
		uid := userID
		assigneeUser = &uid
	}
	if err := s.validateAssignee(ctx, orgID, projectID, assigneeUser, assigneeContact); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := driven.IssueRow{
		ID: uuid.New(), OrganisationID: orgID, ProjectID: projectID,
		Title: title, CurrentPositionNote: strings.TrimSpace(in.CurrentPositionNote),
		Status:         string(domainissues.StatusOpen),
		AssigneeUserID: assigneeUser, AssigneeContactID: assigneeContact,
		CreatedAt: now, UpdatedAt: now,
		Source: sourceOrHuman(in.Source),
	}
	if err := s.Issues.CreateIssue(ctx, row); err != nil {
		return nil, err
	}
	for _, ref := range in.ItemRefs {
		if err := s.addItemInternal(ctx, userID, orgID, row, ref); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, userID, row.ID)
}

func (s *Service) Update(ctx context.Context, userID, issueID uuid.UUID, in UpdateInput) (*IssueView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Issues.GetIssue(ctx, orgID, issueID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if in.Title != nil {
		row.Title = strings.TrimSpace(*in.Title)
	}
	if in.CurrentPositionNote != nil {
		row.CurrentPositionNote = strings.TrimSpace(*in.CurrentPositionNote)
	}
	resolving := false
	if in.Status != nil {
		st := domainissues.Status(strings.TrimSpace(*in.Status))
		if !st.Valid() {
			return nil, ErrInvalidStatus
		}
		resolving = st == domainissues.StatusResolved && row.Status != string(st)
		// Stamp the transition, not the edit: updated_at moves every time the
		// issue is touched, so it cannot date the resolution.
		if string(st) != row.Status {
			if st == domainissues.StatusResolved {
				resolved := time.Now().UTC()
				row.ResolvedAt = &resolved
			} else {
				row.ResolvedAt = nil
			}
		}
		row.Status = string(st)
	}
	if in.ClearAssignee {
		row.AssigneeUserID = nil
		row.AssigneeContactID = nil
	} else if in.SetAssignee {
		row.AssigneeUserID = in.AssigneeUserID
		row.AssigneeContactID = in.AssigneeContactID
		if err := s.validateAssignee(ctx, orgID, row.ProjectID, row.AssigneeUserID, row.AssigneeContactID); err != nil {
			return nil, err
		}
	}
	row.UpdatedAt = time.Now().UTC()
	if err := s.Issues.UpdateIssue(ctx, *row); err != nil {
		return nil, err
	}
	if resolving && in.CompleteTodos && s.Todos != nil {
		if _, err := s.Todos.CompleteForIssue(ctx, userID, row.ProjectID, issueID); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, userID, issueID)
}

func (s *Service) AddItem(ctx context.Context, userID, issueID uuid.UUID, ref ItemRef) (*IssueView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Issues.GetIssue(ctx, orgID, issueID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if err := s.addItemInternal(ctx, userID, orgID, *row, ref); err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, issueID)
}

func (s *Service) RemoveItem(ctx context.Context, userID, issueID, itemID uuid.UUID) (*IssueView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Issues.GetIssue(ctx, orgID, issueID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if err := s.Issues.RemoveIssueItem(ctx, orgID, issueID, itemID); err != nil {
		return nil, ErrNotFound
	}
	return s.Get(ctx, userID, issueID)
}

func (s *Service) addItemInternal(ctx context.Context, userID, orgID uuid.UUID, issue driven.IssueRow, ref ItemRef) error {
	hasMsg := ref.MessageID != nil
	hasManual := ref.ManualItemID != nil
	if hasMsg == hasManual {
		return ErrInvalidItemRef
	}
	if hasMsg {
		existing, err := s.Issues.FindIssueIDByMessage(ctx, *ref.MessageID)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrItemConflict
		}
		eff, err := s.Assignments.EffectiveAssignment(ctx, userID, *ref.MessageID)
		if err != nil {
			return err
		}
		if eff == nil || eff.ProjectID == nil || *eff.ProjectID != issue.ProjectID {
			return ErrWrongProject
		}
	} else {
		existing, err := s.Issues.FindIssueIDByManualItem(ctx, *ref.ManualItemID)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrItemConflict
		}
		manual, err := s.Manuals.GetManualItem(ctx, orgID, *ref.ManualItemID)
		if err != nil {
			return err
		}
		if manual == nil || manual.ProjectID == nil || *manual.ProjectID != issue.ProjectID {
			return ErrWrongProject
		}
	}
	now := time.Now().UTC()
	return s.Issues.AddIssueItem(ctx, driven.IssueItemRow{
		ID: uuid.New(), IssueID: issue.ID,
		MessageID: ref.MessageID, ManualItemID: ref.ManualItemID, AddedAt: now,
	})
}

func (s *Service) validateAssignee(ctx context.Context, orgID, projectID uuid.UUID, userID, contactID *uuid.UUID) error {
	if !domainissues.ValidAssigneeXOR(userID != nil, contactID != nil) {
		return ErrDualAssignee
	}
	if userID != nil {
		m, err := s.Projects.GetProjectMember(ctx, projectID, *userID)
		if err != nil {
			return err
		}
		if m == nil {
			return fmt.Errorf("assignee user must be a project member")
		}
	}
	if contactID != nil {
		c, err := s.Contacts.GetContact(ctx, orgID, *contactID)
		if err != nil {
			return err
		}
		if c == nil {
			return ErrNotFound
		}
	}
	return nil
}

func awaitingMe(row driven.IssueRow, caller uuid.UUID) bool {
	return row.Status == string(domainissues.StatusAwaitingInput) &&
		row.AssigneeUserID != nil && *row.AssigneeUserID == caller
}

func (s *Service) assigneeLabel(ctx context.Context, orgID, caller uuid.UUID, row driven.IssueRow) string {
	if row.AssigneeUserID != nil {
		if *row.AssigneeUserID == caller {
			return "You"
		}
		return "Member"
	}
	if row.AssigneeContactID != nil && s.Contacts != nil {
		c, err := s.Contacts.GetContact(ctx, orgID, *row.AssigneeContactID)
		if err == nil && c != nil {
			return c.DisplayName
		}
		return "Contact"
	}
	return "Unassigned"
}

func (s *Service) enrichTrailItem(ctx context.Context, userID, orgID uuid.UUID, it driven.IssueItemRow) TrailItem {
	out := TrailItem{Item: it}
	if it.MessageID != nil {
		out.Source = "mail"
		msg, err := s.Messages.GetMessage(ctx, userID, *it.MessageID)
		if err == nil && msg != nil {
			out.Title = msg.Subject
			out.OccurredAt = &msg.ReceivedAt
			out.AccountID = &msg.AccountID
			if msg.BodyText != nil {
				out.BodySnippet = *msg.BodyText
				if len(out.BodySnippet) > 160 {
					out.BodySnippet = out.BodySnippet[:160] + "…"
				}
			}
		}
		return out
	}
	out.Source = "manual"
	if it.ManualItemID != nil {
		m, err := s.Manuals.GetManualItem(ctx, orgID, *it.ManualItemID)
		if err == nil && m != nil {
			out.Title = m.Title
			out.OccurredAt = &m.OccurredAt
			out.Channel = m.Channel
			out.BodySnippet = m.BodyText
			if len(out.BodySnippet) > 160 {
				out.BodySnippet = out.BodySnippet[:160] + "…"
			}
		}
	}
	return out
}

// Discard marks an issue as one that should never have been raised.
//
// Distinct from resolving, which means the work is done. Keeping them separate
// preserves the signal for whether extraction is proposing useful issues.
func (s *Service) Discard(ctx context.Context, userID, issueID uuid.UUID) (*IssueView, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	row, err := s.Issues.GetIssue(ctx, orgID, issueID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrNotFound
	}
	if row.DiscardedAt == nil {
		now := time.Now().UTC()
		row.DiscardedAt = &now
		row.UpdatedAt = now
		if err := s.Issues.UpdateIssue(ctx, *row); err != nil {
			return nil, err
		}
	}
	return s.Get(ctx, userID, issueID)
}
