package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

const (
	DefaultLeaseDuration = 960 * time.Second
	PersistReserve       = 30 * time.Second
)

// ExecutorRegistry maps job types to bounded chunk executors.
type ExecutorRegistry map[string]driven.JobExecutor

// ExecutionService owns job lifecycle transitions around chunk executors.
type ExecutionService struct {
	Store       driven.JobStore
	Executors   ExecutorRegistry
	LeaseOwner  string
	LeaseFor    time.Duration
	TerminalTTL time.Duration
}

func (s *ExecutionService) leaseDuration() time.Duration {
	if s.LeaseFor > 0 {
		return s.LeaseFor
	}
	return DefaultLeaseDuration
}

func (s *ExecutionService) terminalTTL() time.Duration {
	if s.TerminalTTL > 0 {
		return s.TerminalTTL
	}
	return DefaultTerminalTTL
}

func (s *ExecutionService) HandleStreamRecord(ctx context.Context, jobID uuid.UUID, now time.Time) error {
	if s == nil || s.Store == nil {
		return fmt.Errorf("job execution service not configured")
	}
	job, err := s.Store.GetByID(ctx, jobID)
	if err != nil {
		if errors.Is(err, driven.ErrJobNotFound) {
			return nil
		}
		return err
	}
	switch job.Status {
	case driven.JobStatusSuccess, driven.JobStatusFailed, driven.JobStatusCancelled:
		return nil
	case driven.JobStatusPending:
		owner := s.LeaseOwner
		if owner == "" {
			owner = "worker"
		}
		_, err := s.Store.KickPending(ctx, job.ID, job.Revision, owner, now.Add(s.leaseDuration()), now)
		if errors.Is(err, driven.ErrJobConflict) {
			return nil
		}
		return err
	case driven.JobStatusRunning:
		err := s.runChunk(ctx, job, now)
		if errors.Is(err, driven.ErrJobConflict) {
			return nil
		}
		return err
	default:
		return fmt.Errorf("unknown job status %q", job.Status)
	}
}

func (s *ExecutionService) runChunk(ctx context.Context, job *driven.JobRecord, now time.Time) error {
	if job.AttemptID == nil {
		return driven.ErrJobConflict
	}
	attemptID := *job.AttemptID
	if job.CancelRequestedAt != nil {
		_, err := s.Store.CancelRunning(ctx, job.ID, job.Revision, attemptID, now, s.terminalTTL())
		return err
	}
	exec, ok := s.Executors[job.JobType]
	if !ok {
		_, err := s.Store.FailJob(ctx, job.ID, job.Revision, attemptID, "unregistered job type: "+job.JobType, now, s.terminalTTL())
		return err
	}
	deadline := now.Add(s.leaseDuration() - PersistReserve)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl.Add(-PersistReserve)
		if !deadline.After(now) {
			deadline = now.Add(5 * time.Second)
		}
	}
	result, err := exec.ExecuteChunk(ctx, driven.RunContext{
		RunID:     job.ID,
		AttemptID: attemptID,
		UserID:    job.UserID,
		AccountID: job.AccountID,
		JobType:   job.JobType,
		Payload:   job.Payload,
		Cursor:    job.Cursor,
		Deadline:  deadline,
		Now:       now,
	})
	current, cerr := s.currentAttempt(ctx, job.ID, attemptID)
	if cerr != nil {
		return cerr
	}
	if err != nil {
		if result.Retryable {
			retryAt := now.Add(time.Minute)
			if result.RetryAfter != nil {
				retryAt = *result.RetryAfter
			}
			msg := result.ErrorMessage
			if msg == "" {
				msg = err.Error()
			}
			_, derr := s.Store.DeferRetry(ctx, current.ID, current.Revision, attemptID, retryAt, msg, now)
			return derr
		}
		msg := result.ErrorMessage
		if msg == "" {
			msg = err.Error()
		}
		_, ferr := s.Store.FailJob(ctx, current.ID, current.Revision, attemptID, msg, now, s.terminalTTL())
		return ferr
	}
	if current.CancelRequestedAt != nil {
		_, err := s.Store.CancelRunning(ctx, current.ID, current.Revision, attemptID, now, s.terminalTTL())
		return err
	}
	progress := current.Progress
	progress.Processed += result.ProgressDelta.Processed
	progress.Failed += result.ProgressDelta.Failed
	if result.ProgressDelta.Detail != nil {
		if progress.Detail == nil {
			progress.Detail = map[string]interface{}{}
		}
		for k, v := range result.ProgressDelta.Detail {
			progress.Detail[k] = v
		}
	}
	if !result.Done {
		_, err := s.Store.AdvanceRunning(ctx, current.ID, current.Revision, attemptID, result.NextCursor, progress, now.Add(s.leaseDuration()), now)
		return err
	}
	var next *driven.CreateJobInput
	if len(current.RemainingJobs) > 0 {
		nextType := current.RemainingJobs[0]
		remaining := append([]string(nil), current.RemainingJobs[1:]...)
		nextID := DeterministicJobID(current.ChainID, current.StepIndex+1, nextType)
		next = &driven.CreateJobInput{
			ID:             nextID,
			JobType:        nextType,
			UserID:         current.UserID,
			AccountID:      current.AccountID,
			TriggerKind:    current.TriggerKind,
			ChainID:        current.ChainID,
			StepIndex:      current.StepIndex + 1,
			RemainingJobs:  remaining,
			ScheduleID:     current.ScheduleID,
			ScheduledFor:   current.ScheduledFor,
			ChainStartedAt: current.ChainStartedAt,
			Payload:        current.Payload,
			Now:            now,
		}
	}
	_, err = s.Store.CompleteStep(ctx, current.ID, current.Revision, attemptID, progress, next, now, s.terminalTTL())
	return err
}

func (s *ExecutionService) currentAttempt(ctx context.Context, jobID, attemptID uuid.UUID) (*driven.JobRecord, error) {
	job, err := s.Store.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.Status != driven.JobStatusRunning || job.AttemptID == nil || *job.AttemptID != attemptID {
		return nil, driven.ErrJobConflict
	}
	return job, nil
}

func (s *ExecutionService) RunSynchronous(ctx context.Context, in driven.CreateJobInput, exec driven.JobExecutor) (*driven.JobRecord, error) {
	if s == nil || s.Store == nil {
		return nil, fmt.Errorf("job execution service not configured")
	}
	if exec == nil {
		return nil, fmt.Errorf("executor required")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	in.Now = now
	if in.JobType == "" {
		in.JobType = exec.JobType()
	} else {
		name, err := normalizeEnqueueType(in.JobType)
		if err != nil {
			return nil, err
		}
		if name != exec.JobType() {
			return nil, fmt.Errorf("executor type mismatch: %s != %s", exec.JobType(), in.JobType)
		}
	}
	enqueuer := Enqueuer{Store: s.Store}
	pending, err := enqueuer.Enqueue(ctx, in)
	if err != nil {
		return nil, err
	}
	owner := s.LeaseOwner
	if owner == "" {
		owner = "sync"
	}
	running, err := s.Store.KickPending(ctx, pending.ID, pending.Revision, owner, now.Add(s.leaseDuration()), now)
	if err != nil {
		return nil, err
	}
	if err := s.runChunk(ctx, running, now); err != nil {
		return s.Store.GetByID(ctx, running.ID)
	}
	return s.Store.GetByID(ctx, running.ID)
}

var _ driven.JobExecutionPort = (*ExecutionService)(nil)
