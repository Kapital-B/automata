// Package mailmime builds and parses RFC 5322 messages for providers that do
// not compose mail server-side. Graph never needs it; Gmail and IMAP/SMTP do.
package mailmime

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	_ "github.com/emersion/go-message/charset" // registers non-UTF-8 charsets for parsing
	"github.com/emersion/go-message/mail"
)

// Attachment is one attached file.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is an outbound message.
type Message struct {
	From        string
	To          []string
	Cc          []string
	Subject     string
	Body        string // plain text
	Date        time.Time
	MessageID   string // without angle brackets; generated when empty
	InReplyTo   string
	References  []string
	Attachments []Attachment
}

func addresses(in []string) []*mail.Address {
	out := make([]*mail.Address, 0, len(in))
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if parsed, err := mail.ParseAddress(a); err == nil {
			out = append(out, parsed)
			continue
		}
		out = append(out, &mail.Address{Address: a})
	}
	return out
}

// NewMessageID returns a unique id scoped to the sender's domain.
func NewMessageID(from string) string {
	domain := "automata.local"
	if at := strings.LastIndex(from, "@"); at >= 0 && at < len(from)-1 {
		domain = strings.Trim(from[at+1:], "> ")
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "@" + domain
}

func header(m Message) mail.Header {
	var h mail.Header
	date := m.Date
	if date.IsZero() {
		date = time.Now()
	}
	h.SetDate(date)
	h.SetAddressList("From", addresses([]string{m.From}))
	h.SetAddressList("To", addresses(m.To))
	if len(m.Cc) > 0 {
		h.SetAddressList("Cc", addresses(m.Cc))
	}
	h.SetSubject(m.Subject)
	id := m.MessageID
	if id == "" {
		id = NewMessageID(m.From)
	}
	h.SetMsgIDList("Message-Id", []string{id})
	if m.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{m.InReplyTo})
	}
	if len(m.References) > 0 {
		h.SetMsgIDList("References", m.References)
	}
	return h
}

// Build renders a message as RFC 5322 bytes.
func Build(m Message) ([]byte, error) {
	var buf bytes.Buffer
	h := header(m)
	w, err := mail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}
	if err := writeText(w, m.Body); err != nil {
		return nil, err
	}
	for _, a := range m.Attachments {
		var ah mail.AttachmentHeader
		ct := a.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah.Set("Content-Type", ct)
		ah.SetFilename(a.Filename)
		aw, err := w.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		if _, err := aw.Write(a.Data); err != nil {
			return nil, err
		}
		if err := aw.Close(); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeText(w *mail.Writer, body string) error {
	tw, err := w.CreateInline()
	if err != nil {
		return err
	}
	var th mail.InlineHeader
	th.Set("Content-Type", "text/plain; charset=utf-8")
	pw, err := tw.CreatePart(th)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(pw, body); err != nil {
		return err
	}
	if err := pw.Close(); err != nil {
		return err
	}
	return tw.Close()
}

// BuildForward wraps an original message as a message/rfc822 attachment
// under an optional comment. Attaching the original whole, rather than
// re-flowing it into a new body, is what keeps its own attachments and
// formatting intact when the provider cannot forward server-side.
func BuildForward(from, to, comment string, original []byte, date time.Time) ([]byte, error) {
	subject := "Fwd:"
	if parsed, err := Parse(original); err == nil && parsed.Subject != "" {
		subject = "Fwd: " + parsed.Subject
	}
	var buf bytes.Buffer
	h := header(Message{From: from, To: []string{to}, Subject: subject, Date: date})
	w, err := mail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}
	if err := writeText(w, comment); err != nil {
		return nil, err
	}
	var ah mail.AttachmentHeader
	ah.Set("Content-Type", "message/rfc822")
	// RFC 2046 §5.2.1 allows only 7bit, 8bit or binary for message/rfc822.
	ah.Set("Content-Transfer-Encoding", "8bit")
	ah.SetFilename("forwarded.eml")
	aw, err := w.CreateAttachment(ah)
	if err != nil {
		return nil, err
	}
	if _, err := aw.Write(original); err != nil {
		return nil, err
	}
	if err := aw.Close(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// BuildReply answers original, threading it with In-Reply-To and References.
func BuildReply(from string, original []byte, body string, date time.Time) ([]byte, error) {
	parsed, err := Parse(original)
	if err != nil {
		return nil, fmt.Errorf("parse original: %w", err)
	}
	to := parsed.ReplyTo
	if to == "" {
		to = parsed.FromAddress
	}
	if to == "" {
		return nil, fmt.Errorf("original has no sender to reply to")
	}
	subject := parsed.Subject
	if !strings.HasPrefix(strings.ToLower(subject), "re:") {
		subject = "Re: " + subject
	}
	refs := append([]string{}, parsed.References...)
	if parsed.MessageID != "" {
		refs = append(refs, parsed.MessageID)
	}
	return Build(Message{
		From: from, To: []string{to}, Subject: subject, Body: body, Date: date,
		InReplyTo: parsed.MessageID, References: refs,
	})
}
