package jobs

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type ScheduleCASRepository interface {
	MarkScheduleExecutedIfDue(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) (bool, error)
}

type EffectAuditReconciler interface {
	BackfillEffectAudit(ctx context.Context, effect EffectRecord, now time.Time) (bool, error)
}

type ChainEnqueuer interface {
	EnqueueChain(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, trigger string, chain []string, payload JobPayload, scheduleID *uuid.UUID, scheduledFor *time.Time) (*JobRecord, error)
}
