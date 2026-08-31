package jobstoretest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type Factory func(t *testing.T) (driven.JobStore, func())

type stubExecutor struct {
	typ    string
	chunks []driven.ChunkResult
	calls  int
}

func (s *stubExecutor) JobType() string { return s.typ }

func (s *stubExecutor) ExecuteChunk(_ context.Context, _ driven.RunContext) (driven.ChunkResult, error) {
	if s.calls >= len(s.chunks) {
		return driven.ChunkResult{Done: true}, nil
	}
	r := s.chunks[s.calls]
	s.calls++
	if r.Retryable {
		return r, errors.New(r.ErrorMessage)
	}
	if r.ErrorMessage != "" && !r.Done {
		return r, errors.New(r.ErrorMessage)
	}
	return r, nil
}

func RunContractTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("state_machine_chain", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		user := uuid.New()
		account := uuid.New()
		enq := &jobs.Enqueuer{Store: store}
		first, err := enq.EnqueueChain(ctx, user, &account, driven.JobTriggerAPI, []string{
			jobs.TypeSync, jobs.TypeResolveContacts, jobs.TypeCategorize,
		}, driven.JobPayload{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if first.Status != driven.JobStatusPending {
			t.Fatalf("expected pending, got %s", first.Status)
		}

		exec := &jobs.ExecutionService{
			Store: store,
			Executors: jobs.ExecutorRegistry{
				jobs.TypeSync: &stubExecutor{typ: jobs.TypeSync, chunks: []driven.ChunkResult{
					{NextCursor: &driven.JobCursor{Kind: "graph_next_link", Value: "p2"}, ProgressDelta: driven.JobProgress{Processed: 100}},
					{Done: true, ProgressDelta: driven.JobProgress{Processed: 50}},
				}},
				jobs.TypeResolveContacts: &stubExecutor{typ: jobs.TypeResolveContacts, chunks: []driven.ChunkResult{
					{Done: true, ProgressDelta: driven.JobProgress{Processed: 10}},
				}},
				jobs.TypeCategorize: &stubExecutor{typ: jobs.TypeCategorize, chunks: []driven.ChunkResult{
					{Done: true, ProgressDelta: driven.JobProgress{Processed: 5}},
				}},
			},
			LeaseOwner: "test",
		}

		if err := exec.HandleStreamRecord(ctx, first.ID, now); err != nil {
			t.Fatal(err)
		}
		running, _ := store.GetByID(ctx, first.ID)
		if running.Status != driven.JobStatusRunning || running.AttemptID == nil {
			t.Fatalf("expected running with attempt, got %+v", running)
		}
		if err := exec.HandleStreamRecord(ctx, first.ID, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		mid, _ := store.GetByID(ctx, first.ID)
		if mid.Cursor == nil || mid.Cursor.Value != "p2" {
			t.Fatalf("expected cursor advanced, got %+v", mid.Cursor)
		}
		if err := exec.HandleStreamRecord(ctx, first.ID, now.Add(2*time.Second)); err != nil {
			t.Fatal(err)
		}
		doneSync, _ := store.GetByID(ctx, first.ID)
		if doneSync.Status != driven.JobStatusSuccess {
			t.Fatalf("expected sync success, got %s", doneSync.Status)
		}
		nextID := jobs.DeterministicJobID(first.ChainID, 1, jobs.TypeResolveContacts)
		next, err := store.GetByID(ctx, nextID)
		if err != nil {
			t.Fatal(err)
		}
		if next.Status != driven.JobStatusPending || next.JobType != jobs.TypeResolveContacts {
			t.Fatalf("unexpected next job %+v", next)
		}

		for _, id := range []uuid.UUID{nextID, jobs.DeterministicJobID(first.ChainID, 2, jobs.TypeCategorize)} {
			if err := exec.HandleStreamRecord(ctx, id, now); err != nil {
				t.Fatal(err)
			}
			if err := exec.HandleStreamRecord(ctx, id, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
		}
		last, _ := store.GetByID(ctx, jobs.DeterministicJobID(first.ChainID, 2, jobs.TypeCategorize))
		if last.Status != driven.JobStatusSuccess {
			t.Fatalf("expected categorize success, got %s", last.Status)
		}
	})

	t.Run("stale_attempt_fence", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		user := uuid.New()
		job, err := store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeDraftSuggest, UserID: user, TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		running, err := store.KickPending(ctx, job.ID, job.Revision, "a", now.Add(time.Minute), now)
		if err != nil {
			t.Fatal(err)
		}
		staleAttempt := *running.AttemptID
		staleRev := running.Revision
		recovered, err := store.RecoverExpiredLease(ctx, job.ID, staleRev, staleAttempt, now.Add(2*time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Status != driven.JobStatusPending {
			t.Fatalf("expected pending after recover, got %s", recovered.Status)
		}
		_, err = store.AdvanceRunning(ctx, job.ID, staleRev, staleAttempt, nil, driven.JobProgress{}, now.Add(3*time.Minute), now.Add(3*time.Minute))
		if !errors.Is(err, driven.ErrJobConflict) {
			t.Fatalf("expected conflict for stale attempt, got %v", err)
		}
	})

	t.Run("cancel_pending_and_running", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		user := uuid.New()
		pending, _ := store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeCategorize, UserID: user, TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: now,
		})
		cancelled, err := store.RequestCancel(ctx, user, pending.ID, now, jobs.DefaultTerminalTTL)
		if err != nil {
			t.Fatal(err)
		}
		if cancelled.Status != driven.JobStatusCancelled {
			t.Fatalf("expected cancelled pending, got %s", cancelled.Status)
		}

		job, _ := store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeCategorize, UserID: user, TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: now,
		})
		running, _ := store.KickPending(ctx, job.ID, job.Revision, "w", now.Add(time.Minute), now)
		marked, err := store.RequestCancel(ctx, user, running.ID, now, jobs.DefaultTerminalTTL)
		if err != nil {
			t.Fatal(err)
		}
		if marked.Status != driven.JobStatusRunning || marked.CancelRequestedAt == nil {
			t.Fatalf("expected cancel requested while running, got %+v", marked)
		}
		final, err := store.CancelRunning(ctx, marked.ID, marked.Revision, *marked.AttemptID, now, jobs.DefaultTerminalTTL)
		if err != nil {
			t.Fatal(err)
		}
		if final.Status != driven.JobStatusCancelled {
			t.Fatalf("expected cancelled, got %s", final.Status)
		}
	})

	t.Run("lock_race", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		user := uuid.New()
		account := uuid.New()
		_, err := store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeSync, UserID: user, AccountID: &account,
			TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(),
			AcquireLock: true, LockScope: "mailbox", LockKey: account.String(), Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeSync, UserID: user, AccountID: &account,
			TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(),
			AcquireLock: true, LockScope: "mailbox", LockKey: account.String(), Now: now,
		})
		if !errors.Is(err, driven.ErrJobLockHeld) {
			t.Fatalf("expected lock held, got %v", err)
		}
	})

	t.Run("effect_claim_once", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		account := uuid.New()
		_, err := store.ClaimEffect(ctx, driven.ClaimEffectInput{
			AccountID: account, EffectKey: "FORWARD#m1#r1", JobID: uuid.New(), AttemptID: uuid.New(), Now: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.ClaimEffect(ctx, driven.ClaimEffectInput{
			AccountID: account, EffectKey: "FORWARD#m1#r1", JobID: uuid.New(), AttemptID: uuid.New(), Now: now,
		})
		if !errors.Is(err, driven.ErrEffectAlreadyClaimed) {
			t.Fatalf("expected already claimed, got %v", err)
		}
	})

	t.Run("list_rejects_offset", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		_, err := store.List(context.Background(), driven.JobListFilter{UserID: uuid.New(), Offset: 10})
		if !errors.Is(err, driven.ErrOffsetNotSupported) {
			t.Fatalf("expected offset error, got %v", err)
		}
	})

	t.Run("list_paginates_with_authenticated_cursor", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		user := uuid.New()
		account := uuid.New()
		otherUser := uuid.New()
		base := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
		for i := 0; i < 3; i++ {
			_, err := store.CreatePending(ctx, driven.CreateJobInput{
				ID: uuid.New(), JobType: jobs.TypeSync, UserID: user, AccountID: &account, TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: base.Add(time.Duration(i) * time.Minute),
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err := store.CreatePending(ctx, driven.CreateJobInput{
			ID: uuid.New(), JobType: jobs.TypeSync, UserID: otherUser, AccountID: &account, TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: base.Add(4 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}

		page1, err := store.List(ctx, driven.JobListFilter{UserID: user, AccountID: &account, Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(page1.Jobs) != 2 {
			t.Fatalf("expected 2 jobs on first page, got %d", len(page1.Jobs))
		}
		if page1.Jobs[0].CreatedAt.Before(page1.Jobs[1].CreatedAt) {
			t.Fatalf("expected newest first ordering")
		}
		if page1.NextCursor == "" {
			t.Fatalf("expected next cursor")
		}

		page2, err := store.List(ctx, driven.JobListFilter{UserID: user, AccountID: &account, Limit: 2, Cursor: page1.NextCursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(page2.Jobs) != 1 {
			t.Fatalf("expected 1 job on second page, got %d", len(page2.Jobs))
		}

		tampered := page1.NextCursor[:len(page1.NextCursor)-1] + "A"
		_, err = store.List(ctx, driven.JobListFilter{UserID: user, AccountID: &account, Limit: 2, Cursor: tampered})
		if err == nil || !strings.Contains(err.Error(), "cursor") {
			t.Fatalf("expected cursor validation error, got %v", err)
		}

		_, err = store.List(ctx, driven.JobListFilter{UserID: otherUser, AccountID: &account, Limit: 2, Cursor: page1.NextCursor})
		if err == nil || !strings.Contains(err.Error(), "cursor") {
			t.Fatalf("expected cursor filter binding error, got %v", err)
		}
	})
}

func RunCrashWindowTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("chain_handoff_is_idempotent", func(t *testing.T) {
		store, cleanup := factory(t)
		defer cleanup()

		ctx := context.Background()
		now := time.Now().UTC()
		user := uuid.New()
		account := uuid.New()
		first, err := (&jobs.Enqueuer{Store: store}).EnqueueChain(ctx, user, &account, driven.JobTriggerAPI, []string{
			jobs.TypeSync, jobs.TypeCategorize,
		}, driven.JobPayload{}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		running, err := store.KickPending(ctx, first.ID, first.Revision, "worker", now.Add(time.Minute), now)
		if err != nil {
			t.Fatal(err)
		}
		attemptID := *running.AttemptID
		revision := running.Revision
		nextType := running.RemainingJobs[0]
		nextID := jobs.DeterministicJobID(running.ChainID, running.StepIndex+1, nextType)
		_, err = store.CompleteStep(ctx, running.ID, revision, attemptID, driven.JobProgress{Processed: 1}, &driven.CreateJobInput{
			ID:             nextID,
			JobType:        nextType,
			UserID:         running.UserID,
			AccountID:      running.AccountID,
			TriggerKind:    running.TriggerKind,
			ChainID:        running.ChainID,
			StepIndex:      running.StepIndex + 1,
			RemainingJobs:  nil,
			ScheduleID:     running.ScheduleID,
			ScheduledFor:   running.ScheduledFor,
			ChainStartedAt: running.ChainStartedAt,
			Payload:        running.Payload,
			Now:            now,
		}, now, jobs.DefaultTerminalTTL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.CompleteStep(ctx, running.ID, revision, attemptID, driven.JobProgress{Processed: 1}, nil, now, jobs.DefaultTerminalTTL)
		if !errors.Is(err, driven.ErrJobConflict) {
			t.Fatalf("expected stale complete conflict, got %v", err)
		}
		next, err := store.GetByID(ctx, nextID)
		if err != nil {
			t.Fatal(err)
		}
		if next.Status != driven.JobStatusPending {
			t.Fatalf("expected pending next job, got %s", next.Status)
		}
	})
}
