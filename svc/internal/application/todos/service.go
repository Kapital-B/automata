// Package todos shares to-dos from mail with the project their message is
// filed to. A to-do is born in one person's mailbox; once the mail is filed to
// a project, everyone on the project can see it and close it. The mail itself
// stays private: teammates see the to-do, not the message behind it.
package todos

import (
	"context"
	"errors"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	"time"

	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

type Service struct {
	Users     domainauth.UserRepository
	Projects  domainprojects.ProjectRepository
	Summaries domainmessages.SummaryRepository
}

// ForProject lists the project's open to-dos, the caller's first.
func (s *Service) ForProject(ctx context.Context, caller, projectID uuid.UUID) ([]domainissues.TodoTodo, error) {
	orgID, err := s.member(ctx, caller, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Summaries.ListOpenActionItemsForProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	mine := make([]domainissues.TodoTodo, 0, len(rows))
	theirs := make([]domainissues.TodoTodo, 0)
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
func (s *Service) ForIssue(ctx context.Context, caller, projectID, issueID uuid.UUID) ([]domainissues.TodoTodo, error) {
	all, err := s.ForProject(ctx, caller, projectID)
	if errors.Is(err, domainissues.TodoErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]domainissues.TodoTodo, 0)
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
	return domainissues.TodoErrNotFound
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
		return uuid.Nil, domainissues.TodoErrNotFound
	}
	m, err := s.Projects.GetProjectMember(ctx, projectID, caller)
	if err != nil {
		return uuid.Nil, err
	}
	if m == nil {
		return uuid.Nil, domainissues.TodoErrNotFound
	}
	return orgID, nil
}

func view(caller uuid.UUID, r domainmessages.ProjectTodoRow) domainissues.TodoTodo {
	t := domainissues.TodoTodo{
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
