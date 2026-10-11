package connectors

import (
	"context"
	"github.com/google/uuid"
	"time"
)

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

// SlackOAuthResult is the identity and token returned by Slack OAuth v2.
type SlackOAuthResult struct {
	TeamID       string
	TeamName     string
	AccessToken  string
	RefreshToken string
	Scopes       []string
}

// SlackConversation is a channel visible to the connected bot.
type SlackConversation struct {
	ID   string
	Name string
}

// SlackMessage is one channel history event.
type SlackMessage struct {
	ProviderEventID string
	Text            string
	AuthorLabel     string
	OccurredAt      time.Time
}

// SlackHistoryPage is one page of channel history.
type SlackHistoryPage struct {
	Messages   []SlackMessage
	NextCursor string
}

// SlackClient is the outbound Slack OAuth and Web API port.
type SlackClient interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (*SlackOAuthResult, error)
	ListConversations(ctx context.Context, accessToken string) ([]SlackConversation, error)
	FetchHistory(ctx context.Context, accessToken, channelID, cursor string) (*SlackHistoryPage, error)
}
