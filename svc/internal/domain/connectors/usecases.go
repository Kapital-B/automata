package connectors

import (
	context "context"
	errors "errors"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	uuid "github.com/google/uuid"
)

type ConnectorSyncOptions struct {
	RunID   *uuid.UUID
	Trigger string
}

var (
	ConnectorErrInvalidOAuthState = errors.New("invalid oauth state")
	ConnectorErrNotFound          = errors.New("connector not found")
)

type ConnectorStartConnectOutput struct {
	AuthorizationURL string
	State            string
}

type ConnectorCompleteOAuthResult struct {
	ConnectorAccountID uuid.UUID
}

type ConnectorStartConnectInput struct {
	Provider  string
	LabelHint *string
}

type ConnectorSyncResult struct {
	JobRunID         uuid.UUID
	MessagesUpserted int
}

type ConnectorCreateBindingInput struct {
	ExternalChannelID string
	OrganisationID    *uuid.UUID
	ProjectID         *uuid.UUID
	Label             string
}

type ConnectorConnectorSyncChunkResult struct {
	MessagesUpserted int
	NextCursor       *domainjobs.JobCursor
	Done             bool
}

type ConnectorUseCases interface {
	StartConnect(ctx context.Context, userID uuid.UUID, in ConnectorStartConnectInput) (*ConnectorStartConnectOutput, error)
	CompleteOAuth(ctx context.Context, code, state string) (*ConnectorCompleteOAuthResult, error)
	List(ctx context.Context, userID uuid.UUID) ([]ConnectorAccountRow, error)
	Disconnect(ctx context.Context, userID, connectorAccountID uuid.UUID) error
	ListBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]ConnectorBindingRow, error)
	CreateBinding(ctx context.Context, userID, connectorAccountID uuid.UUID, in ConnectorCreateBindingInput) (*ConnectorBindingRow, error)
	Sync(ctx context.Context, userID, connectorAccountID uuid.UUID) (*ConnectorSyncResult, error)
	SyncConnector(ctx context.Context, userID, connectorAccountID uuid.UUID) (*ConnectorSyncResult, error)
	SyncConnectorChunk(ctx context.Context, run domainjobs.RunContext) (*ConnectorConnectorSyncChunkResult, error)
	SyncConnectorWithOptions(
		ctx context.Context,
		userID, connectorAccountID uuid.UUID,
		options ConnectorSyncOptions,
	) (*ConnectorSyncResult, error)
}
