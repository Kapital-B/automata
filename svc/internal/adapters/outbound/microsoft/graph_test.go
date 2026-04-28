package microsoft

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestListInboxDeltaPaginatesAndReturnsDeltaLink(t *testing.T) {
	var baseURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1.0/me/mailFolders/inbox/messages/delta":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "m1", "subject": "first"},
				},
				"@odata.nextLink": baseURL + "/v1.0/delta-page-2",
			})
		case "/v1.0/delta-page-2":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"value": []map[string]any{
					{"id": "m2", "subject": "second"},
				},
				"@odata.deltaLink": "delta-final",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	baseURL = server.URL

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	res, err := client.ListInboxDelta(context.Background(), "token", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.DeltaLink != "delta-final" {
		t.Fatalf("expected final delta link, got %q", res.DeltaLink)
	}
	if len(res.Messages) != 2 {
		t.Fatalf("expected two messages, got %d", len(res.Messages))
	}
	if res.Messages[0].ID != "m1" || res.Messages[1].ID != "m2" {
		t.Fatalf("unexpected message ids: %#v", res.Messages)
	}
}

func TestListInboxMessagesRetriesOn429(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "throttled"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"id": "m1", "subject": "ok"},
			},
		})
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	msgs, err := client.ListInboxMessages(context.Background(), "token", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected one message after retry, got %d", len(msgs))
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected one retry, got %d attempts", attempts.Load())
	}
}

func TestListInboxDeltaFailsWithoutDeltaLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"id": "m1"},
			},
		})
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	if _, err := client.ListInboxDelta(context.Background(), "token", "", 10); err == nil {
		t.Fatal("expected error when delta link is missing")
	}
}

func TestReplyToMessagePreservesLineBreaksAsHTML(t *testing.T) {
	var gotContentType string
	var gotContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1.0/me/messages/msg-123/reply" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		message := payload["message"].(map[string]any)
		body := message["body"].(map[string]any)
		gotContentType, _ = body["contentType"].(string)
		gotContent, _ = body["content"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	if err := client.ReplyToMessage(context.Background(), "token", "msg-123", "Hi David,\n\nLine two & three"); err != nil {
		t.Fatal(err)
	}
	if gotContentType != "HTML" {
		t.Fatalf("expected HTML content type, got %q", gotContentType)
	}
	const want = "Hi David,<br><br>Line two &amp; three"
	if gotContent != want {
		t.Fatalf("unexpected reply body: got %q want %q", gotContent, want)
	}
}
