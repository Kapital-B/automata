package driven

import "context"

type LLMMessage struct {
	Role    string
	Content string
}

type LLMResponse struct {
	Content string
}

// LLMClient calls an OpenAI-compatible chat completions endpoint.
type LLMClient interface {
	ChatCompletion(ctx context.Context, messages []LLMMessage) (*LLMResponse, error)
}
