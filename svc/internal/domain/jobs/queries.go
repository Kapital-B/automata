package jobs

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type JobRunsQueries interface {
	InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error
	UpdateJobRunStatus(ctx context.Context, id uuid.UUID, status string, finishedAt *time.Time, errMsg *string, metaJSON string) error
	ListJobRuns(ctx context.Context, userID uuid.UUID, filter JobRunListFilter) ([]JobRunRow, error)
	GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*JobRunRow, error)
}

type JobStoreQueries interface {
	CancelRun(ctx context.Context, user, id uuid.UUID) (*JobRecord, error)
	Get(ctx context.Context, userID, jobID uuid.UUID) (*JobRecord, error)
	List(ctx context.Context, filter JobListFilter) (*JobListPage, error)
	RequestCancel(ctx context.Context, userID, jobID uuid.UUID, now time.Time, terminalTTL time.Duration) (*JobRecord, error)
}

type SchedulesQueries interface {
	SaveSchedules(ctx context.Context, user uuid.UUID, inputs []ScheduleInput) error
	ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]ScheduleChainRow, error)
	ReplaceSchedulesByUser(ctx context.Context, userID uuid.UUID, rows []ScheduleChainRow) error
}
