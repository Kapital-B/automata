// Package todos shares to-dos from mail with the project their message is
// filed to. A to-do is born in one person's mailbox; once the mail is filed to
// a project, everyone on the project can see it and close it. The mail itself
// stays private: teammates see the to-do, not the message behind it.
package todos

import (
	"context"
	"errors"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// ErrNotFound covers a project the caller is not on and a to-do that is not
// open on the project, so neither can be probed for.
var ErrNotFound = errors.New("not found")

// Todo is an open to-do as one member sees it.
type Todo struct {
	ID          uuid.UUID
	Text        string
	OwnerUserID uuid.UUID
	// OwnerLabel is "You" for the caller's own and the owner's email
	// otherwise; users have no display name yet.
	OwnerLabel string
	Mine       bool
	DueAt      *time.Time
	CreatedAt  time.Time
	IssueID    *uuid.UUID
	IssueTitle string
	// AccountID and MessageID open the message, so only the owner gets them.
	AccountID *uuid.UUID
	MessageID *uuid.UUID
}

type Service struct {
	Users     driven.UserRepository
	Projects  driven.ProjectRepository
	Summaries driven.SummaryRepository
}

// ForProject lists the project's open to-dos, the caller's first.
func (s *Service) ForProject(ctx context.Context, caller, projectID uuid.UUID) ([]Todo, error) {
	orgID, err := s.member(ctx, caller, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Summaries.ListOpenActionItemsForProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	mine := make([]Todo, 0, len(rows))
	theirs := make([]Todo, 0)
	for _, r := range rows {
		t := view(caller, r)
		if t.Mine {
			mine = append(mine, t)
		} else {
			theirs = append(theirs, t)
		}
	}
	return append(mine, theirs...), nil
}

// ForIssue lists the project's open to-dos whose message is on the issue. A
// caller who is not on the project sees none rather than an error, so the
// issue itself still opens.
func (s *Service) ForIssue(ctx context.Context, caller, projectID, issueID uuid.UUID) ([]Todo, error) {
	all, err := s.ForProject(ctx, caller, projectID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Todo, 0)
	for _, t := range all {
		if t.IssueID != nil && *t.IssueID == issueID {
			out = append(out, t)
		}
	}
	return out, nil
}

// Complete closes one of the project's open to-dos on the caller's behalf.
func (s *Service) Complete(ctx context.Context, caller, projectID, todoID uuid.UUID) error {
	all, err := s.ForProject(ctx, caller, projectID)
	if err != nil {
		return err
	}
	for _, t := range all {
		if t.ID == todoID {
			return s.Summaries.CompleteActionItem(ctx, todoID, caller, time.Now().UTC())
		}
	}
	return ErrNotFound
}

// CompleteForIssue closes every open to-do on the issue and returns how many.
func (s *Service) CompleteForIssue(ctx context.Context, caller, projectID, issueID uuid.UUID) (int, error) {
	todos, err := s.ForIssue(ctx, caller, projectID, issueID)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	for _, t := range todos {
		if err := s.Summaries.CompleteActionItem(ctx, t.ID, caller, now); err != nil {
			return 0, err
		}
	}
	return len(todos), nil
}

// member returns the caller's organisation when they are on the project.
func (s *Service) member(ctx context.Context, caller, projectID uuid.UUID) (uuid.UUID, error) {
	orgID, err := s.Users.GetHomeOrganisationID(ctx, caller)
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
	m, err := s.Projects.GetProjectMember(ctx, projectID, caller)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, ErrNotFound
	}
	return orgID, nil
}

func view(caller uuid.UUID, r driven.ProjectTodoRow) Todo {
	t := Todo{
		ID:          r.Item.ID,
		Text:        r.Item.Text,
		OwnerUserID: r.Item.UserID,
		OwnerLabel:  r.OwnerEmail,
		DueAt:       r.Item.DueAt,
		CreatedAt:   r.Item.CreatedAt,
		IssueID:     r.IssueID,
		IssueTitle:  r.IssueTitle,
	}
	if r.Item.UserID == caller {
		t.Mine = true
		t.OwnerLabel = "You"
		acc, msg := r.Item.AccountID, r.Item.MessageID
		t.AccountID, t.MessageID = &acc, &msg
	}
	return t
}
