package jobs

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type LegacyJobRequest struct {
	RunID              uuid.UUID
	UserID             uuid.UUID
	AccountID          uuid.UUID
	ConnectorAccountID uuid.UUID
	TriggerKind        string
	MessageID          *uuid.UUID
	Force              bool
	Recategorize       bool
	RemainingJobs      []string
	ScheduleID         *uuid.UUID
	ChainStartedAt     *time.Time
}

type LegacyQueue interface {
	EnqueueSync(ctx context.Context, in LegacyJobRequest) error
	EnqueueCategorize(ctx context.Context, in LegacyJobRequest) error
	EnqueueSummarize(ctx context.Context, in LegacyJobRequest) error
	EnqueueDraftSuggest(ctx context.Context, in LegacyJobRequest) error
	EnqueueForwardRules(ctx context.Context, in LegacyJobRequest) error
	EnqueueSyncSlack(ctx context.Context, in LegacyJobRequest) error
}

type LegacyExecutionPort interface {
	HandleLegacy(ctx context.Context, kind string, in LegacyJobRequest) error
}
