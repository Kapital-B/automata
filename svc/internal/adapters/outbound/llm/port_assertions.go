package llm

import domain "github.com/Kapital-B/automata/svc/internal/domain/ai"

var _ domain.LLMClientWithOptions = (*OpenAIClient)(nil)
