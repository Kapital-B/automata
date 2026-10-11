package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainconnectors "github.com/Kapital-B/automata/svc/internal/domain/connectors"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"github.com/google/uuid"
	"time"
)

type Service struct {
	Inline     bool
	Accounts   domainaccounts.AccountRepository
	Durable    domainjobs.JobEnqueuer
	Legacy     domainjobs.LegacyQueue
	Runs       domainjobs.JobRunRepository
	Sync       domainmessages.MessageSyncUseCases
	Categorize domainmessages.MessageCategorizeUseCases
	Summarize  domainmessages.MessageSummarizeUseCases
	Draft      domainmessages.MessageAutoDraftUseCases
	Forward    domainmessages.MessageForwardRulesUseCases
	Connectors domainconnectors.ConnectorUseCases
	Projects   domainprojects.ProjectUseCases
}

func problem(kind, message string, cause error) error {
	return &domainshared.OperationError{Kind: kind, Message: message, Cause: cause}
}
func (s *Service) Submit(ctx context.Context, in domainjobs.CreateJobInput) (*domainjobs.JobRecord, error) {
	if s.Durable == nil {
		return nil, problem("unavailable", "job queue not configured", nil)
	}
	return s.Durable.Enqueue(ctx, in)
}
func (s *Service) SubmitAccount(ctx context.Context, user, account uuid.UUID, kind string, payload domainjobs.JobPayload) (*domainjobs.Submission, error) {
	if s.Accounts != nil {
		row, _, err := s.Accounts.GetAccount(ctx, user, account)
		if err != nil {
			return nil, problem("storage", "internal", err)
		}
		if row == nil {
			return nil, problem("missing", "not found", nil)
		}
	}
	inline := s.Inline || kind == "forward_rules" && s.Durable == nil && (s.Legacy == nil || s.Runs == nil)
	if inline {
		return s.inlineAccount(ctx, user, account, kind, payload)
	}
	if s.Durable != nil {
		job, err := s.Durable.Enqueue(ctx, domainjobs.CreateJobInput{JobType: kind, UserID: user, AccountID: &account, TriggerKind: domainjobs.JobTriggerAPI, Payload: payload, Now: time.Now().UTC()})
		if err != nil {
			return nil, problem("invalid", err.Error(), err)
		}
		return &domainjobs.Submission{Mode: "durable", RunID: job.ID, ChainID: job.ChainID}, nil
	}
	if s.Legacy == nil || s.Runs == nil {
		message := kind + " service not configured"
		switch kind {
		case "sync":
			message = "sync queue not configured"
		case "draft_suggest":
			message = "auto-draft service not configured"
		}
		return nil, problem("unavailable", message, nil)
	}
	id := uuid.New()
	meta, failed := `{"queued":true}`, `{"queued":false}`
	if kind == "categorize" {
		meta = fmt.Sprintf(`{"queued":true,"recategorize":%t}`, payload.Recategorize)
		failed = fmt.Sprintf(`{"queued":false,"recategorize":%t}`, payload.Recategorize)
	}
	if kind == "draft_suggest" {
		meta = `{"queued":true,"only_unseen":false}`
		failed = `{"queued":false,"only_unseen":false}`
	}
	err := s.Runs.InsertJobRun(ctx, id, account, kind, "api", "pending", time.Now().UTC(), time.Time{}, nil, meta)
	if err != nil && kind != "forward_rules" {
		return nil, problem("storage", "internal", err)
	}
	request := domainjobs.LegacyJobRequest{RunID: id, UserID: user, AccountID: account, TriggerKind: "api", MessageID: payload.MessageID, Recategorize: payload.Recategorize, Force: payload.Force}
	if kind == "sync" {
		request.Force = false
	}
	switch kind {
	case "sync":
		err = s.Legacy.EnqueueSync(ctx, request)
	case "categorize":
		err = s.Legacy.EnqueueCategorize(ctx, request)
	case "summarize":
		err = s.Legacy.EnqueueSummarize(ctx, request)
	case "draft_suggest":
		err = s.Legacy.EnqueueDraftSuggest(ctx, request)
	case "forward_rules":
		err = s.Legacy.EnqueueForwardRules(ctx, request)
	default:
		err = fmt.Errorf("unknown job type")
	}
	if err != nil {
		message := err.Error()
		now := time.Now().UTC()
		_ = s.Runs.UpdateJobRunStatus(ctx, id, "failed", &now, &message, failed)
		return nil, problem("invalid", message, err)
	}
	return &domainjobs.Submission{Mode: "legacy", RunID: id}, nil
}
func (s *Service) inlineAccount(ctx context.Context, user, account uuid.UUID, kind string, payload domainjobs.JobPayload) (*domainjobs.Submission, error) {
	out := &domainjobs.Submission{Mode: "inline"}
	var err error
	switch kind {
	case "sync":
		r, e := s.Sync.SyncInboxWithOptions(ctx, user, account, domainmessages.MessageSyncOptions{Force: payload.Force})
		err = e
		if r != nil {
			out.RunID = r.JobRunID
			out.MessagesUpserted = r.MessagesUpserted
		}
	case "categorize":
		r, e := s.Categorize.CategorizeAccount(ctx, user, account, domainmessages.MessageCategorizeOptions{Recategorize: payload.Recategorize})
		err = e
		if r != nil {
			out.RunID = r.JobRunID
			out.MessagesCategorized = r.MessagesCategorized
		}
	case "summarize":
		r, e := s.Summarize.SummarizeAccount(ctx, user, account, domainmessages.MessageSummarizeOptions{})
		err = e
		if r != nil {
			out.RunID = r.JobRunID
		}
	case "draft_suggest":
		r, e := s.Draft.GenerateForAccount(ctx, user, account, domainmessages.MessageAutoDraftOptions{OnlyUnseen: false, MessageID: payload.MessageID, Force: payload.MessageID != nil})
		err = e
		if r != nil {
			out.RunID = r.JobRunID
			out.DraftsGenerated = r.DraftsGenerated
			out.ActionItemsSeen = r.ActionItemsSeen
		}
	case "forward_rules":
		if s.Forward == nil {
			return nil, problem("unavailable", "forward-rules service not configured", nil)
		}
		out.RunID, err = s.Forward.RunAccount(ctx, user, account, domainmessages.MessageForwardRulesOptions{Trigger: "api"})
	default:
		err = fmt.Errorf("unknown job type")
	}
	if err != nil {
		return nil, problem("invalid", err.Error(), err)
	}
	return out, nil
}
func (s *Service) SubmitConnector(ctx context.Context, user, account uuid.UUID) (*domainjobs.Submission, error) {
	if s.Connectors == nil {
		return nil, problem("unavailable", "connector sync not configured", nil)
	}
	if s.Inline {
		r, err := s.Connectors.Sync(ctx, user, account)
		if err != nil {
			if errors.Is(err, domainconnectors.ConnectorErrNotFound) {
				return nil, problem("missing", "not found", err)
			}
			return nil, problem("invalid", err.Error(), err)
		}
		return &domainjobs.Submission{Mode: "inline", RunID: r.JobRunID, MessagesUpserted: r.MessagesUpserted}, nil
	}
	rows, err := s.Connectors.List(ctx, user)
	if err != nil {
		return nil, problem("storage", "internal", err)
	}
	found := false
	for _, row := range rows {
		if row.ID == account {
			found = true
			break
		}
	}
	if !found {
		return nil, problem("missing", "not found", nil)
	}
	if s.Durable != nil {
		job, err := s.Durable.Enqueue(ctx, domainjobs.CreateJobInput{JobType: "sync_slack", UserID: user, TriggerKind: domainjobs.JobTriggerAPI, Payload: domainjobs.JobPayload{ConnectorAccountID: &account}, Now: time.Now().UTC()})
		if err != nil {
			return nil, problem("invalid", err.Error(), err)
		}
		return &domainjobs.Submission{Mode: "durable", RunID: job.ID}, nil
	}
	if s.Legacy == nil || s.Runs == nil {
		return nil, problem("unavailable", "connector sync not configured", nil)
	}
	id := uuid.New()
	meta, _ := json.Marshal(map[string]any{"queued": true, "connector_account_id": account.String()})
	if err = s.Runs.InsertJobRun(ctx, id, uuid.Nil, "sync_slack", "api", "pending", time.Now().UTC(), time.Time{}, nil, string(meta)); err != nil {
		return nil, problem("storage", "internal", err)
	}
	err = s.Legacy.EnqueueSyncSlack(ctx, domainjobs.LegacyJobRequest{RunID: id, UserID: user, ConnectorAccountID: account, TriggerKind: "api"})
	if err != nil {
		message := err.Error()
		now := time.Now().UTC()
		_ = s.Runs.UpdateJobRunStatus(ctx, id, "failed", &now, &message, string(meta))
		return nil, problem("invalid", message, err)
	}
	return &domainjobs.Submission{Mode: "legacy", RunID: id}, nil
}
func (s *Service) SubmitExtraction(ctx context.Context, user, project uuid.UUID) (*domainjobs.Submission, error) {
	if s.Durable == nil || s.Projects == nil {
		return nil, problem("unavailable", "extraction not configured", nil)
	}
	detail, err := s.Projects.Get(ctx, user, project)
	if err != nil {
		return nil, problem("invalid", err.Error(), err)
	}
	if detail == nil {
		return nil, problem("missing", "not found", nil)
	}
	job, err := s.Durable.Enqueue(ctx, domainjobs.CreateJobInput{JobType: domainjobs.TypeInterpretProject, UserID: user, TriggerKind: domainjobs.JobTriggerAPI, RemainingJobs: []string{domainjobs.TypeReconcileProject}, Payload: domainjobs.JobPayload{ProjectID: &project}, Now: time.Now().UTC()})
	if errors.Is(err, domainjobs.ErrJobLockHeld) || errors.Is(err, domainjobs.ErrJobConflict) {
		return &domainjobs.Submission{AlreadyRunning: true}, nil
	}
	if err != nil {
		return nil, problem("invalid", err.Error(), err)
	}
	out := &domainjobs.Submission{Mode: "durable"}
	if job != nil {
		out.RunID = job.ID
		out.ChainID = job.ChainID
	}
	return out, nil
}

var _ domainmessages.AccountJobSubmission = (*Service)(nil)
var _ domainconnectors.SyncSubmission = (*Service)(nil)
var _ domainprojects.ExtractionSubmission = (*Service)(nil)
var _ domainjobs.RequestSubmission = (*Service)(nil)

func (s *Service) RescanAssignments(ctx context.Context, user uuid.UUID, account *uuid.UUID) ([]uuid.UUID, error) {
	if s.Durable == nil {
		return nil, problem("unavailable", "assignment not configured", nil)
	}
	ids := []uuid.UUID{}
	if account != nil {
		if s.Accounts != nil {
			row, _, err := s.Accounts.GetAccount(ctx, user, *account)
			if err != nil {
				return nil, problem("storage", "internal", err)
			}
			if row == nil {
				return nil, problem("missing", "not found", nil)
			}
		}
		ids = append(ids, *account)
	} else {
		if s.Accounts == nil {
			return nil, problem("unavailable", "accounts not configured", nil)
		}
		rows, err := s.Accounts.ListAccounts(ctx, user)
		if err != nil {
			return nil, problem("storage", "internal", err)
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil, problem("invalid", "no accounts to rescan", nil)
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		job, err := s.Durable.Enqueue(ctx, domainjobs.CreateJobInput{JobType: domainjobs.TypeAssignProjects, UserID: user, AccountID: &id, TriggerKind: domainjobs.JobTriggerAPI, Now: time.Now().UTC()})
		if err != nil {
			return nil, problem("storage", "internal", err)
		}
		out = append(out, job.ID)
	}
	return out, nil
}
