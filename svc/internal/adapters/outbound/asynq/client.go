package asynqadapter

import (
	"context"
	"encoding/json"
	"fmt"
	asynqtask "github.com/Kapital-B/automata/svc/internal/adapters/shared/asynqtask"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/hibiken/asynq"
	"time"
)

const QueueSync = asynqtask.QueueSync
const QueueCategorize = asynqtask.QueueCategorize
const QueueSummarize = asynqtask.QueueSummarize
const QueueDraft = asynqtask.QueueDraft
const QueueForward = asynqtask.QueueForward
const TypeSyncV1 = asynqtask.TypeSyncV1
const TypeCategorizeV1 = asynqtask.TypeCategorizeV1
const TypeSummarizeV1 = asynqtask.TypeSummarizeV1
const TypeDraftV1 = asynqtask.TypeDraftV1
const TypeForwardV1 = asynqtask.TypeForwardV1
const TypeSyncSlackV1 = asynqtask.TypeSyncSlackV1

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

func (q *QueueClient) EnqueueSync(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeSyncV1, QueueSync, payload, true)
}

func (q *QueueClient) EnqueueCategorize(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeCategorizeV1, QueueCategorize, payload, false)
}

func (q *QueueClient) EnqueueSummarize(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeSummarizeV1, QueueSummarize, payload, false)
}

func (q *QueueClient) EnqueueDraftSuggest(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeDraftV1, QueueDraft, payload, false)
}

func (q *QueueClient) EnqueueForwardRules(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeForwardV1, QueueForward, payload, false)
}

func (q *QueueClient) EnqueueSyncSlack(ctx context.Context, payload domainjobs.LegacyJobRequest) error {
	return q.enqueue(ctx, TypeSyncSlackV1, QueueSync, payload, true)
}

func (q *QueueClient) enqueue(ctx context.Context, taskType string, queue string, payload domainjobs.LegacyJobRequest, uniqueSync bool) error {
	if q == nil || q.client == nil {
		return fmt.Errorf("queue client not configured")
	}
	raw, err := json.Marshal(taskPayload(payload))
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
func taskPayload(in domainjobs.LegacyJobRequest) asynqtask.Payload {
	return asynqtask.Payload{SchemaVersion: 1, RunID: in.RunID, UserID: in.UserID, AccountID: in.AccountID, ConnectorAccountID: in.ConnectorAccountID, TriggerKind: in.TriggerKind, MessageID: in.MessageID, Force: in.Force, Recategorize: in.Recategorize, RemainingJobs: in.RemainingJobs, ScheduleID: in.ScheduleID, ChainStartedAt: in.ChainStartedAt}
}

var _ domainjobs.LegacyQueue = (*QueueClient)(nil)
