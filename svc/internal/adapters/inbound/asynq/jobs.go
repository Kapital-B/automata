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

type TaskPayload = asynqtask.Payload
type WorkerDeps struct {
	Execution domainjobs.JobExecutionPort
	Legacy    domainjobs.LegacyExecutionPort
}

func NewWorkerMux(deps WorkerDeps) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	for _, spec := range []struct{ task, kind string }{{TypeSyncV1, "sync"}, {TypeCategorizeV1, "categorize"}, {TypeSummarizeV1, "summarize"}, {TypeDraftV1, "draft_suggest"}, {TypeForwardV1, "forward_rules"}, {TypeSyncSlackV1, "sync_slack"}} {
		mux.HandleFunc(spec.task, func(ctx context.Context, task *asynq.Task) error {
			var payload TaskPayload
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return err
			}
			if deps.Execution != nil {
				return deps.Execution.HandleStreamRecord(ctx, payload.RunID, time.Now().UTC())
			}
			if deps.Legacy == nil {
				return fmt.Errorf("legacy execution not configured")
			}
			return deps.Legacy.HandleLegacy(ctx, spec.kind, legacyRequest(payload))
		})
	}
	return mux
}
func legacyRequest(in TaskPayload) domainjobs.LegacyJobRequest {
	return domainjobs.LegacyJobRequest{RunID: in.RunID, UserID: in.UserID, AccountID: in.AccountID, ConnectorAccountID: in.ConnectorAccountID, TriggerKind: in.TriggerKind, MessageID: in.MessageID, Force: in.Force, Recategorize: in.Recategorize, RemainingJobs: in.RemainingJobs, ScheduleID: in.ScheduleID, ChainStartedAt: in.ChainStartedAt}
}
