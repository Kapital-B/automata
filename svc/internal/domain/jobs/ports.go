package jobs

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"time"
)

const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusSuccess   = "success"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"

	JobTriggerAPI      = "api"
	JobTriggerSchedule = "schedule"

	EffectClaimed               = "claimed"
	EffectSucceededPendingAudit = "succeeded_pending_audit"
	EffectRetryable             = "retryable"
	EffectRejected              = "rejected"
	EffectUnknown               = "unknown"
	EffectSucceeded             = "succeeded"
)

var (
	ErrJobNotFound          = errors.New("job not found")
	ErrJobConflict          = errors.New("job conditional write conflict")
	ErrJobLockHeld          = errors.New("active job lock held")
	ErrEffectAlreadyClaimed = errors.New("effect already claimed")
	ErrOffsetNotSupported   = errors.New("offset pagination not supported; use cursor")
)

type JobCursor struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type JobProgress struct {
	Processed int                    `json:"processed"`
	Failed    int                    `json:"failed"`
	Detail    map[string]interface{} `json:"detail,omitempty"`
}

type JobPayload struct {
	ConnectorAccountID *uuid.UUID `json:"connector_account_id,omitempty"`
	MessageID          *uuid.UUID `json:"message_id,omitempty"`
	ProjectID          *uuid.UUID `json:"project_id,omitempty"`
	Recategorize       bool       `json:"recategorize,omitempty"`
	Force              bool       `json:"force,omitempty"`
	TimeWindowStart    *time.Time `json:"time_window_start,omitempty"`
	TimeWindowEnd      *time.Time `json:"time_window_end,omitempty"`
}

type JobRecord struct {
	ID                uuid.UUID
	JobType           string
	Status            string
	UserID            uuid.UUID
	AccountID         *uuid.UUID
	AccountLabel      *string
	TriggerKind       string
	ChainID           uuid.UUID
	StepIndex         int
	RemainingJobs     []string
	ScheduleID        *uuid.UUID
	ScheduledFor      *time.Time
	ChainStartedAt    *time.Time
	Cursor            *JobCursor
	Progress          JobProgress
	Payload           JobPayload
	ErrorMessage      *string
	ErrorCount        int
	CancelRequestedAt *time.Time
	RetryNotBefore    *time.Time
	Revision          int64
	AttemptID         *uuid.UUID
	LeaseOwner        *string
	LeaseUntil        *time.Time
	WakeToken         uuid.UUID
	CreatedAt         time.Time
	StartedAt         *time.Time
	UpdatedAt         time.Time
	FinishedAt        *time.Time
	ExpiresAt         *time.Time
	SchemaVersion     int
}

type CreateJobInput struct {
	ID             uuid.UUID
	JobType        string
	UserID         uuid.UUID
	AccountID      *uuid.UUID
	TriggerKind    string
	ChainID        uuid.UUID
	StepIndex      int
	RemainingJobs  []string
	ScheduleID     *uuid.UUID
	ScheduledFor   *time.Time
	ChainStartedAt *time.Time
	Payload        JobPayload
	AcquireLock    bool
	LockScope      string
	LockKey        string
	Now            time.Time
}

type JobListFilter struct {
	UserID    uuid.UUID
	AccountID *uuid.UUID
	JobType   string
	Limit     int
	Cursor    string
	Offset    int
}

type JobListPage struct {
	Jobs       []JobRecord
	NextCursor string
}

type EffectRecord struct {
	AccountID uuid.UUID
	EffectKey string
	State     string
	JobID     uuid.UUID
	AttemptID uuid.UUID
	AuditJSON string
	CreatedAt time.Time
	UpdatedAt time.Time
	Revision  int64
}

type ClaimEffectInput struct {
	AccountID uuid.UUID
	EffectKey string
	JobID     uuid.UUID
	AttemptID uuid.UUID
	Now       time.Time
}

type ChunkResult struct {
	NextCursor    *JobCursor
	ProgressDelta JobProgress
	Done          bool
	Retryable     bool
	RetryAfter    *time.Time
	ErrorMessage  string
}

type RunContext struct {
	RunID     uuid.UUID
	AttemptID uuid.UUID
	UserID    uuid.UUID
	AccountID *uuid.UUID
	JobType   string
	Payload   JobPayload
	Cursor    *JobCursor
	Deadline  time.Time
	Now       time.Time
}

// JobStore is the fenced job control-plane persistence port.
type JobStore interface {
	CreatePending(ctx context.Context, in CreateJobInput) (*JobRecord, error)
	Get(ctx context.Context, userID, jobID uuid.UUID) (*JobRecord, error)
	GetByID(ctx context.Context, jobID uuid.UUID) (*JobRecord, error)
	List(ctx context.Context, filter JobListFilter) (*JobListPage, error)

	KickPending(ctx context.Context, jobID uuid.UUID, expectedRevision int64, leaseOwner string, leaseUntil, now time.Time) (*JobRecord, error)
	AdvanceRunning(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, cursor *JobCursor, progress JobProgress, leaseUntil, now time.Time) (*JobRecord, error)
	CompleteStep(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, progress JobProgress, next *CreateJobInput, now time.Time, terminalTTL time.Duration) (*JobRecord, error)
	FailJob(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, errMsg string, now time.Time, terminalTTL time.Duration) (*JobRecord, error)
	RequestCancel(ctx context.Context, userID, jobID uuid.UUID, now time.Time, terminalTTL time.Duration) (*JobRecord, error)
	CancelRunning(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, now time.Time, terminalTTL time.Duration) (*JobRecord, error)
	DeferRetry(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, retryNotBefore time.Time, errMsg string, now time.Time) (*JobRecord, error)
	ReWakePending(ctx context.Context, jobID uuid.UUID, expectedRevision int64, now time.Time) (*JobRecord, error)
	RecoverExpiredLease(ctx context.Context, jobID uuid.UUID, expectedRevision int64, expectedAttemptID uuid.UUID, now time.Time) (*JobRecord, error)

	ListStalePending(ctx context.Context, olderThan time.Time, limit int) ([]JobRecord, error)
	ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]JobRecord, error)

	ClaimEffect(ctx context.Context, in ClaimEffectInput) (*EffectRecord, error)
	UpdateEffect(ctx context.Context, accountID uuid.UUID, effectKey string, expectedRevision int64, state string, auditJSON string, now time.Time) (*EffectRecord, error)
	GetEffect(ctx context.Context, accountID uuid.UUID, effectKey string) (*EffectRecord, error)
	ListEffectsByState(ctx context.Context, state string, updatedBefore time.Time, limit int) ([]EffectRecord, error)
}

// JobEnqueuer creates pending jobs for HTTP/scheduler adapters.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, in CreateJobInput) (*JobRecord, error)
}

// JobExecutor runs one registered bounded unit for a job type.
type JobExecutor interface {
	JobType() string
	ExecuteChunk(ctx context.Context, run RunContext) (ChunkResult, error)
}

// JobExecutionPort orchestrates lifecycle around JobExecutor results.
type JobExecutionPort interface {
	HandleStreamRecord(ctx context.Context, jobID uuid.UUID, now time.Time) error
	RunSynchronous(ctx context.Context, in CreateJobInput, exec JobExecutor) (*JobRecord, error)
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

type ScheduleRepository interface {
	ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]ScheduleChainRow, error)
	ReplaceSchedulesByUser(ctx context.Context, userID uuid.UUID, rows []ScheduleChainRow) error
	ListDueSchedules(ctx context.Context, now time.Time, limit int) ([]ScheduleChainRow, error)
	MarkScheduleExecuted(ctx context.Context, id uuid.UUID, lastRunAt, nextRunAt time.Time) error
}
