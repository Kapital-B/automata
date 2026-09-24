package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	OAuthStates driven.OAuthStateRepository
	Schedules   driven.ScheduleRepository
	Accounts    driven.AccountRepository
	// Projects is optional; without it the extraction debounce pass is skipped.
	Projects         driven.ProjectRepository
	Store            driven.JobStore
	Enqueuer         ChainEnqueuer
	Registry         *Registry
	EffectReconciler EffectAuditReconciler
	OAuthStateTTL    time.Duration
	PendingWakeAfter time.Duration
	// ExtractQuietFor and ExtractCeiling debounce project extraction: run once
	// correspondence has stopped arriving for ExtractQuietFor, or once the
	// oldest unextracted item reaches ExtractCeiling, whichever comes first.
	ExtractQuietFor   time.Duration
	ExtractCeiling    time.Duration
	ScheduleBatchSize int
	Log               *slog.Logger
}

func (s *SchedulerService) log() *slog.Logger {
	if s != nil && s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *SchedulerService) Tick(ctx context.Context, now time.Time) error {
	if s == nil {
		return nil
	}
	now = now.UTC()
	start := time.Now().UTC()
	s.log().Info("scheduler tick begin", "now", now)
	if err := s.deleteExpiredOAuthStates(ctx, now); err != nil {
		s.log().Error("scheduler oauth cleanup failed", "err", err)
		return err
	}
	if err := s.enqueueDueSchedules(ctx, now); err != nil {
		s.log().Error("scheduler enqueue due failed", "err", err)
		return err
	}
	if err := s.rewakePending(ctx, now); err != nil {
		s.log().Error("scheduler rewake pending failed", "err", err)
		return err
	}
	if err := s.recoverExpiredLeases(ctx, now); err != nil {
		s.log().Error("scheduler lease recovery failed", "err", err)
		return err
	}
	if err := s.reconcileEffects(ctx, now); err != nil {
		s.log().Error("scheduler effect reconcile failed", "err", err)
		return err
	}
	if err := s.enqueueDueExtractions(ctx, now); err != nil {
		s.log().Error("scheduler extraction enqueue failed", "err", err)
		return err
	}
	s.log().Info("scheduler tick end", "duration_ms", time.Since(start).Milliseconds())
	return nil
}

// DefaultExtractQuietFor and DefaultExtractCeiling implement the debounce in
// addendum-project-renovation.md §2.2.
const (
	DefaultExtractQuietFor = time.Minute
	DefaultExtractCeiling  = 5 * time.Minute
)

// enqueueDueExtractions starts the interpret→reconcile chain for projects whose
// correspondence has settled.
//
// Due-ness is derived from assignment timestamps rather than tracked, so a
// missed tick self-heals and there is no counter to drift.
func (s *SchedulerService) enqueueDueExtractions(ctx context.Context, now time.Time) error {
	if s.Projects == nil || s.Enqueuer == nil {
		return nil
	}
	quiet := s.ExtractQuietFor
	if quiet <= 0 {
		quiet = DefaultExtractQuietFor
	}
	ceiling := s.ExtractCeiling
	if ceiling <= 0 {
		ceiling = DefaultExtractCeiling
	}
	due, err := s.Projects.ListProjectsDueForExtraction(ctx, now, quiet, ceiling, s.batchLimit())
	if err != nil {
		return err
	}
	enqueued, coalesced := 0, 0
	for _, project := range due {
		pid := project.ProjectID
		_, err := s.Enqueuer.EnqueueChain(ctx, project.OwnerUserID, nil, driven.JobTriggerSchedule,
			[]string{TypeInterpretProject, TypeReconcileProject},
			driven.JobPayload{ProjectID: &pid}, nil, nil)
		if err != nil {
			// Already queued or running for this project: the coalescing
			// working, not a failure.
			if errors.Is(err, driven.ErrJobLockHeld) || errors.Is(err, driven.ErrJobConflict) {
				coalesced++
				continue
			}
			return err
		}
		enqueued++
	}
	if len(due) > 0 {
		s.log().Info("scheduler extraction", "due", len(due), "enqueued", enqueued, "coalesced", coalesced)
	}
	return nil
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

func (s *SchedulerService) registry() *Registry {
	if s.Registry != nil {
		return s.Registry
	}
	return DefaultRegistry()
}

func (s *SchedulerService) enqueueDueSchedules(ctx context.Context, now time.Time) error {
	if s.Schedules == nil || s.Accounts == nil || s.Store == nil {
		s.log().Info("scheduler enqueue skipped", "reason", "schedules/accounts/store not configured")
		return nil
	}
	due, err := s.Schedules.ListDueSchedules(ctx, now, s.batchLimit())
	if err != nil {
		return err
	}
	s.log().Info("scheduler due schedules", "count", len(due))
	enq := s.Enqueuer
	if enq == nil {
		enq = &Enqueuer{Store: s.Store, Registry: s.Registry}
	}
	enqueued, skippedLock, skippedEmpty, skippedInvalid := 0, 0, 0, 0
	for _, chain := range due {
		if len(chain.Jobs) == 0 || chain.IntervalMinutes <= 0 || !chain.Enabled {
			skippedEmpty++
			continue
		}
		scheduledFor := chain.NextRunAt.UTC()
		jobs, err := s.registry().NormalizeScheduleChain(chain.Jobs)
		if err != nil {
			// A chain nothing can run is skipped and moved on, not returned:
			// returning stopped every schedule behind it, and the chain,
			// never marked run, stayed due and failed the same way each tick.
			skippedInvalid++
			s.log().Error("scheduler skipped invalid chain", "schedule_id", chain.ID, "user_id", chain.UserID, "jobs", chain.Jobs, "err", err)
			if err := s.markScheduleExecuted(ctx, chain.ID, scheduledFor, now, scheduledFor.Add(time.Duration(chain.IntervalMinutes)*time.Minute)); err != nil {
				return err
			}
			continue
		}
		accountIDs, err := s.targetAccounts(ctx, chain)
		if err != nil {
			return err
		}
		for _, accountID := range accountIDs {
			if _, err := enq.EnqueueChain(ctx, chain.UserID, &accountID, driven.JobTriggerSchedule, jobs, driven.JobPayload{}, &chain.ID, &scheduledFor); err != nil {
				if errors.Is(err, driven.ErrJobLockHeld) {
					// Another active/pending job owns the mailbox lock — skip this
					// account and keep ticking so pending rewake/lease recovery run.
					skippedLock++
					s.log().Info("scheduler enqueue skipped lock held",
						"schedule_id", chain.ID,
						"account_id", accountID,
						"user_id", chain.UserID,
						"jobs", chain.Jobs,
					)
					continue
				}
				return err
			}
			enqueued++
			s.log().Info("scheduler enqueued chain",
				"schedule_id", chain.ID,
				"account_id", accountID,
				"user_id", chain.UserID,
				"jobs", chain.Jobs,
				"scheduled_for", scheduledFor,
			)
		}
		nextRunAt := scheduledFor.Add(time.Duration(chain.IntervalMinutes) * time.Minute)
		if err := s.markScheduleExecuted(ctx, chain.ID, scheduledFor, now, nextRunAt); err != nil {
			return err
		}
		s.log().Info("scheduler marked executed", "schedule_id", chain.ID, "next_run_at", nextRunAt)
	}
	s.log().Info("scheduler enqueue summary",
		"due", len(due),
		"enqueued", enqueued,
		"skipped_lock", skippedLock,
		"skipped_empty", skippedEmpty,
		"skipped_invalid", skippedInvalid,
	)
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
	rewoke, conflicts := 0, 0
	for _, job := range stale {
		if _, err := s.Store.ReWakePending(ctx, job.ID, job.Revision, now); err != nil {
			if err == driven.ErrJobConflict {
				conflicts++
				continue
			}
			return err
		}
		rewoke++
		s.log().Info("scheduler rewoke pending", "job_id", job.ID, "job_type", job.JobType, "revision", job.Revision)
	}
	if len(stale) > 0 {
		s.log().Info("scheduler rewake summary", "stale", len(stale), "rewoke", rewoke, "conflicts", conflicts)
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
	recovered, conflicts, skipped := 0, 0, 0
	for _, job := range stale {
		if job.AttemptID == nil {
			skipped++
			continue
		}
		if _, err := s.Store.RecoverExpiredLease(ctx, job.ID, job.Revision, *job.AttemptID, now); err != nil {
			if err == driven.ErrJobConflict {
				conflicts++
				continue
			}
			return err
		}
		recovered++
		s.log().Info("scheduler recovered lease", "job_id", job.ID, "job_type", job.JobType, "attempt_id", *job.AttemptID)
	}
	if len(stale) > 0 {
		s.log().Info("scheduler lease recovery summary", "expired", len(stale), "recovered", recovered, "conflicts", conflicts, "skipped", skipped)
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
		if len(effects) == 0 {
			continue
		}
		s.log().Info("scheduler reconcile effects", "state", state, "count", len(effects))
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
