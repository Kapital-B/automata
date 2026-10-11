package jobs

import (
	"context"
	"github.com/google/uuid"
)

type JobRunsQueries interface {
	ListJobRuns(ctx context.Context, userID uuid.UUID, filter JobRunListFilter) ([]JobRunRow, error)
	GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*JobRunRow, error)
}

type JobStoreQueries interface {
	CancelRun(ctx context.Context, user, id uuid.UUID) (*JobRecord, error)
	Get(ctx context.Context, userID, jobID uuid.UUID) (*JobRecord, error)
	List(ctx context.Context, filter JobListFilter) (*JobListPage, error)
}

type SchedulesQueries interface {
	SaveSchedules(ctx context.Context, user uuid.UUID, inputs []ScheduleInput) error
	ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]ScheduleChainRow, error)
}
