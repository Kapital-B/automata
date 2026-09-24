package microsoft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailboxtest"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
)

// fakeGraph is a stateful stand-in for Graph's mail endpoints: an inbox, a
// change log that delta links replay, and a record of reply/forward calls.
type fakeGraph struct {
	mu       sync.Mutex
	srv      *httptest.Server
	seq      int
	next     int
	inbox    map[string]graphFakeMsg
	events   []graphEvent
	expired  int // delta links issued before this sequence are stale
	sent     []mailboxtest.Sent
	pageSize int
}

type graphFakeMsg struct {
	json map[string]any
	raw  []byte
	seq  int
}

type graphEvent struct {
	seq     int
	id      string
	removed bool
}

func newFakeGraph(t *testing.T) *fakeGraph {
	f := &fakeGraph{inbox: map[string]graphFakeMsg{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGraph) deliver(t *testing.T, raw []byte) string {
	p, err := mailmime.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.seq++
	id := fmt.Sprintf("AAMk-%d", f.next)
	to := []map[string]any{}
	for _, r := range p.To {
		to = append(to, map[string]any{"emailAddress": map[string]any{"name": r.Name, "address": r.Address}})
	}
	f.inbox[id] = graphFakeMsg{raw: raw, seq: f.seq, json: map[string]any{
		"id": id, "conversationId": "conv-" + p.ThreadRoot(), "subject": p.Subject,
		"receivedDateTime": p.Date.UTC().Format(time.RFC3339), "bodyPreview": p.Preview(),
		"hasAttachments": p.HasAttachments,
		"from":           map[string]any{"emailAddress": map[string]any{"name": p.FromName, "address": p.FromAddress}},
		"toRecipients":   to,
		"body":           map[string]any{"contentType": "Text", "content": p.TextBody},
	}}
	f.events = append(f.events, graphEvent{seq: f.seq, id: id})
	return id
}

func (f *fakeGraph) remove(t *testing.T, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.inbox, id)
	f.seq++
	f.events = append(f.events, graphEvent{seq: f.seq, id: id, removed: true})
}

func (f *fakeGraph) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/v1.0")
	base := f.srv.URL + "/v1.0"
	switch {
	case path == "/me/mailFolders/inbox/messages/delta":
		top, _ := strconv.Atoi(r.URL.Query().Get("$top"))
		f.pageSize = top
		f.writeSnapshotPage(w, base, 0, f.seq)
	case path == "/delta-page":
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		snap, _ := strconv.Atoi(r.URL.Query().Get("snap"))
		f.writeSnapshotPage(w, base, offset, snap)
	case path == "/delta-link":
		since, _ := strconv.Atoi(r.URL.Query().Get("since"))
		if since < f.expired {
			w.WriteHeader(http.StatusGone)
			_, _ = w.Write([]byte(`{"error":{"code":"syncStateNotFound"}}`))
			return
		}
		value := []any{}
		for _, e := range f.events {
			if e.seq <= since {
				continue
			}
			if e.removed {
				value = append(value, map[string]any{"id": e.id, "@removed": map[string]any{"reason": "deleted"}})
			} else if m, ok := f.inbox[e.id]; ok {
				value = append(value, m.json)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"value": value, "@odata.deltaLink": fmt.Sprintf("%s/delta-link?since=%d", base, f.seq)})
	case strings.HasPrefix(path, "/me/messages/"):
		rest := strings.TrimPrefix(path, "/me/messages/")
		parts := strings.SplitN(rest, "/", 2)
		id := parts[0]
		m, ok := f.inbox[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"ErrorItemNotFound"}}`))
			return
		}
		action := ""
		if len(parts) == 2 {
			action = parts[1]
		}
		switch action {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
		case "$value":
			w.Header().Set("Content-Type", "message/rfc822")
			_, _ = w.Write(m.raw)
		case "reply":
			f.sent = append(f.sent, mailboxtest.Sent{Kind: "reply", OriginalID: id})
			w.WriteHeader(http.StatusAccepted)
		case "forward":
			var body struct {
				ToRecipients []struct {
					EmailAddress struct {
						Address string `json:"address"`
					} `json:"emailAddress"`
				} `json:"toRecipients"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			to := ""
			if len(body.ToRecipients) > 0 {
				to = body.ToRecipients[0].EmailAddress.Address
			}
			f.sent = append(f.sent, mailboxtest.Sent{Kind: "forward", To: to, OriginalID: id})
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// writeSnapshotPage pages through the inbox as it stood at snap.
func (f *fakeGraph) writeSnapshotPage(w http.ResponseWriter, base string, offset, snap int) {
	ids := make([]string, 0, len(f.inbox))
	for id, m := range f.inbox {
		if m.seq <= snap {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	size := f.pageSize
	if size <= 0 {
		size = 50
	}
	end := offset + size
	if end > len(ids) {
		end = len(ids)
	}
	value := []any{}
	for _, id := range ids[offset:end] {
		value = append(value, f.inbox[id].json)
	}
	out := map[string]any{"value": value}
	if end < len(ids) {
		out["@odata.nextLink"] = fmt.Sprintf("%s/delta-page?offset=%d&snap=%d", base, end, snap)
	} else {
		out["@odata.deltaLink"] = fmt.Sprintf("%s/delta-link?since=%d", base, snap)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func TestGraphMailboxContract(t *testing.T) {
	mailboxtest.RunContractTests(t, func(t *testing.T) *mailboxtest.Harness {
		f := newFakeGraph(t)
		box := &mailbox{graph: &GraphClient{APIRoot: f.srv.URL + "/v1.0"}, token: "token", maxRaw: DefaultMaxRawMessageBytes}
		return &mailboxtest.Harness{
			Box:     box,
			Deliver: f.deliver,
			Remove:  f.remove,
			Sent: func(t *testing.T) []mailboxtest.Sent {
				f.mu.Lock()
				defer f.mu.Unlock()
				return append([]mailboxtest.Sent(nil), f.sent...)
			},
			ExpireCursors: func(t *testing.T) {
				f.mu.Lock()
				defer f.mu.Unlock()
				f.expired = f.seq + 1
			},
		}
	})
}
