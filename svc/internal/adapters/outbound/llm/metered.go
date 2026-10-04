package llm

import (
	"context"
	"log/slog"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// MeteredClient records the usage of every call it forwards, attributed to
// one feature and to the user and account on the call's context.
//
// Each service gets its own MeteredClient naming its feature, so usage is
// attributed without the services knowing they are metered.
type MeteredClient struct {
	Inner driven.LLMClient
	Usage driven.LLMUsageRepository
	// Feature names the caller, e.g. "categorize" or "ask".
	Feature string
	// Model is the configured model, recorded when the provider does not
	// name the model that served the call.
	Model string
	Log   *slog.Logger
	Now   func() time.Time
}

var _ driven.LLMClientWithOptions = (*MeteredClient)(nil)

func (c *MeteredClient) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	resp, err := c.Inner.ChatCompletion(ctx, messages)
	c.record(ctx, resp, err)
	return resp, err
}

func (c *MeteredClient) ChatCompletionWithOptions(ctx context.Context, messages []driven.LLMMessage, opts driven.LLMRequestOptions) (*driven.LLMResponse, error) {
	resp, err := driven.ChatCompletion(ctx, c.Inner, messages, opts)
	c.record(ctx, resp, err)
	return resp, err
}

func (c *MeteredClient) record(ctx context.Context, resp *driven.LLMResponse, callErr error) {
	if c.Usage == nil || callErr != nil || resp == nil {
		return
	}
	userID, accountID := driven.UsageScopeFrom(ctx)
	model := resp.Usage.Model
	if model == "" {
		model = c.Model
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	row := driven.LLMUsageRow{
		ID:               uuid.New(),
		UserID:           userID,
		AccountID:        accountID,
		Feature:          c.Feature,
		Model:            model,
		InputTokens:      resp.Usage.InputTokens,
		OutputTokens:     resp.Usage.OutputTokens,
		CacheReadTokens:  resp.Usage.CacheReadTokens,
		CacheWriteTokens: resp.Usage.CacheWriteTokens,
		CreatedAt:        now().UTC(),
	}
	// The call has already been paid for, so record it even if the caller's
	// context is cancelled as the response arrives. A failed write is logged
	// rather than returned: losing a usage row must not fail the user's work.
	if err := c.Usage.InsertLLMUsage(context.WithoutCancel(ctx), row); err != nil {
		log := c.Log
		if log == nil {
			log = slog.Default()
		}
		log.Error("record llm usage", "err", err, "feature", c.Feature)
	}
}
