package jobs

import (
	"context"
	"fmt"
	domainconnectors "github.com/Kapital-B/automata/svc/internal/domain/connectors"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"github.com/google/uuid"
	"log/slog"
	"strings"
	"time"
)

type LegacyWorker struct {
	Log             *slog.Logger
	SyncSvc         domainmessages.MessageSyncUseCases
	CategorizeSvc   domainmessages.MessageCategorizeUseCases
	SummarizeSvc    domainmessages.MessageSummarizeUseCases
	AutoDraftSvc    domainmessages.MessageAutoDraftUseCases
	ForwardRulesSvc domainmessages.MessageForwardRulesUseCases
	ConnectorSync   domainconnectors.ConnectorUseCases
	JobRuns         domainjobs.JobRunRepository
	Execution       domainjobs.JobExecutionPort
	GlobalSemaphore chan struct{}
	Queue           domainjobs.LegacyQueue
}

func (deps *LegacyWorker) handleSync(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.SyncSvc.SyncInboxWithOptions(ctx, p.UserID, p.AccountID, domainmessages.MessageSyncOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, p.RunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{}`)
		return err
	}
	deps.Log.Info("job success", "job_type", "sync", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func (deps *LegacyWorker) handleCategorize(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.CategorizeSvc == nil {
		return fmt.Errorf("categorize service not configured")
	}
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.CategorizeSvc.CategorizeAccount(ctx, p.UserID, p.AccountID, domainmessages.MessageCategorizeOptions{
		Recategorize: p.Recategorize,
		RunID:        &p.RunID,
		Trigger:      p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, p.RunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{}`)
		return err
	}
	deps.Log.Info("job success", "job_type", "categorize", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func (deps *LegacyWorker) handleSummarize(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.SummarizeSvc == nil {
		return fmt.Errorf("summarize service not configured")
	}
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.SummarizeSvc.SummarizeAccount(ctx, p.UserID, p.AccountID, domainmessages.MessageSummarizeOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
		Since:   p.ChainStartedAt,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, p.RunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{}`)
		return err
	}
	deps.Log.Info("job success", "job_type", "summarize", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func (deps *LegacyWorker) handleDraftSuggest(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.AutoDraftSvc == nil {
		return fmt.Errorf("auto-draft service not configured")
	}
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)

	deps.Log.Info("job start", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	onlyUnseen := true
	if strings.EqualFold(strings.TrimSpace(p.TriggerKind), "api") {
		// Manual API-triggered drafts should always attempt generation.
		onlyUnseen = false
	}
	_, err := deps.AutoDraftSvc.GenerateForAccount(ctx, p.UserID, p.AccountID, domainmessages.MessageAutoDraftOptions{
		RunID:      &p.RunID,
		Trigger:    p.TriggerKind,
		OnlyUnseen: onlyUnseen,
		MessageID:  p.MessageID,
		Force:      p.Force,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, p.RunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{}`)
		return err
	}
	deps.Log.Info("job success", "job_type", "draft_suggest", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func (deps *LegacyWorker) handleForwardRules(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.ForwardRulesSvc == nil {
		return fmt.Errorf("forward-rules service not configured")
	}
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)
	deps.Log.Info("job start", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID, "trigger_kind", p.TriggerKind)
	_, err := deps.ForwardRulesSvc.RunAccount(ctx, p.UserID, p.AccountID, domainmessages.MessageForwardRulesOptions{
		RunID:   &p.RunID,
		Trigger: p.TriggerKind,
		Since:   p.ChainStartedAt,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID, "err", err)
		msg := err.Error()
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, p.RunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{}`)
		return err
	}
	deps.Log.Info("job success", "job_type", "forward_rules", "run_id", p.RunID, "account_id", p.AccountID)
	maybeEnqueueNext(ctx, deps, p)
	return nil
}

func (deps *LegacyWorker) handleSyncSlack(ctx context.Context, p domainjobs.LegacyJobRequest) error {
	if deps.ConnectorSync == nil {
		return fmt.Errorf("connector sync service not configured")
	}
	if deps.Execution != nil {
		return deps.Execution.HandleStreamRecord(ctx, p.RunID, time.Now().UTC())
	}
	if p.ConnectorAccountID == uuid.Nil {
		return fmt.Errorf("connector_account_id is required")
	}
	if err := acquireLegacy(deps.GlobalSemaphore); err != nil {
		return err
	}
	defer releaseLegacy(deps.GlobalSemaphore)
	deps.Log.Info("job start", "job_type", "sync_slack", "run_id", p.RunID, "connector_account_id", p.ConnectorAccountID)
	_, err := deps.ConnectorSync.SyncConnectorWithOptions(ctx, p.UserID, p.ConnectorAccountID, domainconnectors.ConnectorSyncOptions{
		RunID: &p.RunID, Trigger: p.TriggerKind,
	})
	if err != nil {
		deps.Log.Error("job failed", "job_type", "sync_slack", "run_id", p.RunID, "connector_account_id", p.ConnectorAccountID, "err", err)
		return err
	}
	deps.Log.Info("job success", "job_type", "sync_slack", "run_id", p.RunID, "connector_account_id", p.ConnectorAccountID)
	return nil
}

func maybeEnqueueNext(ctx context.Context, deps *LegacyWorker, p domainjobs.LegacyJobRequest) {
	if len(p.RemainingJobs) == 0 || deps.Queue == nil || deps.JobRuns == nil {
		return
	}
	next := p.RemainingJobs[0]
	rest := append([]string(nil), p.RemainingJobs[1:]...)
	nextRunID := uuid.New()
	meta := `{"queued":true,"schedule_chain":true}`
	_ = deps.JobRuns.InsertJobRun(ctx, nextRunID, p.AccountID, next, "schedule", "pending", time.Now().UTC(), time.Time{}, nil, meta)
	nextPayload := domainjobs.LegacyJobRequest{
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
		_ = deps.JobRuns.UpdateJobRunStatus(ctx, nextRunID, "failed", legacyTimePtr(time.Now().UTC()), &msg, `{"queued":false}`)
		deps.Log.Error("enqueue next chain job", "job_type", next, "err", err)
	}
}

func EnqueueByJobType(ctx context.Context, queue domainjobs.LegacyQueue, jobType string, payload domainjobs.LegacyJobRequest) error {
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

func legacyTimePtr(t time.Time) *time.Time {
	return &t
}

func acquireLegacy(ch chan struct{}) error {
	if ch == nil {
		return nil
	}
	ch <- struct{}{}
	return nil
}

func releaseLegacy(ch chan struct{}) {
	if ch == nil {
		return
	}
	<-ch
}
func (deps *LegacyWorker) HandleLegacy(ctx context.Context, kind string, in domainjobs.LegacyJobRequest) error {
	switch kind {
	case "sync":
		return deps.handleSync(ctx, in)
	case "categorize":
		return deps.handleCategorize(ctx, in)
	case "summarize":
		return deps.handleSummarize(ctx, in)
	case "draft_suggest":
		return deps.handleDraftSuggest(ctx, in)
	case "forward_rules":
		return deps.handleForwardRules(ctx, in)
	case "sync_slack":
		return deps.handleSyncSlack(ctx, in)
	default:
		return fmt.Errorf("unsupported job type: %s", kind)
	}
}

var _ domainjobs.LegacyExecutionPort = (*LegacyWorker)(nil)
