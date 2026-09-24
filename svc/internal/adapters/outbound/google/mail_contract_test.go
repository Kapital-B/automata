package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailboxtest"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

type fakeGmailMsg struct {
	raw    []byte
	thread string
	labels []string
	date   int64
}

type fakeGmailEvent struct {
	hid     int
	id      string
	removed bool
}

// fakeGmail is a stateful stand-in for the Gmail API and Google's token
// endpoint.
type fakeGmail struct {
	mu      sync.Mutex
	srv     *httptest.Server
	next    int
	hid     int
	msgs    map[string]*fakeGmailMsg
	events  []fakeGmailEvent
	expired int
	sent    []mailboxtest.Sent
	// refresh tokens the token endpoint accepts
	validRefresh map[string]bool
}

func newFakeGmail(t *testing.T) *fakeGmail {
	f := &fakeGmail{msgs: map[string]*fakeGmailMsg{}, hid: 1000, validRefresh: map[string]bool{"good-refresh": true}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (f *fakeGmail) deliver(t *testing.T, raw []byte) string {
	p, err := mailmime.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.hid++
	id := fmt.Sprintf("18c%05d", f.next)
	f.msgs[id] = &fakeGmailMsg{raw: raw, thread: "t-" + p.ThreadRoot(), labels: []string{"INBOX", "UNREAD"}, date: p.Date.UnixMilli()}
	f.events = append(f.events, fakeGmailEvent{hid: f.hid, id: id})
	return id
}

// remove archives the message: it leaves the inbox but still exists, which is
// the common case and the one a history replay has to get right.
func (f *fakeGmail) remove(t *testing.T, id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.msgs[id]; ok {
		m.labels = []string{"UNREAD"}
	}
	f.hid++
	f.events = append(f.events, fakeGmailEvent{hid: f.hid, id: id, removed: true})
}

func (f *fakeGmail) payload(m *fakeGmailMsg) map[string]any {
	p, _ := mailmime.Parse(m.raw)
	headers := []map[string]string{{"name": "Subject", "value": p.Subject}}
	if p.FromAddress != "" {
		headers = append(headers, map[string]string{"name": "From", "value": fmt.Sprintf("%q <%s>", p.FromName, p.FromAddress)})
	}
	var to []string
	for _, r := range p.To {
		to = append(to, r.Address)
	}
	headers = append(headers, map[string]string{"name": "To", "value": strings.Join(to, ", ")})
	parts := []map[string]any{{
		"mimeType": "text/plain", "filename": "",
		"headers": []map[string]string{{"name": "Content-Type", "value": "text/plain; charset=UTF-8"}},
		"body":    map[string]any{"data": b64([]byte(p.TextBody))},
	}}
	if p.HasAttachments {
		parts = append(parts, map[string]any{
			"mimeType": "application/pdf", "filename": "spec.pdf",
			"body": map[string]any{"attachmentId": "att-1"},
		})
	}
	return map[string]any{"mimeType": "multipart/mixed", "headers": headers, "parts": parts}
}

func (f *fakeGmail) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/token" {
		_ = r.ParseForm()
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at", "refresh_token": "good-refresh"})
		case "refresh_token":
			if !f.validRefresh[r.Form.Get("refresh_token")] {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": "Token has been expired or revoked."})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "at"})
		}
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/gmail/v1/users/me")
	q := r.URL.Query()
	switch {
	case path == "/profile":
		_ = json.NewEncoder(w).Encode(map[string]string{"emailAddress": "owner@example.org", "historyId": strconv.Itoa(f.hid)})
	case path == "/messages" && r.Method == http.MethodGet:
		var ids []string
		for id, m := range f.msgs {
			if hasInbox(m.labels) {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		offset, _ := strconv.Atoi(q.Get("pageToken"))
		size, _ := strconv.Atoi(q.Get("maxResults"))
		end := offset + size
		if end > len(ids) {
			end = len(ids)
		}
		refs := []map[string]string{}
		for _, id := range ids[offset:end] {
			refs = append(refs, map[string]string{"id": id})
		}
		out := map[string]any{"messages": refs}
		if end < len(ids) {
			out["nextPageToken"] = strconv.Itoa(end)
		}
		_ = json.NewEncoder(w).Encode(out)
	case path == "/messages/send":
		var body struct {
			Raw string `json:"raw"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := base64.RawURLEncoding.DecodeString(body.Raw)
		p, _ := mailmime.Parse(raw)
		kind := "send"
		switch {
		case strings.HasPrefix(p.Subject, "Fwd:"):
			kind = "forward"
		case strings.HasPrefix(p.Subject, "Re:"):
			kind = "reply"
		}
		to := ""
		if len(p.To) > 0 {
			to = p.To[0].Address
		}
		f.sent = append(f.sent, mailboxtest.Sent{Kind: kind, To: to, Raw: raw})
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "sent-1"})
	case strings.HasPrefix(path, "/messages/"):
		id, _ := url.PathUnescape(strings.TrimPrefix(path, "/messages/"))
		m, ok := f.msgs[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Requested entity was not found."}}`))
			return
		}
		out := map[string]any{"id": id, "threadId": m.thread, "labelIds": m.labels, "internalDate": strconv.FormatInt(m.date, 10), "historyId": strconv.Itoa(f.hid)}
		if q.Get("format") == "raw" {
			out["raw"] = b64(m.raw)
		} else {
			out["payload"] = f.payload(m)
		}
		_ = json.NewEncoder(w).Encode(out)
	case path == "/history":
		start, _ := strconv.Atoi(q.Get("startHistoryId"))
		if start < f.expired {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Requested entity was not found."}}`))
			return
		}
		records := []map[string]any{}
		for _, e := range f.events {
			if e.hid <= start {
				continue
			}
			msg := map[string]any{"id": e.id, "labelIds": []string{"INBOX"}}
			if e.removed {
				records = append(records, map[string]any{"id": strconv.Itoa(e.hid), "labelsRemoved": []map[string]any{{"message": msg, "labelIds": []string{"INBOX"}}}})
			} else {
				records = append(records, map[string]any{"id": strconv.Itoa(e.hid), "messagesAdded": []map[string]any{{"message": msg}}})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"history": records, "historyId": strconv.Itoa(f.hid)})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGmail) provider() *MailProvider {
	return &MailProvider{
		ClientID: "cid", ClientSecret: "secret", RedirectURI: "http://localhost/cb",
		TokenURL: f.srv.URL + "/token", APIRoot: f.srv.URL + "/gmail/v1",
	}
}

func TestGmailMailboxContract(t *testing.T) {
	mailboxtest.RunContractTests(t, func(t *testing.T) *mailboxtest.Harness {
		f := newFakeGmail(t)
		box := f.provider().mailbox("at", "owner@example.org")
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
				f.expired = f.hid + 1
			},
		}
	})
}

// A revoked or expired refresh token — including the 7-day expiry of an
// External client in Testing mode — must say so, so the account can be marked
// for reconnection instead of every sync failing the same way.
func TestGmailOpenReportsRejectedCredentials(t *testing.T) {
	f := newFakeGmail(t)
	cred, _ := json.Marshal(credential{Type: credentialType, RefreshToken: "revoked"})
	_, _, err := f.provider().Open(context.Background(), driven.AccountRow{PrimaryEmail: "owner@example.org"}, cred)
	if !errors.Is(err, driven.ErrCredentialsRejected) {
		t.Fatalf("err = %v, want ErrCredentialsRejected", err)
	}
}

func TestGmailOpenDoesNotRewriteAnUnrotatedCredential(t *testing.T) {
	f := newFakeGmail(t)
	cred, _ := json.Marshal(credential{Type: credentialType, RefreshToken: "good-refresh"})
	box, rotated, err := f.provider().Open(context.Background(), driven.AccountRow{PrimaryEmail: "owner@example.org"}, cred)
	if err != nil {
		t.Fatal(err)
	}
	if box == nil || rotated != nil {
		t.Fatalf("box=%v rotated=%s; want an open mailbox and nothing to persist", box, rotated)
	}
}

// A Microsoft credential stored against a Google account must not be sent to
// Google as if it were a refresh token.
func TestGmailOpenRefusesAForeignCredential(t *testing.T) {
	f := newFakeGmail(t)
	_, _, err := f.provider().Open(context.Background(), driven.AccountRow{}, []byte(`{"refresh_token":"x","ms_account_kind":"work"}`))
	if err == nil {
		t.Fatal("expected a foreign credential to be refused")
	}
}

func TestGmailCompleteLearnsTheMailbox(t *testing.T) {
	f := newFakeGmail(t)
	got, err := f.provider().Complete(context.Background(), "code", driven.ConnectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Email != "owner@example.org" || got.DefaultLabel != "owner@example.org" {
		t.Errorf("connected = %+v", got)
	}
	var c credential
	if err := json.Unmarshal(got.Credential, &c); err != nil || c.Type != credentialType || c.RefreshToken != "good-refresh" {
		t.Errorf("credential = %s (%v)", got.Credential, err)
	}
}

func TestGmailAuthorizationURLAsksForOfflineConsent(t *testing.T) {
	u, err := (&MailProvider{ClientID: "cid", RedirectURI: "http://localhost/cb"}).AuthorizationURL(context.Background(), "st", driven.ConnectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	if q.Get("access_type") != "offline" || q.Get("prompt") != "consent" || q.Get("state") != "st" {
		t.Errorf("url = %s", u)
	}
	if !strings.Contains(q.Get("scope"), "gmail.readonly") || !strings.Contains(q.Get("scope"), "gmail.send") {
		t.Errorf("scope = %q", q.Get("scope"))
	}
	if strings.Contains(q.Get("scope"), "mail.google.com") {
		t.Error("asked for full mailbox access; readonly and send are enough")
	}
}
