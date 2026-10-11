package issues

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
	time "time"
)

// IssueSuggestResult is a proposal the user must confirm via Create.
type IssueSuggestResult struct {
	Title      string
	ItemRefs   []IssueItemRef
	Confidence float64
	Reason     string
}

type IssueItemRef struct {
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
}

var (
	IssueErrNotFound       = errors.New("not found")
	IssueErrInvalidStatus  = errors.New("invalid status")
	IssueErrDualAssignee   = errors.New("cannot set both assignee_user_id and assignee_contact_id")
	IssueErrItemConflict   = errors.New("item already linked to an issue")
	IssueErrWrongProject   = errors.New("item not on this project")
	IssueErrInvalidItemRef = errors.New("invalid item ref")
)

var (
	IssueErrLLMUnavailable   = errors.New("llm not configured")
	IssueErrNothingToSuggest = errors.New("no unassigned correspondence to suggest from")
)

type IssueTrailItem struct {
	Item        IssueItemRow
	Source      string // mail | manual
	Title       string
	OccurredAt  *time.Time
	AccountID   *uuid.UUID
	Channel     string
	BodySnippet string
}

type IssueIssueView struct {
	Issue         IssueRow
	AwaitingMe    bool
	AssigneeLabel string
	Items         []IssueTrailItem
	// ItemCount is the evidence count. On a listing it is resolved in one
	// query for the whole project rather than per issue; on a single issue
	// it is len(Items).
	ItemCount int
	// Todos are the project's open to-dos from mail on this issue's trail,
	// whoever's mailbox they came from. Only a single-issue view carries them.
	Todos []TodoTodo
}

// TodoErrNotFound covers a project the caller is not on and a to-do that is not
// open on the project, so neither can be probed for.
var TodoErrNotFound = errors.New("not found")

// TodoTodo is an open to-do as one member sees it.
type TodoTodo struct {
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

type IssueUpdateInput struct {
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

// IssueSuggestInput scopes an LLM issue proposal.
type IssueSuggestInput struct {
	// AccountID optional: when set, only mail from this account is considered (with manuals).
	AccountID *uuid.UUID
}

type IssueCreateInput struct {
	Title               string
	CurrentPositionNote string
	AssigneeUserID      *uuid.UUID
	AssigneeContactID   *uuid.UUID
	ItemRefs            []IssueItemRef
	// Source is "llm" when extraction raised the issue. Empty means a person
	// did, which is the default for every operator-facing path.
	Source string
}

type IssueUseCases interface {
	HasLLM() bool
	List(ctx context.Context, userID, projectID uuid.UUID) ([]IssueIssueView, error)
	Get(ctx context.Context, userID, issueID uuid.UUID) (*IssueIssueView, error)
	Create(ctx context.Context, userID, projectID uuid.UUID, in IssueCreateInput) (*IssueIssueView, error)
	Update(ctx context.Context, userID, issueID uuid.UUID, in IssueUpdateInput) (*IssueIssueView, error)
	AddItem(ctx context.Context, userID, issueID uuid.UUID, ref IssueItemRef) (*IssueIssueView, error)
	RemoveItem(ctx context.Context, userID, issueID, itemID uuid.UUID) (*IssueIssueView, error)
	Discard(ctx context.Context, userID, issueID uuid.UUID) (*IssueIssueView, error)
	Suggest(ctx context.Context, userID, projectID uuid.UUID, in IssueSuggestInput) (*IssueSuggestResult, error)
}

type TodoUseCases interface {
	ForProject(ctx context.Context, caller, projectID uuid.UUID) ([]TodoTodo, error)
	ForIssue(ctx context.Context, caller, projectID, issueID uuid.UUID) ([]TodoTodo, error)
	Complete(ctx context.Context, caller, projectID, todoID uuid.UUID) error
	CompleteForIssue(ctx context.Context, caller, projectID, issueID uuid.UUID) (int, error)
}
