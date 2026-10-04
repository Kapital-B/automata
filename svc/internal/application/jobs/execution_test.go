package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

type cancellingExecutor struct {
	store driven.JobStore
	user  uuid.UUID
	now   time.Time
}

func (e *cancellingExecutor) JobType() string { return TypeCategorize }

func (e *cancellingExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	_, err := e.store.RequestCancel(ctx, e.user, run.RunID, e.now, DefaultTerminalTTL)
	if err != nil {
		return driven.ChunkResult{}, err
	}
	return driven.ChunkResult{
		Done:          false,
		NextCursor:    &driven.JobCursor{Kind: string(CursorMessageKeyset), Value: "after:1"},
		ProgressDelta: driven.JobProgress{Processed: 1},
	}, nil
}

func TestHandleStreamRecordCancelsAtChunkBoundary(t *testing.T) {
	ctx := context.Background()
	store := memoryjobs.NewStore()
	now := time.Now().UTC()
	userID := uuid.New()

	enqueuer := Enqueuer{Store: store}
	job, err := enqueuer.Enqueue(ctx, driven.CreateJobInput{
		JobType:     TypeCategorize,
		UserID:      userID,
		TriggerKind: driven.JobTriggerAPI,
		ChainID:     uuid.New(),
		Now:         now,
	})
	if err != nil {
		t.Fatal(err)
	}

	service := ExecutionService{
		Store: store,
		Executors: ExecutorRegistry{
			TypeCategorize: &cancellingExecutor{store: store, user: userID, now: now.Add(time.Second)},
		},
		LeaseOwner: "test-worker",
	}
	if err := service.HandleStreamRecord(ctx, job.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := service.HandleStreamRecord(ctx, job.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != driven.JobStatusCancelled {
		t.Fatalf("expected cancelled after chunk boundary, got %s", got.Status)
	}
	if got.Cursor != nil {
		t.Fatalf("expected no persisted cursor after cancellation, got %+v", got.Cursor)
	}
}

type scopeRecordingExecutor struct {
	user, account *uuid.UUID
}

func (e *scopeRecordingExecutor) JobType() string { return TypeCategorize }

func (e *scopeRecordingExecutor) ExecuteChunk(ctx context.Context, run driven.RunContext) (driven.ChunkResult, error) {
	e.user, e.account = driven.UsageScopeFrom(ctx)
	return driven.ChunkResult{Done: true}, nil
}

// Model calls inside a job are metered against the job's user and account,
// so the executor must see them on its context.
func TestRunSynchronousSetsUsageScope(t *testing.T) {
	ctx := context.Background()
	userID, accountID := uuid.New(), uuid.New()
	exec := &scopeRecordingExecutor{}
	service := ExecutionService{Store: memoryjobs.NewStore(), Executors: ExecutorRegistry{TypeCategorize: exec}, LeaseOwner: "test-worker"}
	if _, err := service.RunSynchronous(ctx, driven.CreateJobInput{
		UserID:      userID,
		AccountID:   &accountID,
		TriggerKind: driven.JobTriggerAPI,
		ChainID:     uuid.New(),
	}, exec); err != nil {
		t.Fatal(err)
	}
	if exec.user == nil || *exec.user != userID || exec.account == nil || *exec.account != accountID {
		t.Fatalf("scope = %v/%v, want %s/%s", exec.user, exec.account, userID, accountID)
	}
}
