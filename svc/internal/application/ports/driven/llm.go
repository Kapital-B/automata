package driven

import "context"

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
