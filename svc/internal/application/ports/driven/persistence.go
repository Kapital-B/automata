package driven

import (
	"context"
	"time"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// AccountRow is persistence shape for API responses (not a rich domain entity).
type AccountRow struct {
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
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// AccountRepository persists accounts and OAuth tokens (ciphertext).
type AccountRepository interface {
	InsertAccount(ctx context.Context, a AccountRow, tokenCiphertext []byte) error
	UpdateAccountTokens(ctx context.Context, id uuid.UUID, tokenCiphertext []byte, primaryEmail string, graphTenantID *string, msalHome *string, status string, lastErr *string) error
	GetAccount(ctx context.Context, id uuid.UUID) (*AccountRow, []byte, error)
	ListAccounts(ctx context.Context) ([]AccountRow, error)
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	UpsertSyncStateTime(ctx context.Context, accountID uuid.UUID, at time.Time) error
}

// MessageRepository stores Graph messages.
type MessageRepository interface {
	UpsertMessage(ctx context.Context, m MessageRow) error
	ListMessagesByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]MessageRow, error)
	GetMessage(ctx context.Context, id uuid.UUID) (*MessageRow, error)
}

// OAuthStateRepository stores one-time OAuth state values.
type OAuthStateRepository interface {
	InsertState(ctx context.Context, state string, kind accounts.MsAccountKind, labelHint *string, createdAt time.Time) error
	TakeState(ctx context.Context, state string) (kind accounts.MsAccountKind, labelHint *string, ok bool, err error)
	DeleteExpiredStates(ctx context.Context, before time.Time) error
}

// JobRunRepository records sync runs (Phase 1: synchronous insert).
type JobRunRepository interface {
	InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error
}
