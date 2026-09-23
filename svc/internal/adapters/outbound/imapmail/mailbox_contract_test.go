package imapmail

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/backend"
	"github.com/emersion/go-imap/backend/memory"
	imapserver "github.com/emersion/go-imap/server"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/google/uuid"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailboxtest"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/mailmime"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const (
	testUser     = "owner@example.org"
	testPassword = "app-password"
)

// fakeServers is an IMAP server and an SMTP server sharing one mailbox, both
// real protocol implementations over TLS. The IMAP backend is ours rather than
// go-imap's memory backend so UIDVALIDITY can change and access is locked.
type fakeServers struct {
	mu       sync.Mutex
	validity uint32
	nextUID  uint32
	messages []*memory.Message
	sent     []mailboxtest.Sent

	imap, smtp driven.MailServer
	tls        *tls.Config
}

func newFakeServers(t *testing.T, smtpMaxBytes int64) *fakeServers {
	t.Helper()
	// httptest's certificate covers 127.0.0.1, which is all a test needs.
	h := httptest.NewTLSServer(nil)
	cert := h.TLS.Certificates[0]
	pool := x509.NewCertPool()
	pool.AddCert(h.Certificate())
	h.Close()
	serverTLS := &tls.Config{Certificates: []tls.Certificate{cert}}

	f := &fakeServers{validity: 7, nextUID: 1, tls: &tls.Config{RootCAs: pool}}

	is := imapserver.New(&imapBackend{f: f})
	is.ErrorLog = discardLogger{}
	il, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = is.Serve(il) }()
	t.Cleanup(func() { _ = is.Close() })
	f.imap = driven.MailServer{Host: "127.0.0.1", Port: il.Addr().(*net.TCPAddr).Port, Security: SecurityTLS}

	ss := smtp.NewServer(smtpBackend{f: f})
	ss.Domain = "localhost"
	ss.MaxMessageBytes = smtpMaxBytes
	ss.ErrorLog = discardLogger{}
	sl, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = ss.Serve(sl) }()
	t.Cleanup(func() { _ = ss.Close() })
	f.smtp = driven.MailServer{Host: "127.0.0.1", Port: sl.Addr().(*net.TCPAddr).Port, Security: SecurityTLS}
	return f
}

type discardLogger struct{}

func (discardLogger) Printf(string, ...interface{}) {}
func (discardLogger) Println(...interface{})        {}

func (f *fakeServers) deliver(raw []byte) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	uid := f.nextUID
	f.nextUID++
	f.messages = append(f.messages, &memory.Message{Uid: uid, Date: time.Now().Add(-time.Minute), Size: uint32(len(raw)), Body: raw})
	return uid
}

func (f *fakeServers) remove(uid uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, m := range f.messages {
		if m.Uid == uid {
			f.messages = append(f.messages[:i], f.messages[i+1:]...)
			return
		}
	}
}

func (f *fakeServers) sentMail() []mailboxtest.Sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mailboxtest.Sent(nil), f.sent...)
}

// --- IMAP backend ---

type imapBackend struct{ f *fakeServers }

func (b *imapBackend) Login(_ *imap.ConnInfo, username, password string) (backend.User, error) {
	if username != testUser || password != testPassword {
		return nil, errors.New("Invalid credentials")
	}
	return &imapUser{f: b.f}, nil
}

type imapUser struct{ f *fakeServers }

func (u *imapUser) Username() string { return testUser }
func (u *imapUser) ListMailboxes(bool) ([]backend.Mailbox, error) {
	return []backend.Mailbox{&imapMailbox{f: u.f}}, nil
}
func (u *imapUser) GetMailbox(name string) (backend.Mailbox, error) {
	if !strings.EqualFold(name, "INBOX") {
		return nil, backend.ErrNoSuchMailbox
	}
	return &imapMailbox{f: u.f}, nil
}
func (u *imapUser) CreateMailbox(string) error         { return errors.New("not supported") }
func (u *imapUser) DeleteMailbox(string) error         { return errors.New("not supported") }
func (u *imapUser) RenameMailbox(string, string) error { return errors.New("not supported") }
func (u *imapUser) Logout() error                      { return nil }

type imapMailbox struct{ f *fakeServers }

func (m *imapMailbox) Name() string { return "INBOX" }
func (m *imapMailbox) Info() (*imap.MailboxInfo, error) {
	return &imap.MailboxInfo{Delimiter: "/", Name: "INBOX"}, nil
}
func (m *imapMailbox) Status(items []imap.StatusItem) (*imap.MailboxStatus, error) {
	m.f.mu.Lock()
	defer m.f.mu.Unlock()
	s := imap.NewMailboxStatus("INBOX", items)
	s.PermanentFlags = []string{`\*`}
	for _, it := range items {
		switch it {
		case imap.StatusMessages:
			s.Messages = uint32(len(m.f.messages))
		case imap.StatusUidNext:
			s.UidNext = m.f.nextUID
		case imap.StatusUidValidity:
			s.UidValidity = m.f.validity
		}
	}
	return s, nil
}
func (m *imapMailbox) SetSubscribed(bool) error { return nil }
func (m *imapMailbox) Check() error             { return nil }
func (m *imapMailbox) ListMessages(uid bool, set *imap.SeqSet, items []imap.FetchItem, ch chan<- *imap.Message) error {
	defer close(ch)
	m.f.mu.Lock()
	var out []*imap.Message
	for i, msg := range m.f.messages {
		seq := uint32(i + 1)
		id := seq
		if uid {
			id = msg.Uid
		}
		if !set.Contains(id) {
			continue
		}
		if fm, err := msg.Fetch(seq, items); err == nil {
			out = append(out, fm)
		}
	}
	m.f.mu.Unlock()
	for _, fm := range out {
		ch <- fm
	}
	return nil
}
func (m *imapMailbox) SearchMessages(uid bool, c *imap.SearchCriteria) ([]uint32, error) {
	m.f.mu.Lock()
	defer m.f.mu.Unlock()
	c = m.resolveStar(c)
	var ids []uint32
	for i, msg := range m.f.messages {
		seq := uint32(i + 1)
		if ok, err := msg.Match(seq, c); err != nil || !ok {
			continue
		}
		if uid {
			ids = append(ids, msg.Uid)
		} else {
			ids = append(ids, seq)
		}
	}
	return ids, nil
}

// resolveStar makes "n:*" mean what RFC 3501 says: "*" is the highest UID in
// use, so the range always matches that message even when n is above it.
// Real servers do this and the adapter has to cope; go-imap's SeqSet treats
// "*" as unbounded instead.
func (m *imapMailbox) resolveStar(c *imap.SearchCriteria) *imap.SearchCriteria {
	if c.Uid == nil || len(m.f.messages) == 0 {
		return c
	}
	max := m.f.messages[len(m.f.messages)-1].Uid
	star := func(v uint32) uint32 {
		if v == 0 {
			return max
		}
		return v
	}
	resolved := new(imap.SeqSet)
	for _, r := range c.Uid.Set {
		lo, hi := star(r.Start), star(r.Stop)
		if lo > hi {
			lo, hi = hi, lo
		}
		resolved.AddRange(lo, hi)
	}
	copied := *c
	copied.Uid = resolved
	return &copied
}

func (m *imapMailbox) CreateMessage([]string, time.Time, imap.Literal) error {
	return errors.New("not supported")
}
func (m *imapMailbox) UpdateMessagesFlags(bool, *imap.SeqSet, imap.FlagsOp, []string) error {
	return errors.New("read-only")
}
func (m *imapMailbox) CopyMessages(bool, *imap.SeqSet, string) error { return errors.New("read-only") }
func (m *imapMailbox) Expunge() error                                { return errors.New("read-only") }

// --- SMTP backend ---

type smtpBackend struct{ f *fakeServers }

func (b smtpBackend) NewSession(*smtp.Conn) (smtp.Session, error) { return &smtpSession{f: b.f}, nil }

type smtpSession struct {
	f      *fakeServers
	authed bool
	to     []string
}

func (s *smtpSession) AuthMechanisms() []string { return []string{sasl.Plain} }
func (s *smtpSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		if username != testUser || password != testPassword {
			return &smtp.SMTPError{Code: 535, EnhancedCode: smtp.EnhancedCode{5, 7, 8}, Message: "Authentication credentials invalid"}
		}
		s.authed = true
		return nil
	}), nil
}
func (s *smtpSession) Mail(string, *smtp.MailOptions) error {
	if !s.authed {
		return smtp.ErrAuthRequired
	}
	return nil
}
func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.to = append(s.to, to)
	return nil
}
func (s *smtpSession) Data(r io.Reader) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	kind := "forward"
	if p, err := mailmime.Parse(raw); err == nil && strings.HasPrefix(p.Subject, "Re:") {
		kind = "reply"
	}
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	for _, to := range s.to {
		s.f.sent = append(s.f.sent, mailboxtest.Sent{Kind: kind, To: to, Raw: raw})
	}
	return nil
}
func (s *smtpSession) Reset()        { s.to = nil }
func (s *smtpSession) Logout() error { return nil }

// --- helpers ---

func (f *fakeServers) provider() *Provider {
	return &Provider{TLSConfig: f.tls, DialTimeout: 5 * time.Second, CommandTimeout: 10 * time.Second}
}

func (f *fakeServers) request(password string) driven.PasswordConnectRequest {
	return driven.PasswordConnectRequest{Email: testUser, Password: password, IMAP: f.imap, SMTP: f.smtp}
}

func (f *fakeServers) open(t *testing.T, p *Provider) driven.Mailbox {
	t.Helper()
	connected, err := p.Connect(context.Background(), f.request(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	box, rotated, err := p.Open(context.Background(), driven.AccountRow{ID: uuid.New(), PrimaryEmail: connected.Email}, connected.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if rotated != nil {
		t.Error("a password credential never rotates")
	}
	return box
}

func TestIMAPMailboxContract(t *testing.T) {
	mailboxtest.RunContractTests(t, func(t *testing.T) *mailboxtest.Harness {
		f := newFakeServers(t, 0)
		box := f.open(t, f.provider())
		uidOf := map[string]uint32{}
		return &mailboxtest.Harness{
			Box: box,
			Deliver: func(t *testing.T, raw []byte) string {
				uid := f.deliver(raw)
				p, err := mailmime.Parse(raw)
				if err != nil {
					t.Fatal(err)
				}
				id := messageID(f.validity, uid, p.MessageID)
				uidOf[id] = uid
				return id
			},
			Remove: func(t *testing.T, id string) { f.remove(uidOf[id]) },
			Sent:   func(t *testing.T) []mailboxtest.Sent { return f.sentMail() },
			ExpireCursors: func(t *testing.T) {
				f.mu.Lock()
				f.validity++
				f.mu.Unlock()
			},
		}
	})
}

// Mail without a Message-ID still needs an id that finds it again.
func TestMessageWithoutMessageIDFallsBackToUID(t *testing.T) {
	f := newFakeServers(t, 0)
	box := f.open(t, f.provider())
	raw := []byte("From: Pat <pat@example.com>\r\nTo: owner@example.org\r\nSubject: No id\r\nDate: Mon, 21 Sep 2026 10:00:00 +0000\r\n\r\nhello\r\n")
	uid := f.deliver(raw)
	page, err := box.ListChanges(context.Background(), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(page.Messages))
	}
	want := messageID(f.validity, uid, "")
	if page.Messages[0].ID != want {
		t.Fatalf("id = %q, want %q", page.Messages[0].ID, want)
	}
	got, err := box.GetRawMessage(context.Background(), want)
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("raw = %q, err = %v", got, err)
	}
}

// The RFC's R3 exit criterion: a wrong password fails at connect, saying so.
func TestConnectRejectsAWrongPasswordNamingTheCause(t *testing.T) {
	f := newFakeServers(t, 0)
	_, err := f.provider().Connect(context.Background(), f.request("wrong"))
	var rejected *driven.ConnectRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("err = %v, want a ConnectRejectedError", err)
	}
	if !strings.Contains(rejected.Reason, "refused the username or password") || !strings.Contains(rejected.Reason, "app password") {
		t.Errorf("reason = %q, want it to name the password and the app-password hint", rejected.Reason)
	}
}

func TestConnectNamesAnUnreachableServer(t *testing.T) {
	f := newFakeServers(t, 0)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	req := f.request(testPassword)
	req.SMTP.Port = dead
	_, err = f.provider().Connect(context.Background(), req)
	var rejected *driven.ConnectRejectedError
	if !errors.As(err, &rejected) || !strings.Contains(rejected.Reason, "SMTP server") {
		t.Fatalf("err = %v, want a rejection naming the SMTP server", err)
	}
}

func TestConnectRefusesUnencryptedServers(t *testing.T) {
	f := newFakeServers(t, 0)
	req := f.request(testPassword)
	req.IMAP.Security = "none"
	_, err := f.provider().Connect(context.Background(), req)
	if !errors.Is(err, driven.ErrConnectRejected) || !strings.Contains(err.Error(), "unencrypted") {
		t.Fatalf("err = %v, want a refusal of plaintext", err)
	}
}

func TestConnectStoresATaggedCredentialWithUsernameDefaultingToEmail(t *testing.T) {
	f := newFakeServers(t, 0)
	connected, err := f.provider().Connect(context.Background(), f.request(testPassword))
	if err != nil {
		t.Fatal(err)
	}
	var c credential
	if err := json.Unmarshal(connected.Credential, &c); err != nil {
		t.Fatal(err)
	}
	if c.Type != credentialType || c.Username != testUser || c.IMAP != f.imap || c.SMTP != f.smtp {
		t.Errorf("credential = %+v", c)
	}
	if connected.Email != testUser || connected.DefaultLabel != testUser {
		t.Errorf("connected = %+v", connected)
	}
}

// A password changed after connecting has to mark the account for
// reconnecting, not fail every sync as if the server were down.
func TestOpenReportsAChangedPasswordAsRejectedCredentials(t *testing.T) {
	f := newFakeServers(t, 0)
	blob, _ := json.Marshal(credential{Type: credentialType, Username: testUser, Password: "old", IMAP: f.imap, SMTP: f.smtp})
	_, _, err := f.provider().Open(context.Background(), driven.AccountRow{PrimaryEmail: testUser}, blob)
	if !errors.Is(err, driven.ErrCredentialsRejected) {
		t.Fatalf("err = %v, want ErrCredentialsRejected", err)
	}
}

func TestOpenRefusesAnotherProvidersCredential(t *testing.T) {
	f := newFakeServers(t, 0)
	blob, _ := json.Marshal(map[string]string{"type": "google_oauth", "refresh_token": "x"})
	if _, _, err := f.provider().Open(context.Background(), driven.AccountRow{}, blob); err == nil {
		t.Fatal("opened a google credential as imap")
	}
}

// A server-side size limit is a permanent failure, not a retry.
func TestForwardOverTheServersSizeLimitIsTooLarge(t *testing.T) {
	f := newFakeServers(t, 2048)
	box := f.open(t, f.provider())
	raw, err := mailmime.Build(mailmime.Message{
		From: "pat@example.com", To: []string{testUser}, Subject: "Big", Body: strings.Repeat("x", 4096),
		Date: time.Now(), MessageID: mailmime.NewMessageID("pat@example.com"),
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := mailmime.Parse(raw)
	f.deliver(raw)
	err = box.Forward(context.Background(), "mid:"+p.MessageID, "bills@example.net", "")
	if !errors.Is(err, driven.ErrMailTooLarge) || !errors.Is(err, driven.ErrMailNotSent) {
		t.Fatalf("err = %v, want too large (and so not sent)", err)
	}
	if sent := f.sentMail(); len(sent) != 0 {
		t.Errorf("sent %d messages over the limit", len(sent))
	}
}

func TestGetRawMessageRefusesOversizedMessages(t *testing.T) {
	f := newFakeServers(t, 0)
	p := f.provider()
	p.MaxRawMessageBytes = 512
	box := f.open(t, p)
	raw := []byte("From: pat@example.com\r\nSubject: Big\r\nMessage-ID: <big@example.com>\r\n\r\n" + strings.Repeat("x", 1024))
	f.deliver(raw)
	if _, err := box.GetRawMessage(context.Background(), "mid:big@example.com"); !errors.Is(err, driven.ErrMailTooLarge) {
		t.Fatalf("err = %v, want ErrMailTooLarge", err)
	}
}
