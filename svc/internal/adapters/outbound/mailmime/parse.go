package mailmime

import (
	"bytes"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// maxBodyBytes bounds how much of a body part is read into memory.
const maxBodyBytes = 2 << 20

// Parsed is what the rest of the system needs from a raw message.
type Parsed struct {
	MessageID      string
	InReplyTo      []string
	References     []string
	Subject        string
	FromName       string
	FromAddress    string
	ReplyTo        string
	To             []driven.MailRecipient
	Cc             []driven.MailRecipient
	Date           time.Time
	TextBody       string
	HTMLBody       string
	HasAttachments bool
}

func recipients(h mail.Header, key string) []driven.MailRecipient {
	list, err := h.AddressList(key)
	if err != nil || len(list) == 0 {
		return nil
	}
	out := make([]driven.MailRecipient, 0, len(list))
	for _, a := range list {
		out = append(out, driven.MailRecipient{Name: a.Name, Address: a.Address})
	}
	return out
}

// Parse reads a raw message. It is forgiving by design: a malformed part or
// an unknown charset loses that part, not the whole message.
func Parse(raw []byte) (*Parsed, error) {
	r, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil && r == nil {
		return nil, err
	}
	h := r.Header
	p := &Parsed{}
	p.Subject, _ = h.Subject()
	if from := recipients(h, "From"); len(from) > 0 {
		p.FromName, p.FromAddress = from[0].Name, from[0].Address
	}
	if rt := recipients(h, "Reply-To"); len(rt) > 0 {
		p.ReplyTo = rt[0].Address
	}
	p.To = recipients(h, "To")
	p.Cc = recipients(h, "Cc")
	p.Date, _ = h.Date()
	p.MessageID, _ = h.MessageID()
	p.InReplyTo, _ = h.MsgIDList("In-Reply-To")
	p.References, _ = h.MsgIDList("References")

	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch ph := part.Header.(type) {
		case *mail.InlineHeader:
			ct, _, _ := ph.ContentType()
			body, _ := io.ReadAll(io.LimitReader(part.Body, maxBodyBytes))
			switch strings.ToLower(ct) {
			case "text/html":
				if p.HTMLBody == "" {
					p.HTMLBody = string(body)
				}
			case "text/plain", "":
				if p.TextBody == "" {
					p.TextBody = string(body)
				}
			default:
				// An inline image or similar still counts as an attachment.
				if _, params, _ := ph.ContentDisposition(); params["filename"] != "" {
					p.HasAttachments = true
				}
			}
		case *mail.AttachmentHeader:
			p.HasAttachments = true
		}
	}
	return p, nil
}

// Preview is a short plain-text excerpt for list views.
func (p *Parsed) Preview() string {
	s := strings.Join(strings.Fields(p.TextBody), " ")
	if len([]rune(s)) > 255 {
		s = string([]rune(s)[:255])
	}
	return s
}

// ToMailMessage maps a parsed message onto the port's shape. id and
// conversationID come from the provider, since only it knows them.
func (p *Parsed) ToMailMessage(id, conversationID string, received time.Time) driven.MailMessage {
	body, contentType := p.TextBody, "Text"
	if p.HTMLBody != "" {
		body, contentType = p.HTMLBody, "HTML"
	}
	if received.IsZero() {
		received = p.Date
	}
	return driven.MailMessage{
		ID:               id,
		ConversationID:   conversationID,
		ReceivedDateTime: received.UTC().Format(time.RFC3339),
		Subject:          p.Subject,
		FromName:         p.FromName,
		FromAddress:      p.FromAddress,
		ToRecipients:     p.To,
		CcRecipients:     p.Cc,
		BodyPreview:      p.Preview(),
		BodyContent:      body,
		BodyContentType:  contentType,
		HasAttachments:   p.HasAttachments,
	}
}

// ThreadRoot is the id a conversation is keyed on when the provider has no
// thread id of its own: the first message the thread refers back to.
func (p *Parsed) ThreadRoot() string {
	if len(p.References) > 0 {
		return p.References[0]
	}
	if len(p.InReplyTo) > 0 {
		return p.InReplyTo[0]
	}
	return p.MessageID
}
