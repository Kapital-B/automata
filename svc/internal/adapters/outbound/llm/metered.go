package llm

import (
	"context"
	"log/slog"
	"time"

	domainai "github.com/Kapital-B/automata/svc/internal/domain/ai"
	"github.com/google/uuid"
)

// MeteredClient records the usage of every call it forwards, attributed to
// one feature and to the user and account on the call's context.
//
// Each service gets its own MeteredClient naming its feature, so usage is
// attributed without the services knowing they are metered.
type MeteredClient struct {
	Inner domainai.LLMClient
	Usage domainai.LLMUsageRepository
	// Feature names the caller, e.g. "categorize" or "ask".
	Feature string
	// Model is the configured model, recorded when the provider does not
	// name the model that served the call.
	Model string
	Log   *slog.Logger
	Now   func() time.Time
}

var _ domainai.LLMClientWithOptions = (*MeteredClient)(nil)

func (c *MeteredClient) ChatCompletion(ctx context.Context, messages []domainai.LLMMessage) (*domainai.LLMResponse, error) {
	resp, err := c.Inner.ChatCompletion(ctx, messages)
	c.record(ctx, resp, err)
	return resp, err
}

func (c *MeteredClient) ChatCompletionWithOptions(ctx context.Context, messages []domainai.LLMMessage, opts domainai.LLMRequestOptions) (*domainai.LLMResponse, error) {
	resp, err := domainai.ChatCompletion(ctx, c.Inner, messages, opts)
	c.record(ctx, resp, err)
	return resp, err
}

func (c *MeteredClient) record(ctx context.Context, resp *domainai.LLMResponse, callErr error) {
	if c.Usage == nil || callErr != nil || resp == nil {
		return
	}
	userID, accountID := domainai.UsageScopeFrom(ctx)
	model := resp.Usage.Model
	if model == "" {
		model = c.Model
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	row := domainai.LLMUsageRow{
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
