package jobs

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type failOnceScheduleRepo struct {
	*sqlite.Repository
	fail bool
}

func (r *failOnceScheduleRepo) MarkScheduleExecutedIfDue(ctx context.Context, id uuid.UUID, scheduledFor, lastRunAt, nextRunAt time.Time) (bool, error) {
	if r.fail {
		r.fail = false
		return false, errors.New("simulated crash before schedule advance")
	}
	return r.Repository.MarkScheduleExecutedIfDue(ctx, id, scheduledFor, lastRunAt, nextRunAt)
}

type effectRecorder struct {
	seen []string
}

func (r *effectRecorder) BackfillEffectAudit(_ context.Context, effect driven.EffectRecord, _ time.Time) (bool, error) {
	r.seen = append(r.seen, effect.State+":"+effect.EffectKey)
	return true, nil
}

func TestSchedulerTickEnqueuesDueSchedules(t *testing.T) {
	ctx := context.Background()
	repo := newSchedulerSQLiteRepo(t)
	store := memoryjobs.NewStore()
	now := time.Date(2026, 8, 29, 21, 0, 0, 0, time.UTC)
	userID := uuid.New()
	accountID := uuid.New()
	insertSchedulerAccount(t, repo, userID, accountID, "mailbox")
	scheduleID := uuid.New()
	if err := repo.ReplaceSchedulesByUser(ctx, userID, []driven.ScheduleChainRow{{
		ID:              scheduleID,
		UserID:          userID,
		Name:            "nightly",
		AccountID:       &accountID,
		Jobs:            DefaultMailboxChain,
		IntervalMinutes: 60,
		Enabled:         true,
		NextRunAt:       now,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now.Add(-time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}

	service := SchedulerService{
		OAuthStates:   repo,
		Schedules:     repo,
		Accounts:      repo,
		Store:         store,
		OAuthStateTTL: 15 * time.Minute,
	}
	if err := service.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}

	jobID := DeterministicScheduleJobID(scheduleID, now.Format(time.RFC3339), TypeSync)
	job, err := store.GetByID(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != driven.JobStatusPending {
		t.Fatalf("expected pending scheduled job, got %s", job.Status)
	}
	rows, err := repo.ListSchedulesByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(rows))
	}
	if !rows[0].NextRunAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expected next run %s, got %s", now.Add(time.Hour), rows[0].NextRunAt)
	}
}

func TestSchedulerScheduleCreationCrashWindow(t *testing.T) {
	ctx := context.Background()
	baseRepo := newSchedulerSQLiteRepo(t)
	repo := &failOnceScheduleRepo{Repository: baseRepo, fail: true}
	store := memoryjobs.NewStore()
	now := time.Date(2026, 8, 29, 21, 0, 0, 0, time.UTC)
	userID := uuid.New()
	accountID := uuid.New()
	insertSchedulerAccount(t, baseRepo, userID, accountID, "mailbox")
	scheduleID := uuid.New()
	if err := baseRepo.ReplaceSchedulesByUser(ctx, userID, []driven.ScheduleChainRow{{
		ID:              scheduleID,
		UserID:          userID,
		Name:            "nightly",
		AccountID:       &accountID,
		Jobs:            []string{TypeSync},
		IntervalMinutes: 60,
		Enabled:         true,
		NextRunAt:       now,
		CreatedAt:       now.Add(-time.Hour),
		UpdatedAt:       now.Add(-time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}

	service := SchedulerService{Schedules: repo, Accounts: baseRepo, Store: store}
	if err := service.Tick(ctx, now); err == nil {
		t.Fatalf("expected simulated crash error")
	}

	jobID := DeterministicScheduleJobID(scheduleID, now.Format(time.RFC3339), TypeSync)
	if _, err := store.GetByID(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	if err := service.Tick(ctx, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	page, err := store.List(ctx, driven.JobListFilter{UserID: userID, AccountID: &accountID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Jobs) != 1 {
		t.Fatalf("expected single deterministic scheduled job, got %d", len(page.Jobs))
	}
}

func TestSchedulerRewakesPendingRecoversLeaseAndReconcilesEffects(t *testing.T) {
	ctx := context.Background()
	store := memoryjobs.NewStore()
	now := time.Date(2026, 8, 29, 21, 0, 0, 0, time.UTC)
	userID := uuid.New()
	accountID := uuid.New()

	pending, err := store.CreatePending(ctx, driven.CreateJobInput{
		ID: uuid.New(), JobType: TypeSync, UserID: userID, AccountID: &accountID,
		TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), AcquireLock: true, LockScope: "mailbox", LockKey: accountID.String(),
		Now: now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	initialPending, err := store.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}

	expired, err := store.CreatePending(ctx, driven.CreateJobInput{
		ID: uuid.New(), JobType: TypeCategorize, UserID: userID,
		TriggerKind: driven.JobTriggerAPI, ChainID: uuid.New(), Now: now.Add(-20 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	expired, err = store.KickPending(ctx, expired.ID, expired.Revision, "worker", now.Add(-time.Minute), now.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	effectID := uuid.New()
	effectAttempt := uuid.New()
	effect, err := store.ClaimEffect(ctx, driven.ClaimEffectInput{
		AccountID: accountID, EffectKey: "FORWARD#msg#rule", JobID: effectID, AttemptID: effectAttempt, Now: now.Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateEffect(ctx, accountID, effect.EffectKey, effect.Revision, driven.EffectSucceededPendingAudit, `{"status":"sent"}`, now.Add(-9*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimEffect(ctx, driven.ClaimEffectInput{
		AccountID: accountID, EffectKey: "FORWARD#msg2#rule", JobID: uuid.New(), AttemptID: uuid.New(), Now: now.Add(-8 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	recorder := &effectRecorder{}
	service := SchedulerService{
		Store:            store,
		PendingWakeAfter: time.Minute,
		EffectReconciler: recorder,
	}
	if err := service.Tick(ctx, now); err != nil {
		t.Fatal(err)
	}

	rewoken, err := store.GetByID(ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rewoken.WakeToken == initialPending.WakeToken {
		t.Fatalf("expected pending job wake token to rotate")
	}

	recovered, err := store.GetByID(ctx, expired.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != driven.JobStatusPending || recovered.AttemptID != nil {
		t.Fatalf("expected recovered pending job, got %+v", recovered)
	}

	succeededEffect, err := store.GetEffect(ctx, accountID, effect.EffectKey)
	if err != nil {
		t.Fatal(err)
	}
	if succeededEffect.State != driven.EffectSucceeded {
		t.Fatalf("expected effect settled to succeeded, got %s", succeededEffect.State)
	}

	unknownEffect, err := store.GetEffect(ctx, accountID, claimed.EffectKey)
	if err != nil {
		t.Fatal(err)
	}
	if unknownEffect.State != driven.EffectUnknown {
		t.Fatalf("expected stale claimed effect to become unknown, got %s", unknownEffect.State)
	}
	if len(recorder.seen) < 2 {
		t.Fatalf("expected effect reconciler callbacks, got %v", recorder.seen)
	}
}

func newSchedulerSQLiteRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return sqlite.NewRepository(db, 15*time.Minute)
}

func insertSchedulerAccount(t *testing.T, repo *sqlite.Repository, userID, accountID uuid.UUID, label string) {
	t.Helper()
	err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            label,
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     label + "@example.com",
		ConnectionStatus: "connected",
	}, []byte("cipher"))
	if err != nil {
		t.Fatal(err)
	}
}
