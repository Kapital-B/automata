package ai

import (
	"context"
	"github.com/google/uuid"
	"time"
)

type LLMMessage struct {
	Role    string
	Content string
}

type LLMRequestOptions struct {
	MaxOutputTokens int
	ReasoningHint   string
}

// LLMUsage is what one call consumed, as the provider reported it.
//
// InputTokens counts only input billed at the full rate. Cached input is
// reported separately, because it is billed differently: CacheReadTokens at a
// fraction of the input price, CacheWriteTokens at a premium. Adapters whose
// provider folds cached tokens into the input count must split them out.
type LLMUsage struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	// Model is the model that served the call, when the provider says.
	// Empty means the caller's configured model.
	Model string
}

type LLMResponse struct {
	Content string
	Usage   LLMUsage
}

// LLMClient calls an OpenAI-compatible chat completions endpoint.
type LLMClient interface {
	ChatCompletion(ctx context.Context, messages []LLMMessage) (*LLMResponse, error)
}

type LLMClientWithOptions interface {
	LLMClient
	ChatCompletionWithOptions(ctx context.Context, messages []LLMMessage, opts LLMRequestOptions) (*LLMResponse, error)
}

func ChatCompletion(ctx context.Context, client LLMClient, messages []LLMMessage, opts LLMRequestOptions) (*LLMResponse, error) {
	if withOpts, ok := client.(LLMClientWithOptions); ok {
		return withOpts.ChatCompletionWithOptions(ctx, messages, opts)
	}
	return client.ChatCompletion(ctx, messages)
}

// LLMUsageRow records one model call. Cost is deliberately not stored: it is
// derived at read time from a per-model price table, so a price change or a
// comparison between providers never needs a backfill.
type LLMUsageRow struct {
	ID uuid.UUID
	// UserID and AccountID come from the context the call ran under. Either
	// may be nil when the caller set no scope; the row is still written so the
	// gap is visible rather than the spend going missing.
	UserID           *uuid.UUID
	AccountID        *uuid.UUID
	Feature          string
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CreatedAt        time.Time
}

// LLMUsageTotal is the summed usage for one feature and model.
type LLMUsageTotal struct {
	Feature          string
	Model            string
	Calls            int
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

type LLMUsageRepository interface {
	InsertLLMUsage(ctx context.Context, row LLMUsageRow) error
	// SumLLMUsageByUser totals a user's usage in [from, to), per feature and
	// model, ordered by feature then model.
	SumLLMUsageByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]LLMUsageTotal, error)
}

type usageScopeKey struct{}

type usageScope struct {
	userID    *uuid.UUID
	accountID *uuid.UUID
}

// WithUsageScope records who model calls made under ctx are made for, so
// their usage is attributed to that user and account. Nil arguments leave an
// existing value in place, so an outer user scope survives an inner call that
// only knows the account.
func WithUsageScope(ctx context.Context, userID, accountID *uuid.UUID) context.Context {
	cur, _ := ctx.Value(usageScopeKey{}).(usageScope)
	if userID != nil && *userID != uuid.Nil {
		id := *userID
		cur.userID = &id
	}
	if accountID != nil && *accountID != uuid.Nil {
		id := *accountID
		cur.accountID = &id
	}
	return context.WithValue(ctx, usageScopeKey{}, cur)
}

// UsageScopeFrom returns the user and account set by WithUsageScope.
func UsageScopeFrom(ctx context.Context) (userID, accountID *uuid.UUID) {
	cur, _ := ctx.Value(usageScopeKey{}).(usageScope)
	return cur.userID, cur.accountID
}
