// Package imapmail connects any mailbox that speaks IMAP for reading and SMTP
// for sending: Fastmail, Zoho, iCloud, hosted cPanel mail, on-premises
// Exchange with IMAP enabled, and personal Gmail through an app password.
package imapmail

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-imap/commands"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

const (
	// DefaultMaxRawMessageBytes bounds raw fetches and re-sent forwards, the
	// same ceiling as the other providers.
	DefaultMaxRawMessageBytes int64 = 25 << 20

	defaultDialTimeout    = 15 * time.Second
	defaultCommandTimeout = 60 * time.Second

	inbox = "INBOX"

	credentialType = "imap_password"

	SecurityTLS      = "tls"
	SecurityStartTLS = "starttls"
)

// Provider opens and connects IMAP/SMTP mailboxes. It keeps no connections:
// each operation dials, logs in, works and logs out, which suits a worker
// that runs a chunk and exits.
type Provider struct {
	// TLSConfig is cloned per connection with ServerName set to the host.
	// Nil means the system roots; tests supply their own.
	TLSConfig      *tls.Config
	DialTimeout    time.Duration
	CommandTimeout time.Duration
	// MaxRawMessageBytes bounds raw fetches; zero means the default.
	MaxRawMessageBytes int64
}

var (
	_ driven.MailProvider          = (*Provider)(nil)
	_ driven.PasswordMailConnector = (*Provider)(nil)
)

// credential is the stored form: both transports and the secret, tagged so
// it cannot be mistaken for another provider's blob. The endpoints ride in
// the encrypted envelope rather than a column, since the accounts table has
// nowhere else to put them and they are only ever read alongside the secret.
type credential struct {
	Type     string            `json:"type"`
	Username string            `json:"username"`
	Password string            `json:"password"`
	IMAP     driven.MailServer `json:"imap"`
	SMTP     driven.MailServer `json:"smtp"`
}

func (p *Provider) maxRaw() int64 {
	if p.MaxRawMessageBytes > 0 {
		return p.MaxRawMessageBytes
	}
	return DefaultMaxRawMessageBytes
}

func (p *Provider) dialTimeout() time.Duration {
	if p.DialTimeout > 0 {
		return p.DialTimeout
	}
	return defaultDialTimeout
}

func (p *Provider) commandTimeout() time.Duration {
	if p.CommandTimeout > 0 {
		return p.CommandTimeout
	}
	return defaultCommandTimeout
}

func (p *Provider) tlsConfig(host string) *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if p.TLSConfig != nil {
		cfg = p.TLSConfig.Clone()
	}
	cfg.ServerName = host
	return cfg
}

func addr(s driven.MailServer) string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

// validServer refuses anything but an encrypted transport: the password is a
// long-lived secret and never crosses the wire in the clear.
func validServer(kind string, s driven.MailServer) error {
	if strings.TrimSpace(s.Host) == "" {
		return fmt.Errorf("%s host is required", kind)
	}
	if s.Port <= 0 || s.Port > 65535 {
		return fmt.Errorf("%s port %d is not a valid port", kind, s.Port)
	}
	switch s.Security {
	case SecurityTLS, SecurityStartTLS:
		return nil
	default:
		return fmt.Errorf("%s security must be %q or %q; unencrypted connections are not supported", kind, SecurityTLS, SecurityStartTLS)
	}
}

// dial opens a TCP connection, wrapped in TLS for implicit-TLS servers.
func (p *Provider) dial(ctx context.Context, s driven.MailServer) (net.Conn, error) {
	d := net.Dialer{Timeout: p.dialTimeout()}
	conn, err := d.DialContext(ctx, "tcp", addr(s))
	if err != nil {
		return nil, err
	}
	if s.Security != SecurityTLS {
		return conn, nil
	}
	tc := tls.Client(conn, p.tlsConfig(s.Host))
	hctx, cancel := context.WithTimeout(ctx, p.dialTimeout())
	defer cancel()
	if err := tc.HandshakeContext(hctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tc, nil
}

// errLoginRefused is the server answering LOGIN with NO or BAD: the
// credentials are wrong, as opposed to the server being unreachable.
var errLoginRefused = errors.New("login refused")

// imapSession dials, secures and logs in, leaving INBOX selected read-only.
func (p *Provider) imapSession(ctx context.Context, c credential) (*client.Client, *imap.MailboxStatus, error) {
	conn, err := p.dial(ctx, c.IMAP)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to %s: %w", addr(c.IMAP), err)
	}
	cl, err := client.New(conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("imap greeting from %s: %w", addr(c.IMAP), err)
	}
	cl.Timeout = p.commandTimeout()
	// The client logs every read error to stderr, including the expected one
	// when a failed login is torn down; the error returned is what matters.
	cl.ErrorLog = log.New(io.Discard, "", 0)
	fail := func(err error) (*client.Client, *imap.MailboxStatus, error) {
		_ = cl.Terminate()
		return nil, nil, err
	}
	if c.IMAP.Security == SecurityStartTLS {
		ok, err := cl.SupportStartTLS()
		if err != nil {
			return fail(fmt.Errorf("imap capability: %w", err))
		}
		if !ok {
			return fail(fmt.Errorf("%s does not offer STARTTLS", addr(c.IMAP)))
		}
		if err := cl.StartTLS(p.tlsConfig(c.IMAP.Host)); err != nil {
			return fail(fmt.Errorf("imap starttls: %w", err))
		}
	}
	// Execute rather than Login: Login folds "the server said no" and "the
	// connection dropped" into one error, and only the first means the
	// credentials are wrong.
	status, err := cl.Execute(&commands.Login{Username: c.Username, Password: c.Password}, nil)
	if err != nil {
		return fail(fmt.Errorf("imap login: %w", err))
	}
	if status.Type != imap.StatusRespOk {
		if status.Code == "UNAVAILABLE" {
			return fail(fmt.Errorf("imap login: server unavailable: %s", status.Info))
		}
		return fail(fmt.Errorf("%w: %s", errLoginRefused, status.Info))
	}
	cl.SetState(imap.AuthenticatedState, nil)
	mbox, err := cl.Select(inbox, true)
	if err != nil {
		return fail(fmt.Errorf("open %s: %w", inbox, err))
	}
	return cl, mbox, nil
}

func decodeCredential(raw []byte) (credential, error) {
	var c credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	if c.Type != credentialType || c.Password == "" {
		return c, fmt.Errorf("not an imap credential")
	}
	return c, nil
}

// Open checks the stored password still works, so a changed or revoked one
// marks the account for reconnecting instead of failing every sync.
func (p *Provider) Open(ctx context.Context, account driven.AccountRow, raw []byte) (driven.Mailbox, []byte, error) {
	c, err := decodeCredential(raw)
	if err != nil {
		return nil, nil, err
	}
	cl, _, err := p.imapSession(ctx, c)
	if err != nil {
		if errors.Is(err, errLoginRefused) {
			return nil, nil, fmt.Errorf("%w: %v", driven.ErrCredentialsRejected, err)
		}
		return nil, nil, err
	}
	_ = cl.Logout()
	return &mailbox{p: p, cred: c, email: account.PrimaryEmail}, nil, nil
}

// Connect verifies both transports before anything is stored. Every failure
// the user can fix comes back as a ConnectRejectedError naming the cause.
func (p *Provider) Connect(ctx context.Context, req driven.PasswordConnectRequest) (*driven.ConnectedMailbox, error) {
	email := strings.TrimSpace(req.Email)
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return nil, driven.ConnectRejected("%q is not an email address", req.Email)
	}
	if req.Password == "" {
		return nil, driven.ConnectRejected("a password is required")
	}
	for _, s := range []struct {
		kind string
		srv  driven.MailServer
	}{{"IMAP", req.IMAP}, {"SMTP", req.SMTP}} {
		if err := validServer(s.kind, s.srv); err != nil {
			return nil, driven.ConnectRejected("%s", err.Error())
		}
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		username = email
	}
	c := credential{
		Type:     credentialType,
		Username: username,
		Password: req.Password,
		IMAP:     normalise(req.IMAP),
		SMTP:     normalise(req.SMTP),
	}

	cl, _, err := p.imapSession(ctx, c)
	if err != nil {
		if errors.Is(err, errLoginRefused) {
			return nil, driven.ConnectRejected("the IMAP server at %s refused the username or password (%s). Providers that use two-factor sign-in, such as Gmail and iCloud, need an app password here", c.IMAP.Host, strings.TrimPrefix(err.Error(), errLoginRefused.Error()+": "))
		}
		return nil, driven.ConnectRejected("could not use the IMAP server at %s: %v", addr(c.IMAP), err)
	}
	_ = cl.Logout()

	sc, err := p.smtpSession(ctx, c)
	if err != nil {
		if errors.Is(err, errLoginRefused) {
			return nil, driven.ConnectRejected("the SMTP server at %s refused the username or password: %v", c.SMTP.Host, err)
		}
		return nil, driven.ConnectRejected("could not use the SMTP server at %s: %v", addr(c.SMTP), err)
	}
	_ = sc.Quit()

	blob, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return &driven.ConnectedMailbox{Email: email, DefaultLabel: email, Credential: blob}, nil
}

func normalise(s driven.MailServer) driven.MailServer {
	s.Host = strings.ToLower(strings.TrimSpace(s.Host))
	return s
}
