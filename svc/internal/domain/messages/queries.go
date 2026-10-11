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
	DeleteForwardRule(ctx context.Context, userID, ruleID uuid.UUID) error
	ListForwardActivity(ctx context.Context, userID, ruleID uuid.UUID, limit int) ([]ForwardActivityRow, error)
}

type MessagesQueries interface {
	CreateCategory(ctx context.Context, user uuid.UUID, in CategoryInput) (*CategoryDefinitionRow, error)
	UpdateCategory(ctx context.Context, user, id uuid.UUID, in CategoryInput) (*CategoryDefinitionRow, error)
	DeleteCategory(ctx context.Context, user, id uuid.UUID, replacement string) error
	ListMessagePage(ctx context.Context, user uuid.UUID, filter MessageListFilter) (*MessagePage, error)
	GetMessage(ctx context.Context, userID uuid.UUID, id uuid.UUID) (*MessageRow, error)
	ListCategoryDefinitions(ctx context.Context, userID uuid.UUID) ([]CategoryDefinitionRow, error)
}

type SummariesQueries interface {
	SaveSummarySettings(ctx context.Context, user uuid.UUID, include, exclude []string, chunkSize int) error
	GetSummaryOverview(ctx context.Context, user uuid.UUID, account *uuid.UUID) (*SummaryOverview, error)
	GetSummarySettings(ctx context.Context, userID uuid.UUID) (*SummarySettingsRow, error)
	MarkActionItemDone(ctx context.Context, userID uuid.UUID, itemID uuid.UUID, at time.Time) error
	DeleteFYI(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	ListDraftSuggestions(ctx context.Context, userID uuid.UUID, accountID *uuid.UUID, limit int) ([]DraftSuggestionRow, error)
	ListSendAttemptsByDraft(ctx context.Context, userID uuid.UUID, draftID uuid.UUID, limit int) ([]SendAttemptRow, error)
}
