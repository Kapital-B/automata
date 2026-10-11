package queries

import (
	"context"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
	"time"
)

type Service struct {
	TerminalTTL time.Duration
	Accounts    domainaccounts.AccountRepository
	Assignments domainprojects.AssignmentRepository
	Forwards    domainmessages.ForwardRepository
	JobRuns     domainjobs.JobRunRepository
	JobStore    domainjobs.JobStore
	Messages    domainmessages.MessageRepository
	Schedules   domainjobs.ScheduleRepository
	Summaries   domainmessages.SummaryRepository
	Users       domainauth.UserRepository
}

func (s *Service) ListAccounts(ctx context.Context, userID uuid.UUID) ([]domainaccounts.AccountRow, error) {
	return s.Accounts.ListAccounts(ctx, userID)
}

var _ domainaccounts.AccountsQueries = (*Service)(nil)

func (s *Service) ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]domainmessages.ForwardAllowlistRow, error) {
	return s.Forwards.ListForwardAllowlist(ctx, userID)
}

func (s *Service) DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error {
	return s.Forwards.DeleteForwardRule(ctx, userID, ruleID)
}

func (s *Service) ListForwardActivity(ctx context.Context, userID, ruleID uuid.UUID, limit int) ([]domainmessages.ForwardActivityRow, error) {
	return s.Forwards.ListForwardActivity(ctx, userID, ruleID, limit)
}

var _ domainmessages.ForwardsQueries = (*Service)(nil)

func (s *Service) ListJobRuns(ctx context.Context, userID uuid.UUID, filter domainjobs.JobRunListFilter) ([]domainjobs.JobRunRow, error) {
	return s.JobRuns.ListJobRuns(ctx, userID, filter)
}

func (s *Service) GetJobRun(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*domainjobs.JobRunRow, error) {
	return s.JobRuns.GetJobRun(ctx, userID, id)
}

var _ domainjobs.JobRunsQueries = (*Service)(nil)

func (s *Service) Get(ctx context.Context, userID, jobID uuid.UUID) (*domainjobs.JobRecord, error) {
	row, err := s.JobStore.Get(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	err = s.hydrateJob(ctx, userID, row)
	return row, err
}

func (s *Service) List(ctx context.Context, filter domainjobs.JobListFilter) (*domainjobs.JobListPage, error) {
	page, err := s.JobStore.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for i := range page.Jobs {
		if err = s.hydrateJob(ctx, filter.UserID, &page.Jobs[i]); err != nil {
			return nil, err
		}
	}
	return page, nil
}

var _ domainjobs.JobStoreQueries = (*Service)(nil)

func (s *Service) GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*domainmessages.MessageRow, error) {
	return s.Messages.GetMessage(ctx, userID, id)
}

func (s *Service) ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]domainmessages.CategoryDefinitionRow, error) {
	return s.Messages.ListCategoryDefinitions(ctx, userID)
}

var _ domainmessages.MessagesQueries = (*Service)(nil)

func (s *Service) ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]domainjobs.ScheduleChainRow, error) {
	return s.Schedules.ListSchedulesByUser(ctx, userID)
}

var _ domainjobs.SchedulesQueries = (*Service)(nil)

func (s *Service) GetSummarySettings(ctx context.Context, userID uuid.UUID) (*domainmessages.SummarySettingsRow, error) {
	return s.Summaries.GetSummarySettings(ctx, userID)
}

func (s *Service) MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error {
	return s.Summaries.MarkActionItemDone(ctx, userID, itemID, at)
}

func (s *Service) DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.Summaries.DeleteFYI(ctx, userID, id)
}

func (s *Service) ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]domainmessages.DraftSuggestionRow, error) {
	return s.Summaries.ListDraftSuggestions(ctx, userID, accountID, limit)
}

func (s *Service) ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]domainmessages.SendAttemptRow, error) {
	return s.Summaries.ListSendAttemptsByDraft(ctx, userID, draftID, limit)
}

var _ domainmessages.SummariesQueries = (*Service)(nil)

var _ domainauth.UsersQueries = (*Service)(nil)
