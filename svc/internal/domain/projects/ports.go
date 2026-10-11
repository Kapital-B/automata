package projects

import (
	"context"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"github.com/google/uuid"
	"time"
)

// AssignCandidateFilter pages ListMessagesNeedingAssign. The keyset returns
// messages strictly older than (BeforeReceivedAt, BeforeID).
type AssignCandidateFilter struct {
	Limit            int
	BeforeReceivedAt *time.Time
	BeforeID         *uuid.UUID
}

// ProjectRow is a persisted project.
type ProjectRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	Name           string
	Code           string
	Description    *string
	Client         *string
	Keywords       []string
	ArchivedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	// LastExtractedAt is when extraction last succeeded for this project.
	// Nil means it has never run.
	LastExtractedAt *time.Time
}

// ProjectMemberRow is the operator's membership on a project.
type ProjectMemberRow struct {
	ID                uuid.UUID
	ProjectID         uuid.UUID
	UserID            uuid.UUID
	Role              string
	Discipline        *string
	Responsibilities  *string
	CurrentScope      *string
	ApprovalAuthority *string
	OutOfScope        *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// AttentionRow is one thing awaiting the caller, resolved in a single query
// across the projects they are a member of.
type AttentionRow struct {
	Kind        string // issue_assignee | member_role | provisional_fact | provisional_decision | open_contradiction
	Title       string // raw label/statement/summary; the service formats it
	ProjectID   uuid.UUID
	ProjectName string
	RefType     string
	RefID       uuid.UUID
	OccurredAt  time.Time
}

// DueProject is a project whose correspondence is ready to be extracted.
type DueProject struct {
	ProjectID uuid.UUID
	// OwnerUserID is a project member the extraction job runs as.
	OwnerUserID uuid.UUID
}

// ActivityItem is one change to what a project knows, for the Home feed.
type ActivityItem struct {
	Kind        string // see the kind table in addendum-home-overview.md §6.3
	OccurredAt  time.Time
	ProjectID   uuid.UUID
	ProjectCode string
	ProjectName string
	Title       string
	RefType     string
	RefID       uuid.UUID
	Source      string // user | rule | llm, empty when the row has no source
}

// ActivityFilter narrows the activity feed. Before is a keyset, not an offset:
// the feed orders by a timestamp over a mutating set.
type ActivityFilter struct {
	Limit     int
	Before    *time.Time
	BeforeID  *uuid.UUID
	Kinds     []string
	ProjectID *uuid.UUID
}

// OverviewCounts backs the Home metric cards.
type OverviewCounts struct {
	OpenContradictions int
	ProvisionalFacts   int
	ProposedDecisions  int
	ActiveProjects     int
}

// OverviewProject is one row of the Home projects list.
type OverviewProject struct {
	ID   uuid.UUID
	Code string
	Name string
	// Teaser is the project's newest active fact, or failing that its newest
	// accepted decision. Empty when the project has neither.
	Teaser         string
	LastActivityAt *time.Time
}

// ProjectListFilter narrows project listing.
type ProjectListFilter struct {
	IncludeArchived bool
	// MemberUserID, when set, keeps only projects that user is a member of.
	MemberUserID *uuid.UUID
	Limit        int
	Offset       int
}

// ProjectRepository persists org-scoped projects and membership.
type ProjectRepository interface {
	ListProjects(ctx context.Context, organisationID uuid.UUID, filter ProjectListFilter) ([]ProjectRow, error)
	GetProject(ctx context.Context, organisationID, projectID uuid.UUID) (*ProjectRow, error)
	GetProjectByCode(ctx context.Context, organisationID uuid.UUID, code string) (*ProjectRow, error)
	CreateProject(ctx context.Context, project ProjectRow, member ProjectMemberRow) error
	UpdateProject(ctx context.Context, project ProjectRow) error
	GetProjectMember(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMemberRow, error)
	UpdateProjectMember(ctx context.Context, member ProjectMemberRow) error
	UpsertProjectParticipant(ctx context.Context, projectID, contactID uuid.UUID, firstSeenAt time.Time) error
	// ListAttention returns everything awaiting the caller across the projects
	// they are a member of, in one query.
	ListAttention(ctx context.Context, userID, organisationID uuid.UUID) ([]AttentionRow, error)
	// ListProjectsDueForExtraction returns projects with correspondence assigned
	// since their last extraction, debounced: either the newest such assignment
	// is at least quietFor old, or the oldest is at least ceiling old.
	ListProjectsDueForExtraction(ctx context.Context, now time.Time, quietFor, ceiling time.Duration, limit int) ([]DueProject, error)
	// MarkProjectExtracted advances the project's extraction watermark. Only
	// called on success, so a failed run retries rather than skipping.
	MarkProjectExtracted(ctx context.Context, projectID uuid.UUID, at time.Time) error
	// ListActivity returns recent changes across the projects the caller is a
	// member of, newest first.
	ListActivity(ctx context.Context, userID, organisationID uuid.UUID, filter ActivityFilter) ([]ActivityItem, error)
	// CountOverview returns the Home metric-card counts, membership scoped.
	CountOverview(ctx context.Context, userID, organisationID uuid.UUID) (OverviewCounts, error)
	// ListOverviewProjects returns the caller's projects with their real last
	// activity time, derived from the same events as ListActivity.
	ListOverviewProjects(ctx context.Context, userID, organisationID uuid.UUID, limit int) ([]OverviewProject, error)
	CountProjectMembers(ctx context.Context, projectID uuid.UUID) (int, error)
}

// AssignmentRow is a thread or override assignment.
type AssignmentRow struct {
	ID               uuid.UUID
	OrganisationID   uuid.UUID
	AccountID        uuid.UUID
	ConversationID   string // thread only
	MessageID        *uuid.UUID
	ProjectID        *uuid.UUID
	Status           string
	Confidence       *float64
	Reason           string
	Source           string
	RunID            *uuid.UUID
	AssignedByUserID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
	// NotRelevantAt marks correspondence the operator decided is not project
	// work. Set with a nil ProjectID; assigning a project clears it.
	NotRelevantAt *time.Time
}

// EffectiveAssignment is the resolved project for a message.
type EffectiveAssignment struct {
	ProjectID      *uuid.UUID
	Status         string // committed | provisional | unassigned
	Reason         string
	Source         string
	Scope          string // thread | message | none
	ConversationID *string
	AccountID      uuid.UUID
	MessageID      uuid.UUID
	// NotRelevantAt is set when the effective decision is "not project work".
	NotRelevantAt *time.Time
}

// UnassignedItem is a mail or manual row for the Unassigned queue.
//
// Mail rows are thread units: one row per conversation, represented by its most
// recent queued message. A message carrying its own assignment override is its
// own decision and stays a separate row.
type UnassignedItem struct {
	Kind           string // message | manual
	MessageID      *uuid.UUID
	ManualItemID   *uuid.UUID
	AccountID      *uuid.UUID
	AccountLabel   string
	Subject        string
	FromJSON       string
	Channel        string
	ConversationID *string
	OccurredAt     time.Time
	Status         string // unassigned | provisional
	Reason         string
	ProjectID      *uuid.UUID
	Source         string
	Confidence     *float64 // scorer confidence; nil when never scored
	ThreadCount    int      // queued messages this row stands for; always >= 1
	NotRelevantAt  *time.Time
}

// UnassignedListFilter narrows the unassigned queue.
type UnassignedListFilter struct {
	// Status is unassigned | provisional | all | not_relevant.
	// The first three describe the queue and all exclude not-relevant items;
	// not_relevant returns only those.
	Status string
	Limit  int
	Offset int
}

// UnassignedSummary counts for nav badge.
type UnassignedSummary struct {
	Unassigned  int
	Provisional int
	// NotRelevant is reported separately and excluded from the badge.
	NotRelevant int
}

// ManualItemRow is a pasted correspondence item.
type ManualItemRow struct {
	ID               uuid.UUID
	OrganisationID   uuid.UUID
	Channel          string
	OccurredAt       time.Time
	Title            string
	BodyText         string
	ProjectID        *uuid.UUID
	AssignmentStatus string
	AssignmentReason *string
	AssignmentSource *string
	CreatedByUserID  uuid.UUID
	CreatedAt        time.Time
	NotRelevantAt    *time.Time
}

// TimelineContact is a compact contact summary on a timeline row.
type TimelineContact struct {
	ID          uuid.UUID
	DisplayName string
	Role        string
}

// TimelineItem is one row on a project timeline.
type TimelineItem struct {
	Source             string // mail | manual | slack
	OccurredAt         time.Time
	Title              string
	Snippet            string
	Contacts           []TimelineContact
	AccountID          *uuid.UUID
	AccountLabel       string
	MessageID          *uuid.UUID
	ManualItemID       *uuid.UUID
	ConnectorMessageID *uuid.UUID
	ConnectorAccountID *uuid.UUID
	Channel            string
	BodyText           string // manual evidence; empty for mail list view
	IssueID            *uuid.UUID
}

// TimelineFilter narrows timeline listing.
type TimelineFilter struct {
	Source            string // mail | manual | slack | all
	UnassignedToIssue bool
	Limit             int
	Offset            int
}

// ManualItemRepository persists pasted correspondence.
type ManualItemRepository interface {
	CreateManualItem(ctx context.Context, row ManualItemRow) error
	GetManualItem(ctx context.Context, organisationID, id uuid.UUID) (*ManualItemRow, error)
	// UpdateManualItemAssignment writes the whole assignment decision, including
	// notRelevantAt, so a project and a dismissal cannot both be set.
	UpdateManualItemAssignment(ctx context.Context, organisationID, id uuid.UUID, projectID *uuid.UUID, status, reason, source string, notRelevantAt *time.Time) error
	ListManualItemsForProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]ManualItemRow, error)
	ListUnassignedManualItems(ctx context.Context, organisationID uuid.UUID, limit int) ([]ManualItemRow, error)
}

// TimelineRepository builds unified project timelines.
type TimelineRepository interface {
	ListProjectTimeline(ctx context.Context, userID, organisationID, projectID uuid.UUID, filter TimelineFilter) ([]TimelineItem, error)
}

// AssignmentRepository persists thread/message project assignments.
type AssignmentRepository interface {
	UpsertThreadAssignment(ctx context.Context, row AssignmentRow) error
	// UpsertThreadAssignments writes many thread assignments, chunking so a
	// single transaction stays well inside the DSQL 3,000 mutated-row cap.
	UpsertThreadAssignments(ctx context.Context, rows []AssignmentRow) error
	GetThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) (*AssignmentRow, error)
	DeleteThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) error
	UpsertMessageOverride(ctx context.Context, row AssignmentRow) error
	// UpsertMessageOverrides writes many per-message overrides, chunked as above.
	UpsertMessageOverrides(ctx context.Context, rows []AssignmentRow) error
	GetMessageOverride(ctx context.Context, messageID uuid.UUID) (*AssignmentRow, error)
	DeleteMessageOverride(ctx context.Context, messageID uuid.UUID) error
	EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*EffectiveAssignment, error)
	EffectiveProjectIDsForMessages(ctx context.Context, userID uuid.UUID, messages []domainmessages.MessageRow) (map[uuid.UUID]*uuid.UUID, error)
	ListUnassigned(ctx context.Context, userID uuid.UUID, filter UnassignedListFilter) ([]UnassignedItem, error)
	CountUnassignedSummary(ctx context.Context, userID uuid.UUID) (UnassignedSummary, error)
	// ListMessagesNeedingAssign returns messages automatic assignment may
	// (re)place, newest first: those with no assignment, and those whose only
	// assignment is a machine's provisional suggestion. Anything a user decided
	// — a project, a dismissal, a clear — is never returned.
	ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, filter AssignCandidateFilter) ([]domainmessages.MessageRow, error)
	FindCommittedSiblingProject(ctx context.Context, userID, accountID uuid.UUID, conversationID string, excludeMessageID uuid.UUID) (*uuid.UUID, error)
}
