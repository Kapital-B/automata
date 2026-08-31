package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

type OpenAIClient struct {
	BaseURL    string
	Model      string
	APIKey     string
	HTTPClient *http.Client
}

func (c *OpenAIClient) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	return c.ChatCompletionWithOptions(ctx, messages, driven.LLMRequestOptions{})
}

func (c *OpenAIClient) ChatCompletionWithOptions(ctx context.Context, messages []driven.LLMMessage, opts driven.LLMRequestOptions) (*driven.LLMResponse, error) {
	if c == nil || c.BaseURL == "" || c.Model == "" {
		return nil, fmt.Errorf("llm not configured")
	}
	type reqMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type reqBody struct {
		Model           string   `json:"model"`
		Temperature     float64  `json:"temperature"`
		MaxTokens       int      `json:"max_tokens,omitempty"`
		ReasoningEffort string   `json:"reasoning_effort,omitempty"`
		Messages        []reqMsg `json:"messages"`
	}
	payload := reqBody{Model: c.Model, Temperature: 0, Messages: make([]reqMsg, 0, len(messages))}
	if opts.MaxOutputTokens > 0 {
		payload.MaxTokens = opts.MaxOutputTokens
	}
	if strings.TrimSpace(opts.ReasoningHint) != "" {
		payload.ReasoningEffort = strings.TrimSpace(opts.ReasoningHint)
	}
	for _, m := range messages {
		payload.Messages = append(payload.Messages, reqMsg{Role: m.Role, Content: m.Content})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("llm status %d", res.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llm empty choices")
	}
	return &driven.LLMResponse{Content: out.Choices[0].Message.Content}, nil
}
