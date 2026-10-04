package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/aws/aws-sdk-go-v2/aws"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	"github.com/google/uuid"
)

type fakeLLM struct {
	resp    *driven.LLMResponse
	err     error
	gotOpts *driven.LLMRequestOptions
}

func (f *fakeLLM) ChatCompletion(ctx context.Context, _ []driven.LLMMessage) (*driven.LLMResponse, error) {
	return f.resp, f.err
}

func (f *fakeLLM) ChatCompletionWithOptions(ctx context.Context, _ []driven.LLMMessage, opts driven.LLMRequestOptions) (*driven.LLMResponse, error) {
	f.gotOpts = &opts
	return f.resp, f.err
}

type fakeUsageRepo struct {
	rows []driven.LLMUsageRow
	err  error
}

func (f *fakeUsageRepo) InsertLLMUsage(ctx context.Context, row driven.LLMUsageRow) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.rows = append(f.rows, row)
	return f.err
}

func (f *fakeUsageRepo) SumLLMUsageByUser(context.Context, uuid.UUID, time.Time, time.Time) ([]driven.LLMUsageTotal, error) {
	return nil, nil
}

func TestMeteredClientAttributesUsageToScopeAndFeature(t *testing.T) {
	inner := &fakeLLM{resp: &driven.LLMResponse{Content: "ok", Usage: driven.LLMUsage{
		InputTokens: 100, OutputTokens: 20, CacheReadTokens: 400, CacheWriteTokens: 50,
	}}}
	repo := &fakeUsageRepo{}
	at := time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)
	c := &MeteredClient{Inner: inner, Usage: repo, Feature: "categorize", Model: "configured", Now: func() time.Time { return at }}

	userID, accountID := uuid.New(), uuid.New()
	ctx := driven.WithUsageScope(context.Background(), &userID, nil)
	// An inner scope that only knows the account keeps the outer user.
	ctx = driven.WithUsageScope(ctx, nil, &accountID)

	opts := driven.LLMRequestOptions{MaxOutputTokens: 77}
	if _, err := c.ChatCompletionWithOptions(ctx, nil, opts); err != nil {
		t.Fatal(err)
	}
	if inner.gotOpts == nil || inner.gotOpts.MaxOutputTokens != 77 {
		t.Fatalf("options not forwarded: %+v", inner.gotOpts)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(repo.rows))
	}
	r := repo.rows[0]
	if r.UserID == nil || *r.UserID != userID || r.AccountID == nil || *r.AccountID != accountID {
		t.Errorf("scope = %v/%v, want %s/%s", r.UserID, r.AccountID, userID, accountID)
	}
	if r.Feature != "categorize" || r.Model != "configured" || !r.CreatedAt.Equal(at) {
		t.Errorf("feature/model/time = %q/%q/%v", r.Feature, r.Model, r.CreatedAt)
	}
	if r.InputTokens != 100 || r.OutputTokens != 20 || r.CacheReadTokens != 400 || r.CacheWriteTokens != 50 {
		t.Errorf("tokens = %+v", r)
	}
}

func TestMeteredClientRecordsUnscopedCallsAndProviderModel(t *testing.T) {
	inner := &fakeLLM{resp: &driven.LLMResponse{Content: "ok", Usage: driven.LLMUsage{InputTokens: 1, Model: "served-model"}}}
	repo := &fakeUsageRepo{}
	c := &MeteredClient{Inner: inner, Usage: repo, Feature: "ask", Model: "configured"}
	if _, err := c.ChatCompletion(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("an unscoped call must still be recorded, got %d rows", len(repo.rows))
	}
	if r := repo.rows[0]; r.UserID != nil || r.AccountID != nil || r.Model != "served-model" {
		t.Errorf("row = %+v, want nil scope and the provider's model", r)
	}
}

func TestMeteredClientSkipsFailedCallsAndSurvivesWriteErrors(t *testing.T) {
	repo := &fakeUsageRepo{}
	failing := &MeteredClient{Inner: &fakeLLM{err: errors.New("boom")}, Usage: repo, Feature: "ask"}
	if _, err := failing.ChatCompletion(context.Background(), nil); err == nil {
		t.Fatal("inner error must be returned")
	}
	if len(repo.rows) != 0 {
		t.Fatalf("a failed call has no usage to record, got %d rows", len(repo.rows))
	}

	broken := &fakeUsageRepo{err: errors.New("db down")}
	c := &MeteredClient{Inner: &fakeLLM{resp: &driven.LLMResponse{Content: "ok"}}, Usage: broken, Feature: "ask"}
	resp, err := c.ChatCompletion(context.Background(), nil)
	if err != nil || resp == nil || resp.Content != "ok" {
		t.Fatalf("a usage write failure must not fail the call: %v %v", resp, err)
	}
}

func TestMeteredClientRecordsAfterCallerCancels(t *testing.T) {
	repo := &fakeUsageRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	inner := &cancelOnReturn{cancel: cancel}
	c := &MeteredClient{Inner: inner, Usage: repo, Feature: "ask"}
	if _, err := c.ChatCompletion(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("a call that completed is paid for and must be recorded, got %d rows", len(repo.rows))
	}
}

type cancelOnReturn struct{ cancel context.CancelFunc }

func (c *cancelOnReturn) ChatCompletion(context.Context, []driven.LLMMessage) (*driven.LLMResponse, error) {
	c.cancel()
	return &driven.LLMResponse{Content: "ok"}, nil
}

func TestUsageFromBedrock(t *testing.T) {
	for name, tc := range map[string]struct {
		u    *bedrocktypes.TokenUsage
		want driven.LLMUsage
	}{
		"nil": {nil, driven.LLMUsage{}},
		"input excludes cache": {
			&bedrocktypes.TokenUsage{InputTokens: aws.Int32(100), OutputTokens: aws.Int32(20), TotalTokens: aws.Int32(570),
				CacheReadInputTokens: aws.Int32(400), CacheWriteInputTokens: aws.Int32(50)},
			driven.LLMUsage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 400, CacheWriteTokens: 50},
		},
		"input includes cache": {
			&bedrocktypes.TokenUsage{InputTokens: aws.Int32(550), OutputTokens: aws.Int32(20), TotalTokens: aws.Int32(570),
				CacheReadInputTokens: aws.Int32(400), CacheWriteInputTokens: aws.Int32(50)},
			driven.LLMUsage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 400, CacheWriteTokens: 50},
		},
		"no cache": {
			&bedrocktypes.TokenUsage{InputTokens: aws.Int32(100), OutputTokens: aws.Int32(20), TotalTokens: aws.Int32(120)},
			driven.LLMUsage{InputTokens: 100, OutputTokens: 20},
		},
	} {
		if got := usageFromBedrock(tc.u); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestOpenAIClientSplitsCachedTokensOutOfInput(t *testing.T) {
	srv := newJSONServer(t, `{"model":"served","choices":[{"message":{"content":"x"}}],
		"usage":{"prompt_tokens":500,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":400}}}`)
	c := &OpenAIClient{BaseURL: srv, Model: "configured"}
	resp, err := c.ChatCompletion(context.Background(), []driven.LLMMessage{{Role: "user", Content: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	want := driven.LLMUsage{InputTokens: 100, OutputTokens: 30, CacheReadTokens: 400, Model: "served"}
	if resp.Usage != want {
		t.Fatalf("usage = %+v, want %+v", resp.Usage, want)
	}
}
