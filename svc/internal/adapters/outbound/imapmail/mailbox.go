package imapmail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const (
	// maxPageSize bounds one page: every message on it is fetched whole up to
	// syncFetchBytes, so a page costs up to their product in memory.
	maxPageSize = 50
	// syncFetchBytes is how much of each message sync reads. The text body
	// comes first in nearly every message; attachments after it are not
	// needed to list mail and are fetched whole only to forward.
	syncFetchBytes = 256 << 10

	cursorPrefix = "v1"
)

type mailbox struct {
	p     *Provider
	cred  credential
	email string
}

// IMAP has no change log to replay, only UIDs that grow as mail arrives, so
// sync sees new mail but not removals or edits. Ids are the Message-ID header
// where there is one, which survives a UIDVALIDITY reset that would renumber
// every UID; mail without one falls back to the UID.
func (m *mailbox) Capabilities() driven.MailboxCapabilities {
	return driven.MailboxCapabilities{
		IncrementalSync:   true,
		ServerSideForward: false,
		ServerSideReply:   false,
		ReportsRemovals:   false,
		StableMessageIDs:  false,
	}
}

func (m *mailbox) session(ctx context.Context) (*client.Client, *imap.MailboxStatus, error) {
	cl, status, err := m.p.imapSession(ctx, m.cred)
	if errors.Is(err, errLoginRefused) {
		err = fmt.Errorf("%w: %v", driven.ErrCredentialsRejected, err)
	}
	return cl, status, err
}

// Cursor grammar: "v1:<uidvalidity>:<last uid seen>". One shape serves as
// both next and final cursor, since resuming mid-enumeration and resuming
// after it are the same question: what arrived after this UID.
func formatCursor(validity, uid uint32) string {
	return fmt.Sprintf("%s:%d:%d", cursorPrefix, validity, uid)
}

func parseCursor(c string) (validity, uid uint32, err error) {
	parts := strings.Split(c, ":")
	if len(parts) != 3 || parts[0] != cursorPrefix {
		return 0, 0, fmt.Errorf("imap: unrecognised cursor %q", c)
	}
	v, err1 := strconv.ParseUint(parts[1], 10, 32)
	u, err2 := strconv.ParseUint(parts[2], 10, 32)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("imap: unrecognised cursor %q", c)
	}
	return uint32(v), uint32(u), nil
}

func (m *mailbox) ListChanges(ctx context.Context, cursor string, pageSize int) (*driven.MailChangePage, error) {
	var after uint32
	var wantValidity uint32
	if cursor != "" {
		var err error
		if wantValidity, after, err = parseCursor(cursor); err != nil {
			// A cursor we cannot read is one we cannot resume from.
			return nil, fmt.Errorf("%w: %v", driven.ErrCursorExpired, err)
		}
	}
	cl, status, err := m.session(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cl.Logout() }()
	if cursor != "" && status.UidValidity != wantValidity {
		// The server renumbered the mailbox; our UIDs mean nothing now.
		return nil, fmt.Errorf("%w: uidvalidity changed from %d to %d", driven.ErrCursorExpired, wantValidity, status.UidValidity)
	}
	if pageSize <= 0 || pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	var uids []uint32
	if status.Messages > 0 {
		set := new(imap.SeqSet)
		set.AddRange(after+1, 0)
		found, err := cl.UidSearch(&imap.SearchCriteria{Uid: set})
		if err != nil {
			return nil, fmt.Errorf("imap search: %w", err)
		}
		// "n:*" always matches the highest UID, even below n.
		for _, u := range found {
			if u > after {
				uids = append(uids, u)
			}
		}
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	}
	more := len(uids) > pageSize
	if more {
		uids = uids[:pageSize]
	}
	msgs, err := m.fetchForSync(cl, status.UidValidity, uids)
	if err != nil {
		return nil, err
	}
	page := &driven.MailChangePage{Messages: msgs}
	last := after
	if len(uids) > 0 {
		last = uids[len(uids)-1]
	}
	if more {
		page.NextCursor = formatCursor(status.UidValidity, last)
	} else {
		page.FinalCursor = formatCursor(status.UidValidity, last)
	}
	return page, nil
}

func (m *mailbox) fetchForSync(cl *client.Client, validity uint32, uids []uint32) ([]driven.MailMessage, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	set := new(imap.SeqSet)
	set.AddNum(uids...)
	section := &imap.BodySectionName{Peek: true, Partial: []int{0, syncFetchBytes}}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchInternalDate, imap.FetchBodyStructure, section.FetchItem()}
	ch := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() { done <- cl.UidFetch(set, items, ch) }()
	byUID := map[uint32]driven.MailMessage{}
	for msg := range ch {
		byUID[msg.Uid] = toMailMessage(validity, msg, section)
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	out := make([]driven.MailMessage, 0, len(uids))
	for _, u := range uids {
		if mm, ok := byUID[u]; ok {
			out = append(out, mm)
		}
	}
	return out, nil
}

func messageID(validity, uid uint32, headerID string) string {
	if headerID != "" {
		return "mid:" + headerID
	}
	return fmt.Sprintf("uid:%d:%d", validity, uid)
}

func toMailMessage(validity uint32, msg *imap.Message, section *imap.BodySectionName) driven.MailMessage {
	var raw []byte
	if body := msg.GetBody(section); body != nil {
		raw, _ = io.ReadAll(body)
	}
	p, err := mailmime.Parse(raw)
	if err != nil {
		// Unreadable headers still have to become a row, or one bad message
		// would stall sync behind it forever. Mark it rather than blank it.
		id := messageID(validity, msg.Uid, "")
		return driven.MailMessage{
			ID:               id,
			ConversationID:   id,
			ReceivedDateTime: msg.InternalDate.UTC().Format(time.RFC3339),
			Subject:          "(unreadable message)",
			BodyPreview:      "Automata could not parse this message's headers.",
			BodyContentType:  "Text",
		}
	}
	id := messageID(validity, msg.Uid, p.MessageID)
	conv := p.ThreadRoot()
	if conv == "" {
		conv = id
	}
	out := p.ToMailMessage(id, conv, msg.InternalDate)
	// The body was read only in part; the structure knows about attachments
	// past the cut.
	out.HasAttachments = out.HasAttachments || hasAttachment(msg.BodyStructure)
	return out
}

func hasAttachment(bs *imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	if strings.EqualFold(bs.Disposition, "attachment") {
		return true
	}
	if _, ok := bs.DispositionParams["filename"]; ok {
		return true
	}
	for _, part := range bs.Parts {
		if hasAttachment(part) {
			return true
		}
	}
	return false
}

var errNotInInbox = errors.New("message not in inbox")

// findUID resolves a provider id to a UID in the selected INBOX.
func findUID(cl *client.Client, status *imap.MailboxStatus, id string) (uint32, error) {
	switch {
	case strings.HasPrefix(id, "uid:"):
		v, u, err := parseCursor(cursorPrefix + ":" + strings.TrimPrefix(id, "uid:"))
		if err != nil || v != status.UidValidity {
			return 0, errNotInInbox
		}
		return u, nil
	case strings.HasPrefix(id, "mid:"):
		mid := strings.TrimPrefix(id, "mid:")
		criteria := imap.NewSearchCriteria()
		criteria.Header.Add("Message-Id", mid)
		uids, err := cl.UidSearch(criteria)
		if err != nil {
			return 0, fmt.Errorf("imap search: %w", err)
		}
		if len(uids) == 0 {
			return 0, errNotInInbox
		}
		// The same Message-ID twice is one message delivered twice; the
		// latest copy is as good as any.
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		return uids[len(uids)-1], nil
	default:
		return 0, errNotInInbox
	}
}

func (m *mailbox) GetRawMessage(ctx context.Context, id string) ([]byte, error) {
	cl, status, err := m.session(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cl.Logout() }()
	uid, err := findUID(cl, status, id)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	set := new(imap.SeqSet)
	set.AddNum(uid)

	// Size first, so an oversized message is refused without reading it.
	sizes := make(chan *imap.Message, 1)
	if err := cl.UidFetch(set, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size}, sizes); err != nil {
		return nil, fmt.Errorf("imap fetch size: %w", err)
	}
	var size uint32
	found := false
	for msg := range sizes {
		size, found = msg.Size, true
	}
	if !found {
		return nil, fmt.Errorf("%s: %w", id, errNotInInbox)
	}
	if int64(size) > m.p.maxRaw() {
		return nil, driven.TooLarge(m.p.maxRaw())
	}

	section := &imap.BodySectionName{Peek: true}
	bodies := make(chan *imap.Message, 1)
	if err := cl.UidFetch(set, []imap.FetchItem{imap.FetchUid, section.FetchItem()}, bodies); err != nil {
		return nil, fmt.Errorf("imap fetch: %w", err)
	}
	var raw []byte
	for msg := range bodies {
		if body := msg.GetBody(section); body != nil {
			if raw, err = io.ReadAll(io.LimitReader(body, m.p.maxRaw()+1)); err != nil {
				return nil, err
			}
		}
	}
	if raw == nil {
		return nil, fmt.Errorf("%s: %w", id, errNotInInbox)
	}
	if int64(len(raw)) > m.p.maxRaw() {
		return nil, driven.TooLarge(m.p.maxRaw())
	}
	return raw, nil
}

func recipients(raw []byte) ([]string, error) {
	p, err := mailmime.Parse(raw)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range append(append([]driven.MailRecipient{}, p.To...), p.Cc...) {
		out = append(out, r.Address)
	}
	return out, nil
}

func (m *mailbox) Reply(ctx context.Context, id, body string) error {
	original, err := m.GetRawMessage(ctx, id)
	if err != nil {
		return notSent(err)
	}
	reply, err := mailmime.BuildReply(m.email, original, body, time.Now())
	if err != nil {
		return notSent(err)
	}
	to, err := recipients(reply)
	if err != nil {
		return notSent(err)
	}
	return m.p.send(ctx, m.cred, m.email, to, reply)
}

// Forward re-sends the original as an attachment; IMAP has no server-side
// forward. Everything up to the send is not-sent, so a rule can retry it.
func (m *mailbox) Forward(ctx context.Context, id, to, comment string) error {
	original, err := m.GetRawMessage(ctx, id)
	if err != nil {
		return notSent(err)
	}
	to = strings.TrimSpace(to)
	fwd, err := mailmime.BuildForward(m.email, to, comment, original, time.Now())
	if err != nil {
		return notSent(err)
	}
	return m.p.send(ctx, m.cred, m.email, []string{to}, fwd)
}
