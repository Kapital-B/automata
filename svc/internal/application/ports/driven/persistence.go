package driven

import (
	"context"
	"time"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// AccountRow is persistence shape for API responses (not a rich domain entity).
type AccountRow struct {
	UserID            uuid.UUID
	ID                uuid.UUID
	Label             string
	Provider          string
	MsAccountKind     accounts.MsAccountKind
	GraphTenantID     *string
	PrimaryEmail      string
	MsalHomeAccountID *string
	ConnectionStatus  string
	LastError         *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastSyncedAt      *time.Time
}

// ConnectorAccountRow is a connected non-mail provider account.
type ConnectorAccountRow struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Provider         string
	Label            string
	ExternalTenantID *string
	ConnectionStatus string
	LastError        *string
	Scopes           []string
	LastSyncedAt     *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ConnectorBindingRow assigns one provider channel to a project.
type ConnectorBindingRow struct {
	ID                 uuid.UUID
	ConnectorAccountID uuid.UUID
	OrganisationID     uuid.UUID
	ExternalChannelID  string
	ProjectID          *uuid.UUID
	Label              string
	SyncCursor         *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ConnectorMessageRow is one persisted provider event.
type ConnectorMessageRow struct {
	ID                 uuid.UUID
	ConnectorAccountID uuid.UUID
	OrganisationID     uuid.UUID
	ProjectID          *uuid.UUID
	ProviderEventID    string
	ExternalChannelID  string
	Title              string
	BodyText           string
	AuthorLabel        string
	OccurredAt         time.Time
	MetaJSON           string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// MessageRow is a stored inbox message.
type MessageRow struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	ProviderMessageID  string
	ConversationID     *string
	ReceivedAt         time.Time
	Subject            string
	FromJSON           string
	ToJSON             string
	CcJSON             string
	ToCCPreview        *string
	BodyText           *string
	BodyFetchedAt      *time.Time
	HasAttachments     bool
	RawEtag            *string
	CategorySlug       *string
	CategoryConfidence *float64
	SummarySeenAt      *time.Time
	ForwardSeenAt      *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type CategoryDefinitionRow struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Slug        string
	DisplayName string
	Definition  string
	SortOrder   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MessageCategoryRow struct {
	ID         uuid.UUID
	MessageID  uuid.UUID
	AccountID  uuid.UUID
	CategoryID uuid.UUID
	Source     string
	Confidence *float64
	RunID      uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MessageListFilter struct {
	AccountID         *uuid.UUID
	ProjectID         *uuid.UUID
	Category          string
	Since             *time.Time
	OnlySummaryUnseen bool
	OnlyForwardUnseen bool
	Limit             int
	Offset            int
}

// JobRunRow is the persisted job run shape for API responses and auditing.
type JobRunRow struct {
	ID              uuid.UUID
	AccountID       *uuid.UUID
	AccountLabel    *string
	JobType         string
	TriggerKind     string
	Status          string
	TimeWindowStart *time.Time
	TimeWindowEnd   *time.Time
	StartedAt       time.Time
	FinishedAt      *time.Time
	ErrorMessage    *string
	MetaJSON        string
}

// JobRunListFilter narrows a run listing for one user.
type JobRunListFilter struct {
	AccountID *uuid.UUID
	JobType   string
	Limit     int
	Offset    int
}

type SummarySettingsRow struct {
	UserID               uuid.UUID
	IncludeCategorySlugs []string
	ExcludeCategorySlugs []string
	ChunkSize            int
	UpdatedAt            time.Time
}

type SummarySnapshotRow struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	AccountID      *uuid.UUID
	RunID          uuid.UUID
	WindowStart    time.Time
	WindowEnd      time.Time
	GeneralSummary string
	CreatedAt      time.Time
}

type SummaryJobChunkRow struct {
	ID          uuid.UUID
	RunID       uuid.UUID
	AccountID   uuid.UUID
	ChunkIndex  int
	Phase       string
	MessageIDs  []uuid.UUID
	PayloadJSON string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ActionItemRow struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	AccountID       uuid.UUID
	MessageID       uuid.UUID
	RunID           uuid.UUID
	Text            string
	DueAt           *time.Time
	Status          string
	ActionedAt      *time.Time
	AutoDraftSeenAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type FYIRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	RunID     uuid.UUID
	Text      string
	CreatedAt time.Time
}

type DraftSuggestionRow struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	AccountID    uuid.UUID
	MessageID    uuid.UUID
	ActionItemID uuid.UUID
	RunID        uuid.UUID
	Subject      string
	Body         string
	Model        string
	FromJSON     string
	Status       string
	SentAt       *time.Time
	DiscardedAt  *time.Time
	UpdatedAt    *time.Time
	CreatedAt    time.Time
}

type SendAttemptRow struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	AccountID         uuid.UUID
	DraftID           uuid.UUID
	MessageID         uuid.UUID
	Status            string
	ProviderMessageID *string
	ErrorMessage      *string
	CreatedAt         time.Time
}

type ScheduleChainRow struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Name            string
	AccountID       *uuid.UUID
	Jobs            []string
	IntervalMinutes int
	Enabled         bool
	LastRunAt       *time.Time
	NextRunAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ForwardAllowlistRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Email     string
	CreatedAt time.Time
}

type ForwardRuleRow struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	AccountID     uuid.UUID
	Name          string
	Mode          string
	ConditionJSON string
	ForwardTo     string
	Enabled       bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type ForwardAuditRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	RuleID    uuid.UUID
	RunID     uuid.UUID
	Status    string
	Reason    *string
	CreatedAt time.Time
}

type ManualForwardAuditRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	AccountID uuid.UUID
	MessageID uuid.UUID
	ToEmail   string
	Comment   *string
	Status    string
	Reason    *string
	CreatedAt time.Time
}

// AccountRepository persists accounts and OAuth tokens (ciphertext).
type AccountRepository interface {
	InsertAccount(ctx context.Context, a AccountRow, tokenCiphertext []byte) error
	UpdateAccountTokens(ctx context.Context, userID uuid.UUID, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error
	GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*AccountRow, []byte, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]AccountRow, error)
	DeleteAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	GetSyncDeltaLink(ctx context.Context, userID uuid.UUID, accountID uuid.UUID) (*string, error)
	UpsertSyncState(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, deltaLink *string, at time.Time) error
	UpsertSyncStateTime(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, at time.Time) error
}

// ConnectorRepository persists connector accounts, channel bindings, and events.
type ConnectorRepository interface {
	InsertConnectorAccount(ctx context.Context, row ConnectorAccountRow, tokenCiphertext []byte) error
	GetConnectorAccount(ctx context.Context, userID, id uuid.UUID) (*ConnectorAccountRow, error)
	ListConnectorAccounts(ctx context.Context, userID uuid.UUID) ([]ConnectorAccountRow, error)
	DeleteConnectorAccount(ctx context.Context, userID, id uuid.UUID) error
	GetConnectorTokenCipher(ctx context.Context, userID, id uuid.UUID) ([]byte, error)
	UpdateConnectorToken(ctx context.Context, userID, id uuid.UUID, tokenCiphertext []byte, scopes []string, status string, lastError *string, lastSyncedAt *time.Time) error
	CreateConnectorBinding(ctx context.Context, row ConnectorBindingRow) error
	GetConnectorBinding(ctx context.Context, userID, id uuid.UUID) (*ConnectorBindingRow, error)
	ListConnectorBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]ConnectorBindingRow, error)
	DeleteConnectorBinding(ctx context.Context, userID, id uuid.UUID) error
	UpdateConnectorBindingCursor(ctx context.Context, userID, id uuid.UUID, cursor *string, updatedAt time.Time) error
	UpsertConnectorMessage(ctx context.Context, row ConnectorMessageRow) error
	ListConnectorMessagesForProject(ctx context.Context, userID, organisationID, projectID uuid.UUID) ([]ConnectorMessageRow, error)
	GetConnectorMessage(ctx context.Context, userID, id uuid.UUID) (*ConnectorMessageRow, error)
}

// MessageRepository stores Graph messages.
type MessageRepository interface {
	UpsertMessage(ctx context.Context, m MessageRow) error
	GetMessageIDByProvider(ctx context.Context, accountID uuid.UUID, providerMessageID string) (uuid.UUID, error)
	ListMessagesByAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, limit, offset int) ([]MessageRow, error)
	ListMessages(ctx context.Context, userID uuid.UUID, filter MessageListFilter) ([]MessageRow, error)
	GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*MessageRow, error)
	MarkMessagesSummarySeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error
	MarkMessagesForwardSeen(ctx context.Context, userID uuid.UUID, messageIDs []uuid.UUID, at time.Time) error
	UpsertMessageCategory(ctx context.Context, row MessageCategoryRow) error
	ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]CategoryDefinitionRow, error)
	GetCategoryDefinitionBySlug(ctx context.Context, userID uuid.UUID, slug string) (*CategoryDefinitionRow, error)
	GetCategoryDefinitionByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*CategoryDefinitionRow, error)
	CreateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	UpdateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error)
	CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error)
}

// OAuthStateRepository stores one-time OAuth state values (mail connect + auth flows).
type OAuthStateRepository interface {
	InsertOAuthState(ctx context.Context, state, flow string, userID *uuid.UUID, payloadJSON string, createdAt time.Time) error
	TakeOAuthState(ctx context.Context, state string) (flow string, userID *uuid.UUID, payloadJSON string, ok bool, err error)
	DeleteExpiredStates(ctx context.Context, before time.Time) error
}

// UserRepository persists app users and external identities.
type UserRepository interface {
	CreateUser(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time) error
	// CreateUserWithHomeOrg inserts the user, a Personal organisation, owner membership,
	// and optional password/oauth identity in one transaction. identityProvider empty skips identity.
	CreateUserWithHomeOrg(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time, identityProvider, identitySubject, identityEmail string) (orgID uuid.UUID, err error)
	GetUserByEmail(ctx context.Context, email string) (id uuid.UUID, passwordHash *string, err error)
	GetUserByID(ctx context.Context, id uuid.UUID) (email string, err error)
	GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	FindIdentity(ctx context.Context, provider, providerSubject string) (userID uuid.UUID, ok bool, err error)
	AttachIdentity(ctx context.Context, id uuid.UUID, userID uuid.UUID, provider, providerSubject, emailAtLink string, now time.Time) error
}

// OrganisationRow is a persisted organisation.
type OrganisationRow struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// OrganisationRepository reads organisations and membership.
type OrganisationRepository interface {
	GetOrganisation(ctx context.Context, id uuid.UUID) (*OrganisationRow, error)
}

// ContactRow is a persisted address-book contact.
type ContactRow struct {
	ID                  uuid.UUID
	OrganisationID      uuid.UUID
	DisplayName         string
	Company             *string
	MergedIntoContactID *uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ContactIdentityRow is one identity on a contact.
type ContactIdentityRow struct {
	ID              uuid.UUID
	OrganisationID  uuid.UUID
	ContactID       uuid.UUID
	Kind            string
	ValueNormalized string
	ValueRaw        string
	CreatedAt       time.Time
}

// CorrespondenceParticipantRow links a contact to a message (or later manual item).
type CorrespondenceParticipantRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ContactID      uuid.UUID
	Role           string
	MessageID      *uuid.UUID
	ManualItemID   *uuid.UUID
}

// ContactListFilter narrows contact listing within one organisation.
type ContactListFilter struct {
	Query  string
	Limit  int
	Offset int
}

// ContactRepository persists org-scoped contacts and mail participants.
type ContactRepository interface {
	ListContacts(ctx context.Context, organisationID uuid.UUID, filter ContactListFilter) ([]ContactRow, error)
	GetContact(ctx context.Context, organisationID, contactID uuid.UUID) (*ContactRow, error)
	CreateContact(ctx context.Context, row ContactRow) error
	UpdateContactDisplayNameIfEmpty(ctx context.Context, organisationID, contactID uuid.UUID, displayName string, at time.Time) error
	ListIdentities(ctx context.Context, organisationID, contactID uuid.UUID) ([]ContactIdentityRow, error)
	FindContactIdentity(ctx context.Context, organisationID uuid.UUID, kind, valueNormalized string) (*ContactIdentityRow, error)
	CreateIdentity(ctx context.Context, row ContactIdentityRow) error
	UpsertParticipant(ctx context.Context, row CorrespondenceParticipantRow) error
	ListRecentMessageIDs(ctx context.Context, organisationID, contactID uuid.UUID, limit int) ([]uuid.UUID, error)
	SuggestMerges(ctx context.Context, organisationID, contactID uuid.UUID) ([]ContactRow, error)
	MergeContacts(ctx context.Context, organisationID, survivorID, sourceID uuid.UUID, at time.Time) error
	// ResolveEmailContact finds or creates a contact for an email in the organisation.
	ResolveEmailContact(ctx context.Context, organisationID uuid.UUID, email, displayName string, now time.Time) (contactID uuid.UUID, err error)
	ListMessageIDsForAccount(ctx context.Context, accountID uuid.UUID, limit int) ([]uuid.UUID, error)
	ListContactIDsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]uuid.UUID, error)
	ListContactIDsForThread(ctx context.Context, organisationID, accountID uuid.UUID, conversationID string) ([]uuid.UUID, error)
	ListContactIDsForManualItem(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]uuid.UUID, error)
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

// ProjectListFilter narrows project listing.
type ProjectListFilter struct {
	IncludeArchived bool
	Limit           int
	Offset          int
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
}

// UnassignedItem is a mail or manual row for the Unassigned queue.
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
}

// UnassignedListFilter narrows the unassigned queue.
type UnassignedListFilter struct {
	Status string // unassigned | provisional | all
	Limit  int
	Offset int
}

// UnassignedSummary counts for nav badge.
type UnassignedSummary struct {
	Unassigned  int
	Provisional int
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
}

// IssueItemRow links correspondence to an issue.
type IssueItemRow struct {
	ID           uuid.UUID
	IssueID      uuid.UUID
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AddedAt      time.Time
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
	FindIssueIDByMessage(ctx context.Context, messageID uuid.UUID) (*uuid.UUID, error)
	FindIssueIDByManualItem(ctx context.Context, manualItemID uuid.UUID) (*uuid.UUID, error)
}

// FactRow is a stable project fact identity (subject lineage).
type FactRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ProjectID      uuid.UUID
	IssueID        *uuid.UUID
	SubjectKey     string
	Label          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// FactVersionRow is one version of a fact value.
type FactVersionRow struct {
	ID                    uuid.UUID
	FactID                uuid.UUID
	Status                string
	ValueJSON             string
	ValueText             string
	Unit                  *string
	Source                string
	Confidence            *float64
	InterpretationID      *uuid.UUID
	SupersedesVersionID   *uuid.UUID
	SupersededByVersionID *uuid.UUID
	SupersededAt          *time.Time
	CreatedByUserID       *uuid.UUID
	CreatedAt             time.Time
}

// FactEvidenceRow links correspondence evidence to a fact version.
type FactEvidenceRow struct {
	ID            uuid.UUID
	FactVersionID uuid.UUID
	MessageID     *uuid.UUID
	ManualItemID  *uuid.UUID
	AddedAt       time.Time
}

// FactRepository persists facts, versions, and evidence links.
type FactRepository interface {
	CreateFact(ctx context.Context, row FactRow) error
	UpdateFact(ctx context.Context, row FactRow) error
	GetFact(ctx context.Context, organisationID, factID uuid.UUID) (*FactRow, error)
	GetFactBySubject(ctx context.Context, organisationID, projectID uuid.UUID, subjectKey string) (*FactRow, error)
	ListFactsByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]FactRow, error)
	CreateFactVersion(ctx context.Context, row FactVersionRow) error
	GetFactVersion(ctx context.Context, organisationID, versionID uuid.UUID) (*FactVersionRow, error)
	ListFactVersions(ctx context.Context, factID uuid.UUID) ([]FactVersionRow, error)
	GetActiveFactVersion(ctx context.Context, factID uuid.UUID) (*FactVersionRow, error)
	UpdateFactVersion(ctx context.Context, row FactVersionRow) error
	AddFactEvidence(ctx context.Context, row FactEvidenceRow) error
	RemoveFactEvidence(ctx context.Context, organisationID, versionID, evidenceID uuid.UUID) error
	ListFactEvidence(ctx context.Context, versionID uuid.UUID) ([]FactEvidenceRow, error)
	ListFactEvidenceForFact(ctx context.Context, factID uuid.UUID) ([]FactEvidenceRow, error)
}

// InterpretationRow is one LLM interpret run (candidates in payload).
type InterpretationRow struct {
	ID             uuid.UUID
	OrganisationID uuid.UUID
	ProjectID      uuid.UUID
	AccountID      *uuid.UUID
	RunID          *uuid.UUID
	Status         string
	PayloadJSON    string
	Confidence     *float64
	Reason         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// InterpretationSourceRow links correspondence used as interpret input.
type InterpretationSourceRow struct {
	ID                 uuid.UUID
	InterpretationID   uuid.UUID
	MessageID          *uuid.UUID
	ManualItemID       *uuid.UUID
	ConnectorMessageID *uuid.UUID
}

// InterpretationRepository persists interpretation candidates and sources.
type InterpretationRepository interface {
	CreateInterpretation(ctx context.Context, row InterpretationRow) error
	AddInterpretationSource(ctx context.Context, row InterpretationSourceRow) error
	GetInterpretation(ctx context.Context, organisationID, id uuid.UUID) (*InterpretationRow, error)
	ListPendingInterpretations(ctx context.Context, organisationID, projectID uuid.UUID) ([]InterpretationRow, error)
	UpdateInterpretationStatus(ctx context.Context, organisationID, id uuid.UUID, status string, updatedAt time.Time) error
	ListInterpretationSources(ctx context.Context, interpretationID uuid.UUID) ([]InterpretationSourceRow, error)
}

// ContradictionRow is an open or resolved conflict between claims.
type ContradictionRow struct {
	ID               uuid.UUID
	OrganisationID   uuid.UUID
	ProjectID        uuid.UUID
	Status           string
	Summary          string
	ResolutionNote   *string
	ResolvedAt       *time.Time
	ResolvedByUserID *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ContradictionSideRow links one side of a contradiction.
type ContradictionSideRow struct {
	ID              uuid.UUID
	ContradictionID uuid.UUID
	FactVersionID   *uuid.UUID
	DecisionID      *uuid.UUID
}

// ContradictionRepository persists contradictions.
type ContradictionRepository interface {
	CreateContradiction(ctx context.Context, row ContradictionRow) error
	AddContradictionSide(ctx context.Context, row ContradictionSideRow) error
	GetContradiction(ctx context.Context, organisationID, id uuid.UUID) (*ContradictionRow, error)
	ListContradictionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]ContradictionRow, error)
	UpdateContradiction(ctx context.Context, row ContradictionRow) error
	ListContradictionSides(ctx context.Context, contradictionID uuid.UUID) ([]ContradictionSideRow, error)
}

// DecisionRow is a project decision (proposed through withdrawn).
type DecisionRow struct {
	ID                   uuid.UUID
	OrganisationID       uuid.UUID
	ProjectID            uuid.UUID
	IssueID              *uuid.UUID
	Statement            string
	Status               string
	DecidedAt            *time.Time
	AssigneeUserID       *uuid.UUID
	AssigneeContactID    *uuid.UUID
	Source               string
	Confidence           *float64
	SupersedesDecisionID *uuid.UUID
	CreatedByUserID      *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// DecisionEvidenceRow links correspondence to a decision.
type DecisionEvidenceRow struct {
	ID           uuid.UUID
	DecisionID   uuid.UUID
	MessageID    *uuid.UUID
	ManualItemID *uuid.UUID
	AddedAt      time.Time
}

// DecisionRepository persists decisions and evidence.
type DecisionRepository interface {
	CreateDecision(ctx context.Context, row DecisionRow) error
	GetDecision(ctx context.Context, organisationID, id uuid.UUID) (*DecisionRow, error)
	ListDecisionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]DecisionRow, error)
	UpdateDecision(ctx context.Context, row DecisionRow) error
	AddDecisionEvidence(ctx context.Context, row DecisionEvidenceRow) error
	RemoveDecisionEvidence(ctx context.Context, organisationID, decisionID, evidenceID uuid.UUID) error
	ListDecisionEvidence(ctx context.Context, decisionID uuid.UUID) ([]DecisionEvidenceRow, error)
}

// ManualItemRepository persists pasted correspondence.
type ManualItemRepository interface {
	CreateManualItem(ctx context.Context, row ManualItemRow) error
	GetManualItem(ctx context.Context, organisationID, id uuid.UUID) (*ManualItemRow, error)
	UpdateManualItemAssignment(ctx context.Context, organisationID, id uuid.UUID, projectID *uuid.UUID, status, reason, source string) error
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
	GetThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) (*AssignmentRow, error)
	DeleteThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) error
	UpsertMessageOverride(ctx context.Context, row AssignmentRow) error
	GetMessageOverride(ctx context.Context, messageID uuid.UUID) (*AssignmentRow, error)
	DeleteMessageOverride(ctx context.Context, messageID uuid.UUID) error
	EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*EffectiveAssignment, error)
	ListUnassigned(ctx context.Context, userID uuid.UUID, filter UnassignedListFilter) ([]UnassignedItem, error)
	CountUnassignedSummary(ctx context.Context, userID uuid.UUID) (UnassignedSummary, error)
	// ListMessagesNeedingAssign returns recent messages on an account with no effective project.
	ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]MessageRow, error)
	FindCommittedSiblingProject(ctx context.Context, userID, accountID uuid.UUID, conversationID string, excludeMessageID uuid.UUID) (*uuid.UUID, error)
}

// JobRunRepository records sync runs (Phase 1: synchronous insert).
type JobRunRepository interface {
	InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error
	// PromoteJobRunToRunning transitions a queued run from pending to running (worker pickup).
	// Idempotent if the run is already running. Fails if the row is missing or in a terminal state.
	PromoteJobRunToRunning(ctx context.Context, id uuid.UUID, startedAt time.Time) error
	UpdateJobRunMeta(ctx context.Context, id uuid.UUID, metaJSON string) error
	UpdateJobRunStatus(ctx context.Context, id uuid.UUID, status string, finishedAt *time.Time, errMsg *string, metaJSON string) error
	ListJobRuns(ctx context.Context, userID uuid.UUID, filter JobRunListFilter) ([]JobRunRow, error)
	GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*JobRunRow, error)
}

type SummaryRepository interface {
	GetSummarySettings(ctx context.Context, userID uuid.UUID) (*SummarySettingsRow, error)
	UpsertSummarySettings(ctx context.Context, row SummarySettingsRow) error
	InsertSummarySnapshot(ctx context.Context, row SummarySnapshotRow) error
	ListSummarySnapshots(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]SummarySnapshotRow, error)
	UpsertSummaryJobChunk(ctx context.Context, row SummaryJobChunkRow) error
	ListSummaryJobChunks(ctx context.Context, runID uuid.UUID) ([]SummaryJobChunkRow, error)
	InsertActionItems(ctx context.Context, rows []ActionItemRow) error
	ListOpenActionItems(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]ActionItemRow, error)
	MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error
	InsertFYI(ctx context.Context, rows []FYIRow) error
	ListFYIByRun(ctx context.Context, userID uuid.UUID, runID uuid.UUID) ([]FYIRow, error)
	ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]FYIRow, error)
	DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ListActionItemsForAutoDraft(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, onlyUnseen bool, limit int) ([]ActionItemRow, error)
	MarkActionItemsAutoDraftSeen(ctx context.Context, userID uuid.UUID, itemIDs []uuid.UUID, at time.Time) error
	InsertDraftSuggestions(ctx context.Context, rows []DraftSuggestionRow) error
	ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]DraftSuggestionRow, error)
	GetDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID) (*DraftSuggestionRow, error)
	UpdateDraftSuggestion(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, subject, body string, at time.Time) error
	MarkDraftSuggestionStatus(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, status string, at time.Time) error
	InsertSendAttempt(ctx context.Context, row SendAttemptRow) error
	ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]SendAttemptRow, error)
}

type ScheduleRepository interface {
	ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]ScheduleChainRow, error)
	ReplaceSchedulesByUser(ctx context.Context, userID uuid.UUID, rows []ScheduleChainRow) error
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]ScheduleChainRow, error)
	MarkScheduleExecuted(ctx context.Context, id uuid.UUID, lastRunAt, nextRunAt time.Time) error
}

type ForwardRepository interface {
	ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]ForwardAllowlistRow, error)
	ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error
	ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]ForwardRuleRow, error)
	CreateForwardRule(ctx context.Context, row ForwardRuleRow) error
	UpdateForwardRule(ctx context.Context, row ForwardRuleRow) error
	DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error
	ListForwardAuditByRun(ctx context.Context, userID, runID uuid.UUID) ([]ForwardAuditRow, error)
	InsertForwardAudit(ctx context.Context, row ForwardAuditRow) error
	InsertManualForwardAudit(ctx context.Context, row ManualForwardAuditRow) error
}
