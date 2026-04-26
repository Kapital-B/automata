package asynqadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	QueueDraft      = "draft_suggest"
	QueueForward    = "forward_rules"

	TypeSyncV1       = "sync:v1"
	TypeCategorizeV1 = "categorize:v1"
	TypeSummarizeV1  = "summarize:v1"
	TypeDraftV1      = "draft_suggest:v1"
	TypeForwardV1    = "forward_rules:v1"
)

type TaskPayload struct {
	SchemaVersion  int        `json:"schema_version"`
	RunID          uuid.UUID  `json:"run_id"`
	UserID         uuid.UUID  `json:"user_id"`
	AccountID      uuid.UUID  `json:"account_id"`
	TriggerKind    string     `json:"trigger_kind"`
	Recategorize   bool       `json:"recategorize,omitempty"`
	RemainingJobs  []string   `json:"remaining_jobs,omitempty"`
	ScheduleID     *uuid.UUID `json:"schedule_id,omitempty"`
	ChainStartedAt *time.Time `json:"chain_started_at,omitempty"`
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

func (q *QueueClient) EnqueueDraftSuggest(ctx context.Context, payload TaskPayload) error {
	return q.enqueue(ctx, TypeDraftV1, QueueDraft, payload, false)
}

func (q *QueueClient) EnqueueForwardRules(ctx context.Context, payload TaskPayload) error {
	return q.enqueue(ctx, TypeForwardV1, QueueForward, payload, false)
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
	AutoDraftSvc    *appmessages.AutoDraftService
	ForwardRulesSvc *appmessages.ForwardRulesService
	JobRuns         driven.JobRunRepository
	GlobalSemaphore chan struct{}
	Queue           *QueueClient
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
	mux.HandleFunc(TypeDraftV1, func(ctx context.Context, task *asynq.Task) error {
		return handleDraftSuggest(ctx, task, deps)
	})
	mux.HandleFunc(TypeForwardV1, func(ctx context.Context, task *asynq.Task) error {
		return handleForwardRules(ctx, task, deps)
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
	maybeEnqueueNext(ctx, deps, p)
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
	maybeEnqueueNext(ctx, deps, p)
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
		Since:   p.ChainStartedAt,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func handleDraftSuggest(ctx context.Context, task *asynq.Task, deps WorkerDeps) error {
	if deps.AutoDraftSvc == nil {
		return fmt.Errorf("auto-draft service not configured")
	}
	var p TaskPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return err
	}
	if err := acquire(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer release(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.AutoDraftSvc.GenerateForAccount(ctx, p.UserID, p.AccountID, appmessages.AutoDraftOptions{
		RunID:      &p.RunID,
		Trigger:    p.TriggerKind,
		OnlyUnseen: true,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func handleForwardRules(ctx context.Context, task *asynq.Task, deps WorkerDeps) error {
	if deps.ForwardRulesSvc == nil {
		return fmt.Errorf("forward-rules service not configured")
	}
	var p TaskPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		return err
	}
	if err := acquire(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer release(deps.GlobalSemaphore)
	deps.Log.Info("job start", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.ForwardRulesSvc.RunAccount(ctx, p.UserID, p.AccountID, appmessages.ForwardRulesOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
		Since:   p.ChainStartedAt,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func maybeEnqueueNext(ctx context.Context, deps WorkerDeps, p TaskPayload) {
	if len(p.RemainingJobs) == 0 || deps.Queue == nil || deps.JobRuns == nil {
		return
	}
	next := p.RemainingJobs[0]
	rest := append([]string(nil), p.RemainingJobs[1:]...)
	nextRunID := uuid.New()
	meta := `{"queued":true,"schedule_chain":true}`
	_ = deps.JobRuns.InsertJobRun(ctx, nextRunID, p.AccountID, next, "schedule", "pending", time.Now().UTC(), time.Time{}, nil, meta)
	nextPayload := TaskPayload{
		SchemaVersion:  1,
		RunID:          nextRunID,
		UserID:         p.UserID,
		AccountID:      p.AccountID,
		TriggerKind:    "schedule",
		RemainingJobs:  rest,
		ScheduleID:     p.ScheduleID,
		ChainStartedAt: p.ChainStartedAt,
	}
	if err := EnqueueByJobType(ctx, deps.Queue, next, nextPayload); err != nil {
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, nextRunID, "failed", timePtrAsynq(time.Now().UTC()), &msg, `{"queued":false}`)
		deps.Log.Error("enqueue next chain job", "job_type", next, "err", err)
	}
}

func EnqueueByJobType(ctx context.Context, queue *QueueClient, jobType string, payload TaskPayload) error {
	switch strings.TrimSpace(strings.ToLower(jobType)) {
	case "sync":
		return queue.EnqueueSync(ctx, payload)
	case "categorize":
		return queue.EnqueueCategorize(ctx, payload)
	case "summarize":
		return queue.EnqueueSummarize(ctx, payload)
	case "auto-draft", "draft_suggest":
		return queue.EnqueueDraftSuggest(ctx, payload)
	case "forward", "forward_rules":
		return queue.EnqueueForwardRules(ctx, payload)
	default:
		return fmt.Errorf("unsupported job type: %s", jobType)
	}
}

func timePtrAsynq(t time.Time) *time.Time {
	return &t
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
