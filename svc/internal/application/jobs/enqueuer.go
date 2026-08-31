package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// Enqueuer validates job types/chains and creates pending control-plane items.
type Enqueuer struct {
	Store    driven.JobStore
	Registry *Registry
}

func (e *Enqueuer) Enqueue(ctx context.Context, in driven.CreateJobInput) (*driven.JobRecord, error) {
	if e == nil || e.Store == nil {
		return nil, fmt.Errorf("job enqueuer not configured")
	}
	reg := e.Registry
	if reg == nil {
		reg = DefaultRegistry()
	}
	name, err := normalizeEnqueueType(in.JobType)
	if err != nil {
		return nil, err
	}
	name, err = reg.ValidateType(name)
	if err != nil {
		return nil, err
	}
	in.JobType = name
	remaining := make([]string, 0, len(in.RemainingJobs))
	for _, raw := range in.RemainingJobs {
		n, err := normalizeEnqueueType(raw)
		if err != nil {
			return nil, err
		}
		n, err = reg.ValidateType(n)
		if err != nil {
			return nil, err
		}
		remaining = append(remaining, n)
	}
	in.RemainingJobs = remaining
	if in.ChainID == uuid.Nil {
		in.ChainID = uuid.New()
	}
	if in.ID == uuid.Nil {
		if in.ScheduleID != nil && in.ScheduledFor != nil && in.StepIndex == 0 {
			in.ID = DeterministicScheduleJobID(*in.ScheduleID, in.ScheduledFor.UTC().Format(time.RFC3339), in.JobType)
		} else {
			in.ID = DeterministicJobID(in.ChainID, in.StepIndex, in.JobType)
		}
	}
	if in.TriggerKind == "" {
		in.TriggerKind = driven.JobTriggerAPI
	}
	def := reg.MustGet(name)
	if def.RequiresLock {
		in.AcquireLock = true
		if in.LockScope == "" {
			in.LockScope = def.LockScope
		}
		if in.LockKey == "" {
			if def.LockScope == "connector" && in.Payload.ConnectorAccountID != nil {
				in.LockKey = in.Payload.ConnectorAccountID.String()
			} else if in.AccountID != nil {
				in.LockKey = in.AccountID.String()
			}
		}
	}
	if in.Now.IsZero() {
		in.Now = time.Now().UTC()
	}
	if in.StepIndex == 0 && in.ChainStartedAt == nil {
		chainStartedAt := in.Now
		in.ChainStartedAt = &chainStartedAt
	}
	return e.Store.CreatePending(ctx, in)
}

// EnqueueChain creates the first pending step of a validated chain.
func (e *Enqueuer) EnqueueChain(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, trigger string, chain []string, payload driven.JobPayload, scheduleID *uuid.UUID, scheduledFor *time.Time) (*driven.JobRecord, error) {
	reg := e.Registry
	if reg == nil {
		reg = DefaultRegistry()
	}
	steps, err := reg.ValidateChain(chain)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var chainID uuid.UUID
	var jobID uuid.UUID
	if scheduleID != nil && scheduledFor != nil {
		when := scheduledFor.UTC().Format(time.RFC3339)
		chainID = DeterministicChainID(*scheduleID, when)
		jobID = DeterministicScheduleJobID(*scheduleID, when, steps[0])
	} else {
		chainID = uuid.New()
		jobID = DeterministicJobID(chainID, 0, steps[0])
	}
	remaining := append([]string(nil), steps[1:]...)
	return e.Enqueue(ctx, driven.CreateJobInput{
		ID:             jobID,
		JobType:        steps[0],
		UserID:         userID,
		AccountID:      accountID,
		TriggerKind:    trigger,
		ChainID:        chainID,
		StepIndex:      0,
		RemainingJobs:  remaining,
		ScheduleID:     scheduleID,
		ScheduledFor:   scheduledFor,
		ChainStartedAt: &now,
		Payload:        payload,
		Now:            now,
	})
}

var _ driven.JobEnqueuer = (*Enqueuer)(nil)
