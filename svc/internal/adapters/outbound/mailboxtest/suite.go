// Package mailboxtest is the contract every mail provider adapter must pass.
// A provider without it running in CI should not ship: the job store taught
// this repository that the implementation nobody tests is the one that breaks.
package mailboxtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// Sent is one thing a provider was asked to deliver.
type Sent struct {
	Kind string // "forward" or "reply"
	To   string
	// OriginalID is set when the provider acted server-side on a message.
	OriginalID string
	// Raw is set when the adapter composed and sent the message itself.
	Raw []byte
}

// Harness drives one adapter against a fake of its provider.
type Harness struct {
	Box driven.Mailbox
	// Deliver puts a message in the inbox and returns its provider id.
	Deliver func(t *testing.T, raw []byte) string
	// Remove takes a message out of the inbox, as a move or delete would.
	Remove func(t *testing.T, providerID string)
	// Sent reports everything the provider was asked to deliver.
	Sent func(t *testing.T) []Sent
	// ExpireCursors makes every issued resume cursor stale. Nil skips the
	// check for a provider with no such notion.
	ExpireCursors func(t *testing.T)
}

type Factory func(t *testing.T) *Harness

func message(t *testing.T, subject string, withAttachment bool) []byte {
	t.Helper()
	m := mailmime.Message{
		From:      "Pat Sender <pat@example.com>",
		To:        []string{"owner@example.org"},
		Subject:   subject,
		Body:      "Body of " + subject,
		Date:      time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
		MessageID: mailmime.NewMessageID("pat@example.com"),
	}
	if withAttachment {
		m.Attachments = []mailmime.Attachment{{Filename: "spec.pdf", ContentType: "application/pdf", Data: []byte("%PDF-contract")}}
	}
	raw, err := mailmime.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// drain lists from cursor to completion, checking the page invariant.
func drain(t *testing.T, box driven.Mailbox, cursor string, pageSize int) (msgs []driven.MailMessage, removed []string, final string) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		page, err := box.ListChanges(ctx, cursor, pageSize)
		if err != nil {
			t.Fatalf("list changes: %v", err)
		}
		if (page.NextCursor == "") == (page.FinalCursor == "") {
			t.Fatalf("a page must continue or complete, not both or neither: next=%q final=%q", page.NextCursor, page.FinalCursor)
		}
		msgs = append(msgs, page.Messages...)
		removed = append(removed, page.Removed...)
		if page.FinalCursor != "" {
			return msgs, removed, page.FinalCursor
		}
		cursor = page.NextCursor
	}
	t.Fatal("enumeration did not complete")
	return nil, nil, ""
}

func byID(msgs []driven.MailMessage) map[string]driven.MailMessage {
	out := map[string]driven.MailMessage{}
	for _, m := range msgs {
		out[m.ID] = m
	}
	return out
}

// RunContractTests asserts the RFC multi-provider mail §8 contract.
func RunContractTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("enumeration_returns_full_payloads", func(t *testing.T) {
		h := factory(t)
		ids := map[string]string{}
		for i := 0; i < 3; i++ {
			subject := fmt.Sprintf("Contract %d", i)
			ids[h.Deliver(t, message(t, subject, false))] = subject
		}
		msgs, _, final := drain(t, h.Box, "", 10)
		if final == "" {
			t.Fatal("no final cursor")
		}
		got := byID(msgs)
		for id, subject := range ids {
			m, ok := got[id]
			if !ok {
				t.Fatalf("message %s missing from enumeration", id)
			}
			if m.Subject != subject || m.FromAddress != "pat@example.com" {
				t.Errorf("message %s = %q from %q, want %q from pat@example.com", id, m.Subject, m.FromAddress, subject)
			}
			if _, err := time.Parse(time.RFC3339, m.ReceivedDateTime); err != nil {
				t.Errorf("message %s has no usable received time: %q", id, m.ReceivedDateTime)
			}
			if !strings.Contains(m.BodyContent+m.BodyPreview, "Body of") {
				t.Errorf("message %s has no body", id)
			}
		}
	})

	t.Run("pages_continue_until_complete", func(t *testing.T) {
		h := factory(t)
		want := map[string]bool{}
		for i := 0; i < 5; i++ {
			want[h.Deliver(t, message(t, fmt.Sprintf("Paged %d", i), false))] = true
		}
		msgs, _, _ := drain(t, h.Box, "", 2)
		got := byID(msgs)
		for id := range want {
			if _, ok := got[id]; !ok {
				t.Errorf("message %s lost across pages", id)
			}
		}
	})

	t.Run("resume_picks_up_new_mail", func(t *testing.T) {
		h := factory(t)
		old := h.Deliver(t, message(t, "Before", false))
		_, _, final := drain(t, h.Box, "", 10)
		fresh := h.Deliver(t, message(t, "After", false))
		msgs, _, _ := drain(t, h.Box, final, 10)
		got := byID(msgs)
		if _, ok := got[fresh]; !ok {
			t.Fatal("resume did not return the new message")
		}
		if h.Box.Capabilities().IncrementalSync {
			if _, ok := got[old]; ok {
				t.Error("an incremental provider re-returned a message it already reported")
			}
		}
	})

	// A removal is never a message with empty fields. That is how a Graph
	// tombstone once blanked the sender and subject of a real row.
	t.Run("removal_is_reported_never_as_an_empty_message", func(t *testing.T) {
		h := factory(t)
		id := h.Deliver(t, message(t, "Leaving", false))
		_, _, final := drain(t, h.Box, "", 10)
		h.Remove(t, id)
		msgs, removed, _ := drain(t, h.Box, final, 10)
		for _, m := range msgs {
			if m.Subject == "" && m.FromAddress == "" {
				t.Fatalf("change page carried an empty message %q", m.ID)
			}
		}
		if h.Box.Capabilities().ReportsRemovals {
			found := false
			for _, r := range removed {
				found = found || r == id
			}
			if !found {
				t.Errorf("removal of %s not reported (removed=%v)", id, removed)
			}
		}
	})

	t.Run("raw_message_keeps_attachments", func(t *testing.T) {
		h := factory(t)
		id := h.Deliver(t, message(t, "With attachment", true))
		raw, err := h.Box.GetRawMessage(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		p, err := mailmime.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if p.Subject != "With attachment" || !p.HasAttachments {
			t.Errorf("raw = subject %q attachments %v", p.Subject, p.HasAttachments)
		}
	})

	t.Run("forward_delivers", func(t *testing.T) {
		h := factory(t)
		id := h.Deliver(t, message(t, "Invoice 42", true))
		if err := h.Box.Forward(context.Background(), id, "bills@example.net", "fyi"); err != nil {
			t.Fatal(err)
		}
		sent := h.Sent(t)
		if len(sent) != 1 || sent[0].Kind != "forward" || sent[0].To != "bills@example.net" {
			t.Fatalf("sent = %+v, want one forward to bills@example.net", sent)
		}
		if h.Box.Capabilities().ServerSideForward {
			if sent[0].OriginalID != id {
				t.Errorf("server-side forward of %q, want %q", sent[0].OriginalID, id)
			}
			return
		}
		// Re-sent: the original has to ride along whole.
		p, err := mailmime.Parse(sent[0].Raw)
		if err != nil {
			t.Fatal(err)
		}
		if p.Subject != "Fwd: Invoice 42" || !p.HasAttachments {
			t.Errorf("forwarded copy = subject %q attachments %v", p.Subject, p.HasAttachments)
		}
		if !strings.Contains(string(sent[0].Raw), "message/rfc822") {
			t.Error("original not embedded as message/rfc822")
		}
	})

	t.Run("reply_threads", func(t *testing.T) {
		h := factory(t)
		raw := message(t, "Question", false)
		original, _ := mailmime.Parse(raw)
		id := h.Deliver(t, raw)
		if err := h.Box.Reply(context.Background(), id, "Answer"); err != nil {
			t.Fatal(err)
		}
		sent := h.Sent(t)
		if len(sent) != 1 || sent[0].Kind != "reply" {
			t.Fatalf("sent = %+v, want one reply", sent)
		}
		if h.Box.Capabilities().ServerSideReply {
			return
		}
		p, err := mailmime.Parse(sent[0].Raw)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.InReplyTo) != 1 || p.InReplyTo[0] != original.MessageID {
			t.Errorf("reply in-reply-to = %v, want %s", p.InReplyTo, original.MessageID)
		}
		if len(p.To) != 1 || p.To[0].Address != "pat@example.com" {
			t.Errorf("reply to = %+v", p.To)
		}
	})

	// Forward rules rely on this to decide between retrying and recording an
	// unknown outcome: nothing may be sent for a message that does not exist.
	t.Run("forwarding_a_missing_message_is_not_sent", func(t *testing.T) {
		h := factory(t)
		err := h.Box.Forward(context.Background(), "no-such-message", "bills@example.net", "")
		if !errors.Is(err, driven.ErrMailNotSent) {
			t.Fatalf("err = %v, want ErrMailNotSent", err)
		}
		if sent := h.Sent(t); len(sent) != 0 {
			t.Errorf("sent %+v for a missing message", sent)
		}
	})

	t.Run("expired_cursor_is_reported", func(t *testing.T) {
		h := factory(t)
		if h.ExpireCursors == nil {
			t.Skip("provider has no expiring cursors")
		}
		h.Deliver(t, message(t, "Seed", false))
		_, _, final := drain(t, h.Box, "", 10)
		h.ExpireCursors(t)
		_, err := h.Box.ListChanges(context.Background(), final, 10)
		if !errors.Is(err, driven.ErrCursorExpired) {
			t.Fatalf("err = %v, want ErrCursorExpired", err)
		}
	})
}
