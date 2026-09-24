package microsoft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

func TestListInboxDeltaReturnsSinglePageCursor(t *testing.T) {
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
	if res.NextCursor != baseURL+"/v1.0/delta-page-2" {
		t.Fatalf("expected next link, got %q", res.NextCursor)
	}
	if res.FinalCursor != "" {
		t.Fatalf("expected final delta link to be deferred, got %q", res.FinalCursor)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("expected one page of messages, got %d", len(res.Messages))
	}
	if res.Messages[0].ID != "m1" {
		t.Fatalf("unexpected message ids: %#v", res.Messages)
	}
}

func TestListInboxDeltaRetriesOn429(t *testing.T) {
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
			"@odata.deltaLink": "delta-final",
		})
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	page, err := client.ListInboxDelta(context.Background(), "token", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("expected one message after retry, got %d", len(page.Messages))
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

// Graph delta reports messages that left the folder as an id plus @removed and
// no other fields. Decoded as an ordinary message that is a full payload of
// empty strings, which downstream writes over the real row.
func TestListInboxDeltaSkipsRemovedTombstones(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{
				{"id": "kept", "subject": "still here"},
				{"id": "gone", "@removed": map[string]any{"reason": "deleted"}},
			},
			"@odata.deltaLink": "delta-final",
		})
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	res, err := client.ListInboxDelta(context.Background(), "token", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(res.Messages))
	}
	if res.Messages[0].ID != "kept" {
		t.Errorf("kept %q, want the message that is still in the folder", res.Messages[0].ID)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "gone" {
		t.Errorf("removed = %v, want the tombstone reported as a removal", res.Removed)
	}
}

// An expired delta link has to be distinguishable from any other failure, so
// sync can fall back to a full enumeration instead of failing forever.
func TestListInboxDeltaReportsAnExpiredCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":"syncStateNotFound"}}`))
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	_, err := client.ListInboxDelta(context.Background(), "token", server.URL+"/v1.0/stale-delta", 10)
	if !errors.Is(err, driven.ErrCursorExpired) {
		t.Fatalf("err = %v, want ErrCursorExpired", err)
	}
}

// Without a resume cursor the same response is just an error: there is no
// cursor to have expired.
func TestListInboxDeltaDoesNotCallAFreshListExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"error":{"code":"syncStateNotFound"}}`))
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	_, err := client.ListInboxDelta(context.Background(), "token", "", 10)
	if err == nil || errors.Is(err, driven.ErrCursorExpired) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
}

func TestGetRawMessageRefusesOversizedMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64))
	}))
	defer server.Close()

	client := &GraphClient{APIRoot: server.URL + "/v1.0"}
	if _, err := client.GetRawMessage(context.Background(), "token", "m1", 32); !errors.Is(err, driven.ErrMailTooLarge) {
		t.Fatalf("err = %v, want ErrMailTooLarge", err)
	}
	raw, err := client.GetRawMessage(context.Background(), "token", "m1", 128)
	if err != nil || len(raw) != 64 {
		t.Fatalf("raw = %d bytes, err = %v; want the whole message", len(raw), err)
	}
}
