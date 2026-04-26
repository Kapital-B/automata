package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type schedulerService struct {
	log       *slog.Logger
	schedules driven.ScheduleRepository
	accounts  driven.AccountRepository
	jobRuns   driven.JobRunRepository
	queue     *asynqadapter.QueueClient
}

func (s *schedulerService) Tick(ctx context.Context, now time.Time) error {
	if s == nil || s.schedules == nil || s.accounts == nil || s.jobRuns == nil || s.queue == nil {
		return nil
	}
	due, err := s.schedules.ListDueSchedules(ctx, now.UTC(), 50)
	s.log.Info("due schedules", "count", len(due))
	if err != nil {
		return err
	}
	for _, chain := range due {
		if len(chain.Jobs) == 0 || chain.IntervalMinutes <= 0 {
			continue
		}
		accountIDs, err := s.targetAccounts(ctx, chain)
		if err != nil {
			continue
		}
		for _, accountID := range accountIDs {
			s.startChainForAccount(ctx, chain, accountID)
		}
		next := now.UTC().Add(time.Duration(chain.IntervalMinutes) * time.Minute)
		_ = s.schedules.MarkScheduleExecuted(ctx, chain.ID, now.UTC(), next)
	}
	return nil
}

func (s *schedulerService) targetAccounts(ctx context.Context, chain driven.ScheduleChainRow) ([]uuid.UUID, error) {
	if chain.AccountID != nil {
		return []uuid.UUID{*chain.AccountID}, nil
	}
	accounts, err := s.accounts.ListAccounts(ctx, chain.UserID)
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

func (s *schedulerService) startChainForAccount(ctx context.Context, chain driven.ScheduleChainRow, accountID uuid.UUID) {
	first := strings.TrimSpace(strings.ToLower(chain.Jobs[0]))
	if first == "" {
		return
	}
	rest := append([]string(nil), chain.Jobs[1:]...)
	runID := uuid.New()
	meta := `{"queued":true,"schedule_chain":true}`
	_ = s.jobRuns.InsertJobRun(ctx, runID, accountID, first, "schedule", "pending", time.Now().UTC(), time.Time{}, nil, meta)
	payload := asynqadapter.TaskPayload{
		SchemaVersion:  1,
		RunID:          runID,
		UserID:         chain.UserID,
		AccountID:      accountID,
		TriggerKind:    "schedule",
		RemainingJobs:  rest,
		ScheduleID:     &chain.ID,
		ChainStartedAt: timePtrScheduler(nowUTC()),
	}
	if err := asynqadapter.EnqueueByJobType(ctx, s.queue, first, payload); err != nil {
		msg := err.Error()
		_ = s.jobRuns.UpdateJobRunStatus(ctx, runID, "failed", timePtrScheduler(time.Now().UTC()), &msg, `{"queued":false}`)
		if s.log != nil {
			s.log.Error("enqueue scheduled chain", "schedule_id", chain.ID, "job_type", first, "account_id", accountID, "err", err)
		}
	}
}

func timePtrScheduler(t time.Time) *time.Time {
	return &t
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
