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

type LLMResponse struct {
	Content string
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
