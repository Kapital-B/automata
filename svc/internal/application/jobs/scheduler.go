package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

const (
	DefaultSchedulerBatchLimit = 100
	DefaultPendingWakeAfter    = 2 * time.Minute
)

type ScheduleCASRepository interface {
	MarkScheduleExecutedIfDue(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) (bool, error)
}

type EffectAuditReconciler interface {
	BackfillEffectAudit(ctx context.Context, effect driven.EffectRecord, now time.Time) (bool, error)
}

type ChainEnqueuer interface {
	EnqueueChain(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, trigger string, chain []string, payload driven.JobPayload, scheduleID *uuid.UUID, scheduledFor *time.Time) (*driven.JobRecord, error)
}

type SchedulerService struct {
	OAuthStates       driven.OAuthStateRepository
	Schedules         driven.ScheduleRepository
	Accounts          driven.AccountRepository
	Store             driven.JobStore
	Enqueuer          ChainEnqueuer
	Registry          *Registry
	EffectReconciler  EffectAuditReconciler
	OAuthStateTTL     time.Duration
	PendingWakeAfter  time.Duration
	ScheduleBatchSize int
}

func (s *SchedulerService) Tick(ctx context.Context, now time.Time) error {
	if s == nil {
		return nil
	}
	now = now.UTC()
	if err := s.deleteExpiredOAuthStates(ctx, now); err != nil {
		return err
	}
	if err := s.enqueueDueSchedules(ctx, now); err != nil {
		return err
	}
	if err := s.rewakePending(ctx, now); err != nil {
		return err
	}
	if err := s.recoverExpiredLeases(ctx, now); err != nil {
		return err
	}
	return s.reconcileEffects(ctx, now)
}

func (s *SchedulerService) deleteExpiredOAuthStates(ctx context.Context, now time.Time) error {
	if s.OAuthStates == nil {
		return nil
	}
	ttl := s.OAuthStateTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return s.OAuthStates.DeleteExpiredStates(ctx, now.Add(-ttl))
}

func (s *SchedulerService) enqueueDueSchedules(ctx context.Context, now time.Time) error {
	if s.Schedules == nil || s.Accounts == nil || s.Store == nil {
		return nil
	}
	due, err := s.Schedules.ListDueSchedules(ctx, now, s.batchLimit())
	if err != nil {
		return err
	}
	enq := s.Enqueuer
	if enq == nil {
		enq = &Enqueuer{Store: s.Store, Registry: s.Registry}
	}
	for _, chain := range due {
		if len(chain.Jobs) == 0 || chain.IntervalMinutes <= 0 || !chain.Enabled {
			continue
		}
		accountIDs, err := s.targetAccounts(ctx, chain)
		if err != nil {
			return err
		}
		scheduledFor := chain.NextRunAt.UTC()
		for _, accountID := range accountIDs {
			if _, err := enq.EnqueueChain(ctx, chain.UserID, &accountID, driven.JobTriggerSchedule, chain.Jobs, driven.JobPayload{}, &chain.ID, &scheduledFor); err != nil {
				return err
			}
		}
		nextRunAt := scheduledFor.Add(time.Duration(chain.IntervalMinutes) * time.Minute)
		if err := s.markScheduleExecuted(ctx, chain.ID, scheduledFor, now, nextRunAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SchedulerService) rewakePending(ctx context.Context, now time.Time) error {
	if s.Store == nil {
		return nil
	}
	stale, err := s.Store.ListStalePending(ctx, now.Add(-s.pendingWakeAfter()), s.batchLimit())
	if err != nil {
		return err
	}
	for _, job := range stale {
		if _, err := s.Store.ReWakePending(ctx, job.ID, job.Revision, now); err != nil && err != driven.ErrJobConflict {
			return err
		}
	}
	return nil
}

func (s *SchedulerService) recoverExpiredLeases(ctx context.Context, now time.Time) error {
	if s.Store == nil {
		return nil
	}
	stale, err := s.Store.ListExpiredLeases(ctx, now, s.batchLimit())
	if err != nil {
		return err
	}
	for _, job := range stale {
		if job.AttemptID == nil {
			continue
		}
		if _, err := s.Store.RecoverExpiredLease(ctx, job.ID, job.Revision, *job.AttemptID, now); err != nil && err != driven.ErrJobConflict {
			return err
		}
	}
	return nil
}

func (s *SchedulerService) reconcileEffects(ctx context.Context, now time.Time) error {
	if s.Store == nil {
		return nil
	}
	for _, state := range []string{
		driven.EffectSucceededPendingAudit,
		driven.EffectClaimed,
		driven.EffectUnknown,
	} {
		effects, err := s.Store.ListEffectsByState(ctx, state, now, s.batchLimit())
		if err != nil {
			return err
		}
		for _, effect := range effects {
			switch effect.State {
			case driven.EffectSucceededPendingAudit:
				if s.EffectReconciler == nil {
					continue
				}
				ok, err := s.EffectReconciler.BackfillEffectAudit(ctx, effect, now)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if _, err := s.Store.UpdateEffect(ctx, effect.AccountID, effect.EffectKey, effect.Revision, driven.EffectSucceeded, effect.AuditJSON, now); err != nil && err != driven.ErrJobConflict {
					return err
				}
			case driven.EffectClaimed:
				updated, err := s.Store.UpdateEffect(ctx, effect.AccountID, effect.EffectKey, effect.Revision, driven.EffectUnknown, effect.AuditJSON, now)
				if err != nil {
					if err == driven.ErrJobConflict {
						continue
					}
					return err
				}
				if s.EffectReconciler != nil {
					if _, err := s.EffectReconciler.BackfillEffectAudit(ctx, *updated, now); err != nil {
						return err
					}
				}
			case driven.EffectUnknown:
				if s.EffectReconciler == nil {
					continue
				}
				if _, err := s.EffectReconciler.BackfillEffectAudit(ctx, effect, now); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *SchedulerService) targetAccounts(ctx context.Context, chain driven.ScheduleChainRow) ([]uuid.UUID, error) {
	if chain.AccountID != nil {
		return []uuid.UUID{*chain.AccountID}, nil
	}
	accounts, err := s.Accounts.ListAccounts(ctx, chain.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(accounts))
	for _, a := range accounts {
		if strings.EqualFold(a.ConnectionStatus, "connected") {
			out = append(out, a.ID)
		}
	}
	return out, nil
}

func (s *SchedulerService) markScheduleExecuted(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) error {
	if repo, ok := s.Schedules.(ScheduleCASRepository); ok {
		_, err := repo.MarkScheduleExecutedIfDue(ctx, id, scheduledFor, lastRunAt, nextRunAt)
		return err
	}
	if s.Schedules == nil {
		return nil
	}
	return s.Schedules.MarkScheduleExecuted(ctx, id, lastRunAt, nextRunAt)
}

func (s *SchedulerService) batchLimit() int {
	if s.ScheduleBatchSize <= 0 {
		return DefaultSchedulerBatchLimit
	}
	if s.ScheduleBatchSize > DefaultSchedulerBatchLimit {
		return DefaultSchedulerBatchLimit
	}
	return s.ScheduleBatchSize
}

func (s *SchedulerService) pendingWakeAfter() time.Duration {
	if s.PendingWakeAfter <= 0 {
		return DefaultPendingWakeAfter
	}
	return s.PendingWakeAfter
}

func ValidateScheduleChain(chain []string, reg *Registry) error {
	if reg == nil {
		reg = DefaultRegistry()
	}
	_, err := reg.ValidateChain(chain)
	if err != nil {
		return fmt.Errorf("invalid schedule chain: %w", err)
	}
	return nil
}
