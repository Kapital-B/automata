package asynqadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	QueueSync       = "sync"
	QueueCategorize = "categorize"
	QueueSummarize  = "summarize"

	TypeSyncV1       = "sync:v1"
	TypeCategorizeV1 = "categorize:v1"
	TypeSummarizeV1  = "summarize:v1"
)

type TaskPayload struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         uuid.UUID `json:"run_id"`
	UserID        uuid.UUID `json:"user_id"`
	AccountID     uuid.UUID `json:"account_id"`
	TriggerKind   string    `json:"trigger_kind"`
	Recategorize  bool      `json:"recategorize,omitempty"`
}

type QueueClient struct {
	client *asynq.Client
	prefix string
}

func NewQueueClient(redisAddr string, prefix string) *QueueClient {
	return &QueueClient{
		client: asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr}),
		prefix: prefix,
	}
}

func (q *QueueClient) Close() error {
	if q == nil || q.client == nil {
		return nil
	}
	return q.client.Close()
}

func (q *QueueClient) EnqueueSync(ctx context.Context, payload TaskPayload) error {
	return q.enqueue(ctx, TypeSyncV1, QueueSync, payload, true)
}

func (q *QueueClient) EnqueueCategorize(ctx context.Context, payload TaskPayload) error {
	return q.enqueue(ctx, TypeCategorizeV1, QueueCategorize, payload, false)
}

func (q *QueueClient) EnqueueSummarize(ctx context.Context, payload TaskPayload) error {
	return q.enqueue(ctx, TypeSummarizeV1, QueueSummarize, payload, false)
}

func (q *QueueClient) enqueue(ctx context.Context, taskType string, queue string, payload TaskPayload, uniqueSync bool) error {
	if q == nil || q.client == nil {
		return fmt.Errorf("queue client not configured")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, raw, asynq.Queue(queue))
	opts := []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(0),
	}
	if uniqueSync {
		opts = append(opts, asynq.Unique(5*time.Minute))
	}
	_, err = q.client.EnqueueContext(ctx, task, opts...)
	return err
}

type WorkerDeps struct {
	Log             *slog.Logger
	SyncSvc         *appmessages.SyncService
	CategorizeSvc   *appmessages.CategorizeService
	SummarizeSvc    *appmessages.SummarizeService
	JobRuns         driven.JobRunRepository
	GlobalSemaphore chan struct{}
}

func NewWorkerMux(deps WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeSyncV1, func(ctx context.Context, task *asynq.Task) error {
		return handleSync(ctx, task, deps)
	})
	mux.HandleFunc(TypeCategorizeV1, func(ctx context.Context, task *asynq.Task) error {
		return handleCategorize(ctx, task, deps)
	})
	mux.HandleFunc(TypeSummarizeV1, func(ctx context.Context, task *asynq.Task) error {
		return handleSummarize(ctx, task, deps)
	})
	return mux
}

func handleSync(ctx context.Context, task *asynq.Task, deps WorkerDeps) error {
	var p TaskPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return err
	}
	if err := acquire(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer release(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.SyncSvc.SyncInboxWithOptions(ctx, p.UserID, p.AccountID, appmessages.SyncOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID)
	return nil
}

func handleCategorize(ctx context.Context, task *asynq.Task, deps WorkerDeps) error {
	if deps.CategorizeSvc == nil {
		return fmt.Errorf("categorize service not configured")
	}
	var p TaskPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return err
	}
	if err := acquire(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer release(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.CategorizeSvc.CategorizeAccount(ctx, p.UserID, p.AccountID, appmessages.CategorizeOptions{
		Recategorize: p.Recategorize,
		RunID:        &p.RunID,
		Trigger:      p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID)
	return nil
}

func handleSummarize(ctx context.Context, task *asynq.Task, deps WorkerDeps) error {
	if deps.SummarizeSvc == nil {
		return fmt.Errorf("summarize service not configured")
	}
	var p TaskPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return err
	}
	if err := acquire(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer release(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.SummarizeSvc.SummarizeAccount(ctx, p.UserID, p.AccountID, appmessages.SummarizeOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID)
	return nil
}

func acquire(ch chan struct{}) error {
	if ch == nil {
		return nil
	}
	ch <- struct{}{}
	return nil
}

func release(ch chan struct{}) {
	if ch == nil {
		return
	}
	<-ch
}
