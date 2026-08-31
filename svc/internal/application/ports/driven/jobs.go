package driven

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
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
