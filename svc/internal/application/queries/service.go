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

func (s *Service) GetAccount(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*domainaccounts.AccountRow, []byte, error) {
	return s.Accounts.GetAccount(ctx, userID, id)
}

func (s *Service) ListAccounts(ctx context.Context, userID uuid.UUID) ([]domainaccounts.AccountRow, error) {
	return s.Accounts.ListAccounts(ctx, userID)
}

var _ domainaccounts.AccountsQueries = (*Service)(nil)

func (s *Service) EffectiveProjectIDsForMessages(ctx context.Context, userID uuid.UUID, messages []domainmessages.MessageRow) (map[uuid.UUID]*uuid.UUID, error) {
	return s.Assignments.EffectiveProjectIDsForMessages(ctx, userID, messages)
}

var _ domainprojects.AssignmentsQueries = (*Service)(nil)

func (s *Service) ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]domainmessages.ForwardAllowlistRow, error) {
	return s.Forwards.ListForwardAllowlist(ctx, userID)
}

func (s *Service) ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error {
	return s.Forwards.ReplaceForwardAllowlist(ctx, userID, emails)
}

func (s *Service) ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]domainmessages.ForwardRuleRow, error) {
	return s.Forwards.ListForwardRules(ctx, userID, accountID)
}

func (s *Service) CreateForwardRule(ctx context.Context, row domainmessages.ForwardRuleRow) error {
	return s.Forwards.CreateForwardRule(ctx, row)
}

func (s *Service) UpdateForwardRule(ctx context.Context, row domainmessages.ForwardRuleRow) error {
	return s.Forwards.UpdateForwardRule(ctx, row)
}

func (s *Service) DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error {
	return s.Forwards.DeleteForwardRule(ctx, userID, ruleID)
}

func (s *Service) ForwardRuleStats(ctx context.Context, userID, accountID uuid.UUID) ([]domainmessages.ForwardRuleStats, error) {
	return s.Forwards.ForwardRuleStats(ctx, userID, accountID)
}

func (s *Service) ListForwardActivity(ctx context.Context, userID, ruleID uuid.UUID, limit int) ([]domainmessages.ForwardActivityRow, error) {
	return s.Forwards.ListForwardActivity(ctx, userID, ruleID, limit)
}

var _ domainmessages.ForwardsQueries = (*Service)(nil)

func (s *Service) InsertJobRun(ctx context.Context, id uuid.UUID, accountID uuid.UUID, jobType string, trigger string, status string, startedAt, finishedAt time.Time, errMsg *string, metaJSON string) error {
	return s.JobRuns.InsertJobRun(ctx, id, accountID, jobType, trigger, status, startedAt, finishedAt, errMsg, metaJSON)
}

func (s *Service) UpdateJobRunStatus(ctx context.Context, id uuid.UUID, status string, finishedAt *time.Time, errMsg *string, metaJSON string) error {
	return s.JobRuns.UpdateJobRunStatus(ctx, id, status, finishedAt, errMsg, metaJSON)
}

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

func (s *Service) RequestCancel(ctx context.Context, userID, jobID uuid.UUID, now time.Time, terminalTTL time.Duration) (*domainjobs.JobRecord, error) {
	return s.JobStore.RequestCancel(ctx, userID, jobID, now, terminalTTL)
}

var _ domainjobs.JobStoreQueries = (*Service)(nil)

func (s *Service) ListMessages(ctx context.Context, userID uuid.UUID, filter domainmessages.MessageListFilter) ([]domainmessages.MessageRow, error) {
	return s.Messages.ListMessages(ctx, userID, filter)
}

func (s *Service) GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*domainmessages.MessageRow, error) {
	return s.Messages.GetMessage(ctx, userID, id)
}

func (s *Service) ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]domainmessages.CategoryDefinitionRow, error) {
	return s.Messages.ListCategoryDefinitions(ctx, userID)
}

func (s *Service) GetCategoryDefinitionByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*domainmessages.CategoryDefinitionRow, error) {
	return s.Messages.GetCategoryDefinitionByID(ctx, userID, id)
}

func (s *Service) CreateCategoryDefinition(ctx context.Context, row domainmessages.CategoryDefinitionRow) error {
	return s.Messages.CreateCategoryDefinition(ctx, row)
}

func (s *Service) UpdateCategoryDefinition(ctx context.Context, row domainmessages.CategoryDefinitionRow) error {
	return s.Messages.UpdateCategoryDefinition(ctx, row)
}

func (s *Service) DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.Messages.DeleteCategoryDefinition(ctx, userID, id)
}

func (s *Service) ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error) {
	return s.Messages.ReassignMessageCategories(ctx, userID, fromCategoryID, toCategoryID)
}

func (s *Service) CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error) {
	return s.Messages.CountMessageCategoriesByCategory(ctx, userID, categoryID)
}

var _ domainmessages.MessagesQueries = (*Service)(nil)

func (s *Service) ListSchedulesByUser(ctx context.Context, userID uuid.UUID) ([]domainjobs.ScheduleChainRow, error) {
	return s.Schedules.ListSchedulesByUser(ctx, userID)
}

func (s *Service) ReplaceSchedulesByUser(ctx context.Context, userID uuid.UUID, rows []domainjobs.ScheduleChainRow) error {
	return s.Schedules.ReplaceSchedulesByUser(ctx, userID, rows)
}

var _ domainjobs.SchedulesQueries = (*Service)(nil)

func (s *Service) GetSummarySettings(ctx context.Context, userID uuid.UUID) (*domainmessages.SummarySettingsRow, error) {
	return s.Summaries.GetSummarySettings(ctx, userID)
}

func (s *Service) UpsertSummarySettings(ctx context.Context, row domainmessages.SummarySettingsRow) error {
	return s.Summaries.UpsertSummarySettings(ctx, row)
}

func (s *Service) ListSummarySnapshots(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]domainmessages.SummarySnapshotRow, error) {
	return s.Summaries.ListSummarySnapshots(ctx, userID, accountID, limit)
}

func (s *Service) ListOpenActionItems(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]domainmessages.ActionItemRow, error) {
	return s.Summaries.ListOpenActionItems(ctx, userID, accountID)
}

func (s *Service) MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error {
	return s.Summaries.MarkActionItemDone(ctx, userID, itemID, at)
}

func (s *Service) ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]domainmessages.FYIRow, error) {
	return s.Summaries.ListOpenFYI(ctx, userID, accountID, limit)
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

func (s *Service) GetUserByID(ctx context.Context, id uuid.UUID) (email string, err error) {
	return s.Users.GetUserByID(ctx, id)
}

func (s *Service) GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

var _ domainauth.UsersQueries = (*Service)(nil)
