package mailmime

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func sample(t *testing.T) []byte {
	t.Helper()
	raw, err := Build(Message{
		From:      "Pat <pat@example.com>",
		To:        []string{"dee@example.org"},
		Subject:   "Pump P-03 duty",
		Body:      "Confirmed at 90 kW.",
		Date:      time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
		MessageID: "orig-1@example.com",
		Attachments: []Attachment{
			{Filename: "datasheet.pdf", ContentType: "application/pdf", Data: []byte("%PDF-sample")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestBuildParseRoundTrip(t *testing.T) {
	p, err := Parse(sample(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Pump P-03 duty" || p.FromAddress != "pat@example.com" || p.FromName != "Pat" {
		t.Fatalf("headers = %+v", p)
	}
	if p.MessageID != "orig-1@example.com" {
		t.Errorf("message id = %q", p.MessageID)
	}
	if !strings.Contains(p.TextBody, "90 kW") {
		t.Errorf("body = %q", p.TextBody)
	}
	if !p.HasAttachments {
		t.Error("attachment not detected")
	}
	if len(p.To) != 1 || p.To[0].Address != "dee@example.org" {
		t.Errorf("to = %+v", p.To)
	}
}

// Forwarding by re-send must carry the original whole, including its own
// attachments — that is the property a server-side forward gave for free.
func TestBuildForwardEmbedsTheOriginal(t *testing.T) {
	original := sample(t)
	fwd, err := BuildForward("me@example.com", "bills@example.com", "fyi", original, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(fwd)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Fwd: Pump P-03 duty" {
		t.Errorf("subject = %q", p.Subject)
	}
	if !bytes.Contains(fwd, []byte("message/rfc822")) {
		t.Error("original not attached as message/rfc822")
	}
	if !bytes.Contains(fwd, []byte("%PDF-sample")) && !bytes.Contains(fwd, []byte("JVBERi1zYW1wbGU=")) {
		t.Error("original's own attachment did not survive the forward")
	}
	if !strings.Contains(p.TextBody, "fyi") {
		t.Errorf("comment missing: %q", p.TextBody)
	}
}

func TestBuildReplyThreads(t *testing.T) {
	reply, err := BuildReply("dee@example.org", sample(t), "Thanks", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(reply)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Re: Pump P-03 duty" {
		t.Errorf("subject = %q", p.Subject)
	}
	if len(p.To) != 1 || p.To[0].Address != "pat@example.com" {
		t.Errorf("reply went to %+v, want the original sender", p.To)
	}
	if len(p.InReplyTo) != 1 || p.InReplyTo[0] != "orig-1@example.com" {
		t.Errorf("in-reply-to = %v", p.InReplyTo)
	}
	if p.ThreadRoot() != "orig-1@example.com" {
		t.Errorf("thread root = %q, want the original", p.ThreadRoot())
	}
}

// Real mail is not all UTF-8; a Latin-1 message must still yield its text.
func TestParseDecodesNonUTF8Charsets(t *testing.T) {
	raw := []byte("From: a@example.com\r\nSubject: =?iso-8859-1?q?Caf=E9?=\r\n" +
		"Content-Type: text/plain; charset=iso-8859-1\r\n\r\nCaf\xe9 at noon\r\n")
	p, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if p.Subject != "Café" {
		t.Errorf("subject = %q", p.Subject)
	}
	if !strings.Contains(p.TextBody, "Café") {
		t.Errorf("body = %q", p.TextBody)
	}
}
