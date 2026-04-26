package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

func TestOpenAIClientChatCompletion(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"schema_version\":1,\"category_slug\":\"important\",\"confidence\":0.9}"}}]}`))
	}))
	defer srv.Close()

	c := &OpenAIClient{
		BaseURL: srv.URL,
		Model:   "local-model",
		APIKey:  "abc",
	}
	resp, err := c.ChatCompletion(context.Background(), []driven.LLMMessage{
		{Role: "user", Content: "classify"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil || resp.Content == "" {
		t.Fatal("expected content")
	}
	if gotAuth != "Bearer abc" {
		t.Fatalf("expected auth header, got %q", gotAuth)
	}
}

func TestOpenAIClientNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := &OpenAIClient{BaseURL: srv.URL, Model: "x"}
	if _, err := c.ChatCompletion(context.Background(), []driven.LLMMessage{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected non-2xx error")
	}
}
