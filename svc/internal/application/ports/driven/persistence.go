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
	AccountID *uuid.UUID
	Category  string
	Since     *time.Time
	Limit     int
	Offset    int
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

// AccountRepository persists accounts and OAuth tokens (ciphertext).
type AccountRepository interface {
	InsertAccount(ctx context.Context, a AccountRow, tokenCiphertext []byte) error
	UpdateAccountTokens(ctx context.Context, userID uuid.UUID, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error
	GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*AccountRow, []byte, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]AccountRow, error)
	DeleteAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	UpsertSyncStateTime(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, at time.Time) error
}

// MessageRepository stores Graph messages.
type MessageRepository interface {
	UpsertMessage(ctx context.Context, m MessageRow) error
	ListMessagesByAccount(ctx context.Context, userID uuid.UUID, accountID uuid.UUID, limit, offset int) ([]MessageRow, error)
	ListMessages(ctx context.Context, userID uuid.UUID, filter MessageListFilter) ([]MessageRow, error)
	GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*MessageRow, error)
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
	ListJobRuns(ctx context.Context, userID uuid.UUID, filter JobRunListFilter) ([]JobRunRow, error)
	GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*JobRunRow, error)
}
