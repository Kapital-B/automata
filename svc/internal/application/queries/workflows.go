package queries

import (
	"context"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"

	"fmt"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"github.com/google/uuid"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func failure(kind, message string, cause error) error {
	return &domainshared.OperationError{Kind: kind, Message: message, Cause: cause}
}
func (s *Service) CreateCategory(ctx context.Context, user uuid.UUID, in domainmessages.CategoryInput) (*domainmessages.CategoryDefinitionRow, error) {
	in, err := domainmessages.NormalizeCategoryInput(in)
	if err != nil {
		return nil, failure("invalid", err.Error(), err)
	}
	now := time.Now().UTC()
	row := domainmessages.CategoryDefinitionRow{ID: uuid.New(), UserID: user, Slug: in.Slug, DisplayName: in.DisplayName, Definition: in.Definition, SortOrder: in.SortOrder, CreatedAt: now, UpdatedAt: now}
	if err = s.Messages.CreateCategoryDefinition(ctx, row); err != nil {
		return nil, failure("invalid", "create failed", err)
	}
	return &row, nil
}
func (s *Service) UpdateCategory(ctx context.Context, user, id uuid.UUID, in domainmessages.CategoryInput) (*domainmessages.CategoryDefinitionRow, error) {
	in, err := domainmessages.NormalizeCategoryInput(in)
	if err != nil {
		return nil, failure("invalid", err.Error(), err)
	}
	row, err := s.Messages.GetCategoryDefinitionByID(ctx, user, id)
	if err != nil {
		return nil, failure("storage", "internal", err)
	}
	if row == nil {
		return nil, failure("missing", "not found", nil)
	}
	row.Slug = in.Slug
	row.DisplayName = in.DisplayName
	row.Definition = in.Definition
	row.SortOrder = in.SortOrder
	row.UpdatedAt = time.Now().UTC()
	if err = s.Messages.UpdateCategoryDefinition(ctx, *row); err != nil {
		return nil, failure("invalid", "update failed", err)
	}
	return row, nil
}
func (s *Service) DeleteCategory(ctx context.Context, user, id uuid.UUID, replacement string) error {
	row, err := s.Messages.GetCategoryDefinitionByID(ctx, user, id)
	if err != nil {
		return failure("storage", "internal", err)
	}
	if row == nil {
		return failure("missing", "not found", nil)
	}
	replacement = strings.TrimSpace(replacement)
	if replacement == "" {
		n, err := s.Messages.CountMessageCategoriesByCategory(ctx, user, id)
		if err != nil {
			return failure("storage", "internal", err)
		}
		if n > 0 {
			return failure("invalid", "category is in use; pass replacement_id", nil)
		}
	} else {
		repl, err := uuid.Parse(replacement)
		if err != nil || repl == id {
			return failure("invalid", "invalid replacement_id", err)
		}
		r, err := s.Messages.GetCategoryDefinitionByID(ctx, user, repl)
		if err != nil {
			return failure("storage", "internal", err)
		}
		if r == nil {
			return failure("invalid", "replacement category not found", nil)
		}
		if _, err = s.Messages.ReassignMessageCategories(ctx, user, id, repl); err != nil {
			return failure("invalid", "reassign failed", err)
		}
	}
	if err = s.Messages.DeleteCategoryDefinition(ctx, user, id); err != nil {
		return failure("invalid", "delete failed", err)
	}
	return nil
}
func (s *Service) ListMessagePage(ctx context.Context, user uuid.UUID, filter domainmessages.MessageListFilter) (*domainmessages.MessagePage, error) {
	filter.OmitBody = true
	rows, err := s.Messages.ListMessages(ctx, user, filter)
	if err != nil {
		return nil, err
	}
	result := &domainmessages.MessagePage{Messages: rows}
	if s.Assignments != nil {
		result.ProjectIDs, err = s.Assignments.EffectiveProjectIDsForMessages(ctx, user, rows)
	}
	return result, err
}
func (s *Service) GetSummaryOverview(ctx context.Context, user uuid.UUID, account *uuid.UUID) (*domainmessages.SummaryOverview, error) {
	snaps, err := s.Summaries.ListSummarySnapshots(ctx, user, account, 1)
	if err != nil {
		return nil, err
	}
	out := &domainmessages.SummaryOverview{}
	if len(snaps) == 0 {
		return out, nil
	}
	out.Snapshot = &snaps[0]
	out.ActionItems, err = s.Summaries.ListOpenActionItems(ctx, user, account)
	if err != nil {
		return nil, err
	}
	out.FYI, err = s.Summaries.ListOpenFYI(ctx, user, account, 200)
	return out, err
}
func (s *Service) GetUserProfile(ctx context.Context, user uuid.UUID) (*domainauth.UserProfile, error) {
	email, err := s.Users.GetUserByID(ctx, user)
	if err != nil {
		return nil, err
	}
	out := &domainauth.UserProfile{Email: email}
	if org, err := s.Users.GetHomeOrganisationID(ctx, user); err == nil {
		out.HomeOrganisationID = &org
	}
	return out, nil
}
func (s *Service) SaveSchedules(ctx context.Context, user uuid.UUID, inputs []domainjobs.ScheduleInput) error {
	now := time.Now().UTC()
	rows := make([]domainjobs.ScheduleChainRow, 0, len(inputs))
	for _, in := range inputs {
		if in.ID == uuid.Nil {
			in.ID = uuid.New()
		}
		if in.IntervalMinutes <= 0 {
			in.IntervalMinutes = 10
		}
		if in.IntervalMinutes > 1440 {
			in.IntervalMinutes = 1440
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "Scheduled chain"
		}
		if len(in.Jobs) == 0 {
			return failure("unprocessable", fmt.Sprintf("%q needs at least one step", name), nil)
		}
		jobs, err := domainjobs.DefaultRegistry().NormalizeScheduleChain(in.Jobs)
		if err != nil {
			return failure("unprocessable", fmt.Sprintf("%q cannot run: %v", name, err), err)
		}
		rows = append(rows, domainjobs.ScheduleChainRow{ID: in.ID, UserID: user, Name: name, AccountID: in.AccountID, Jobs: jobs, IntervalMinutes: in.IntervalMinutes, Enabled: in.Enabled, NextRunAt: now.Add(time.Duration(in.IntervalMinutes) * time.Minute), CreatedAt: now, UpdatedAt: now})
	}
	return s.Schedules.ReplaceSchedulesByUser(ctx, user, rows)
}
func (s *Service) SaveForwardAllowlist(ctx context.Context, user uuid.UUID, inputs []string) error {
	seen := map[string]bool{}
	emails := make([]string, 0, len(inputs))
	for _, email := range inputs {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || seen[email] {
			continue
		}
		if !domainmessages.MessageValidForwardAddress(email) {
			return failure("unprocessable", strconv.Quote(email)+" is not an email address", nil)
		}
		seen[email] = true
		emails = append(emails, email)
	}
	return s.Forwards.ReplaceForwardAllowlist(ctx, user, emails)
}
func (s *Service) validateForwardRule(ctx context.Context, user uuid.UUID, in domainmessages.ForwardRuleInput) (mode, condition, to string, err error) {
	if strings.TrimSpace(in.Name) == "" {
		return "", "", "", failure("unprocessable", "give the rule a name", nil)
	}
	mode, condition, err = domainmessages.MessageNormalizeForwardRule(in.Mode, in.ConditionJSON)
	if err != nil {
		return "", "", "", err
	}
	to = strings.ToLower(strings.TrimSpace(in.ForwardTo))
	allow, err := s.Forwards.ListForwardAllowlist(ctx, user)
	if err != nil {
		return "", "", "", failure("storage", "internal", err)
	}
	if _, ok := domainmessages.MessageAllowlistSet(allow)[to]; !ok {
		return "", "", "", failure("unprocessable", "the destination must be on your forwarding allowlist", nil)
	}
	return mode, condition, to, nil
}
func (s *Service) SaveForwardRule(ctx context.Context, user, id uuid.UUID, create bool, in domainmessages.ForwardRuleInput) (uuid.UUID, error) {
	if create && s.Accounts != nil {
		account, _, err := s.Accounts.GetAccount(ctx, user, id)
		if err != nil {
			return uuid.Nil, failure("storage", "internal", err)
		}
		if account == nil {
			return uuid.Nil, failure("missing", "not found", nil)
		}
	}
	mode, condition, to, err := s.validateForwardRule(ctx, user, in)
	if err != nil {
		return uuid.Nil, err
	}
	now := time.Now().UTC()
	enabled := in.Enabled != nil && *in.Enabled
	var apply *time.Time
	if create && enabled || !create && strings.TrimSpace(in.Start) != "" {
		var ok bool
		apply, ok = domainmessages.ForwardStart(strings.ToLower(strings.TrimSpace(in.Start)), now)
		if !ok {
			message := `start must be "new" or "existing"`
			if create {
				message = `say whether the rule covers existing mail: start must be "new" or "existing"`
			}
			return uuid.Nil, failure("unprocessable", message, nil)
		}
	}
	row := domainmessages.ForwardRuleRow{ID: id, UserID: user, Name: strings.TrimSpace(in.Name), Mode: mode, ConditionJSON: condition, ForwardTo: to, Enabled: enabled, ApplyFrom: apply, UpdatedAt: now}
	if create {
		row.ID = uuid.New()
		row.AccountID = id
		row.CreatedAt = now
		err = s.Forwards.CreateForwardRule(ctx, row)
	} else {
		err = s.Forwards.UpdateForwardRule(ctx, row)
	}
	return row.ID, err
}
func (s *Service) GetForwardRuleListing(ctx context.Context, user, account uuid.UUID) (*domainmessages.ForwardRuleListing, error) {
	rows, err := s.Forwards.ListForwardRules(ctx, user, account)
	if err != nil {
		return nil, err
	}
	allow, err := s.Forwards.ListForwardAllowlist(ctx, user)
	if err != nil {
		return nil, err
	}
	out := &domainmessages.ForwardRuleListing{Rules: rows, Allowlist: domainmessages.MessageAllowlistSet(allow), Stats: map[uuid.UUID]domainmessages.ForwardRuleStats{}}
	stats, err := s.Forwards.ForwardRuleStats(ctx, user, account)
	if err != nil {
		slog.Default().Warn("forward rule stats", "err", err)
	} else {
		for _, row := range stats {
			out.Stats[row.RuleID] = row
		}
	}
	return out, nil
}

// Keep JSON-shaped rule input outside persistence; the port uses a domain value.

func (s *Service) GetScopedAccount(ctx context.Context, user, id uuid.UUID) (*domainaccounts.AccountRow, error) {
	row, _, err := s.Accounts.GetAccount(ctx, user, id)
	return row, err
}
func (s *Service) OwnsAccount(ctx context.Context, user, id uuid.UUID) (bool, error) {
	if s.Accounts == nil {
		return true, nil
	}
	row, _, err := s.Accounts.GetAccount(ctx, user, id)
	return row != nil, err
}
func (s *Service) hydrateJob(ctx context.Context, user uuid.UUID, row *domainjobs.JobRecord) error {
	row.AccountLabel = nil
	if row.AccountID != nil && s.Accounts != nil {
		account, _, err := s.Accounts.GetAccount(ctx, user, *row.AccountID)
		if err != nil {
			return err
		}
		if account != nil {
			label := strings.TrimSpace(account.Label)
			if label == "" {
				label = strings.TrimSpace(account.PrimaryEmail)
			}
			if label != "" {
				row.AccountLabel = &label
			}
		}
	}
	return nil
}
func (s *Service) CancelRun(ctx context.Context, user, id uuid.UUID) (*domainjobs.JobRecord, error) {
	ttl := s.TerminalTTL
	if ttl <= 0 {
		ttl = domainjobs.DefaultTerminalTTL
	}
	row, err := s.JobStore.RequestCancel(ctx, user, id, time.Now().UTC(), ttl)
	if err != nil {
		return nil, err
	}
	err = s.hydrateJob(ctx, user, row)
	return row, err
}

func (s *Service) SaveSummarySettings(ctx context.Context, user uuid.UUID, include, exclude []string, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = 12
	}
	return s.Summaries.UpsertSummarySettings(ctx, domainmessages.SummarySettingsRow{UserID: user, IncludeCategorySlugs: include, ExcludeCategorySlugs: exclude, ChunkSize: chunkSize, UpdatedAt: time.Now().UTC()})
}
