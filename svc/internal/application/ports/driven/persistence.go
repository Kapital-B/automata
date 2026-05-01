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

// MessageRow is a stored inbox message.
type MessageRow struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	ProviderMessageID  string
	ConversationID     *string
	ReceivedAt         time.Time
	Subject            string
	FromJSON           string
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
	AccountID           *uuid.UUID
	Category            string
	Since               *time.Time
	OnlySummaryUnseen   bool
	OnlyForwardUnseen   bool
	Limit               int
	Offset              int
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

// MessageRepository stores Graph messages.
type MessageRepository interface {
	UpsertMessage(ctx context.Context, m MessageRow) error
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
	GetUserByEmail(ctx context.Context, email string) (id uuid.UUID, passwordHash *string, err error)
	GetUserByID(ctx context.Context, id uuid.UUID) (email string, err error)
	FindIdentity(ctx context.Context, provider, providerSubject string) (userID uuid.UUID, ok bool, err error)
	AttachIdentity(ctx context.Context, id uuid.UUID, userID uuid.UUID, provider, providerSubject, emailAtLink string, now time.Time) error
}

// JobRunRepository records sync runs (Phase 1: synchronous insert).
type JobRunRepository interface {
	InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error
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
}
