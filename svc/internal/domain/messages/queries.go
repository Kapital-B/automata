package messages

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type ForwardsQueries interface {
	SaveForwardAllowlist(ctx context.Context, user uuid.UUID, inputs []string) error
	SaveForwardRule(ctx context.Context, user, id uuid.UUID, create bool, in ForwardRuleInput) (uuid.UUID, error)
	GetForwardRuleListing(ctx context.Context, user, account uuid.UUID) (*ForwardRuleListing, error)
	ListForwardAllowlist(ctx context.Context, userID uuid.UUID) ([]ForwardAllowlistRow, error)
	ReplaceForwardAllowlist(ctx context.Context, userID uuid.UUID, emails []string) error
	ListForwardRules(ctx context.Context, userID, accountID uuid.UUID) ([]ForwardRuleRow, error)
	CreateForwardRule(ctx context.Context, row ForwardRuleRow) error
	UpdateForwardRule(ctx context.Context, row ForwardRuleRow) error
	DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error
	ForwardRuleStats(ctx context.Context, userID, accountID uuid.UUID) ([]ForwardRuleStats, error)
	ListForwardActivity(ctx context.Context, userID, ruleID uuid.UUID, limit int) ([]ForwardActivityRow, error)
}

type MessagesQueries interface {
	CreateCategory(ctx context.Context, user uuid.UUID, in CategoryInput) (*CategoryDefinitionRow, error)
	UpdateCategory(ctx context.Context, user, id uuid.UUID, in CategoryInput) (*CategoryDefinitionRow, error)
	DeleteCategory(ctx context.Context, user, id uuid.UUID, replacement string) error
	ListMessagePage(ctx context.Context, user uuid.UUID, filter MessageListFilter) (*MessagePage, error)
	ListMessages(ctx context.Context, userID uuid.UUID, filter MessageListFilter) ([]MessageRow, error)
	GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*MessageRow, error)
	ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]CategoryDefinitionRow, error)
	GetCategoryDefinitionByID(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*CategoryDefinitionRow, error)
	CreateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	UpdateCategoryDefinition(ctx context.Context, row CategoryDefinitionRow) error
	DeleteCategoryDefinition(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ReassignMessageCategories(ctx context.Context, userID uuid.UUID, fromCategoryID, toCategoryID uuid.UUID) (int, error)
	CountMessageCategoriesByCategory(ctx context.Context, userID uuid.UUID, categoryID uuid.UUID) (int, error)
}

type SummariesQueries interface {
	SaveSummarySettings(ctx context.Context, user uuid.UUID, include, exclude []string, chunkSize int) error
	GetSummaryOverview(ctx context.Context, user uuid.UUID, account *uuid.UUID) (*SummaryOverview, error)
	GetSummarySettings(ctx context.Context, userID uuid.UUID) (*SummarySettingsRow, error)
	UpsertSummarySettings(ctx context.Context, row SummarySettingsRow) error
	ListSummarySnapshots(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]SummarySnapshotRow, error)
	ListOpenActionItems(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID) ([]ActionItemRow, error)
	MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error
	ListOpenFYI(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]FYIRow, error)
	DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]DraftSuggestionRow, error)
	ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]SendAttemptRow, error)
}
