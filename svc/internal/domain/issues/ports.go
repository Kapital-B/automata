package issues

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// IssueRow is a persisted project issue.
type IssueRow struct {
	ID                  uuid.UUID
	OrganisationID      uuid.UUID
	ProjectID           uuid.UUID
	Title               string
	CurrentPositionNote string
	Status              string
	AssigneeUserID      *uuid.UUID
	AssigneeContactID   *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
	// ResolvedAt is when the issue was resolved. Nil on rows written before
	// the column existed; readers fall back to UpdatedAt.
	ResolvedAt *time.Time
	// DiscardedAt marks an issue that should never have been raised, as
	// opposed to one whose work is done.
	DiscardedAt *time.Time
	// Source is "llm" for an extracted issue and "human" for one a person
	// raised. Empty on rows written before extraction existed, all of which
	// were created by hand.
	Source string
}

// IssueItemRow links correspondence to an issue.
type IssueItemRow struct {
	ID           uuid.UUID
	IssueID      uuid.UUID
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AddedAt      time.Time
}

// IssueLink is the issue a message sits on, with enough to label it.
type IssueLink struct {
	IssueID   uuid.UUID
	ProjectID uuid.UUID
	Title     string
	Status    string
}

// IssueRepository persists issues and trail links.
type IssueRepository interface {
	CreateIssue(ctx context.Context, row IssueRow) error
	GetIssue(ctx context.Context, organisationID, issueID uuid.UUID) (*IssueRow, error)
	ListIssuesByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]IssueRow, error)
	UpdateIssue(ctx context.Context, row IssueRow) error
	AddIssueItem(ctx context.Context, row IssueItemRow) error
	RemoveIssueItem(ctx context.Context, organisationID, issueID, itemID uuid.UUID) error
	GetIssueItem(ctx context.Context, organisationID, itemID uuid.UUID) (*IssueItemRow, error)
	ListIssueItems(ctx context.Context, issueID uuid.UUID) ([]IssueItemRow, error)
	// CountIssueItemsByProject returns evidence counts keyed by issue id, in
	// one query, so a project's issue list can show provenance without a
	// lookup per row.
	CountIssueItemsByProject(ctx context.Context, organisationID, projectID uuid.UUID) (map[uuid.UUID]int, error)
	FindIssueIDByMessage(ctx context.Context, messageID uuid.UUID) (*uuid.UUID, error)
	// ListIssueLinksForMessages returns, for each message on an issue's trail,
	// the issue it belongs to. Discarded issues are left out. A message can be
	// on at most one issue, so the result is keyed by message id.
	ListIssueLinksForMessages(ctx context.Context, organisationID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]IssueLink, error)
	FindIssueIDByManualItem(ctx context.Context, manualItemID uuid.UUID) (*uuid.UUID, error)
}
