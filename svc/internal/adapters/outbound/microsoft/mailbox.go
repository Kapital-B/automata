package microsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

// DefaultMaxRawMessageBytes bounds GetRawMessage.
const DefaultMaxRawMessageBytes int64 = 25 << 20

// credential is the stored form of a Microsoft mailbox credential. The shape
// predates provider support — it carries no type tag — and is kept exactly so
// existing accounts decode without a data migration. The account's provider
// column is what says a blob is this shape.
type credential struct {
	RefreshToken string `json:"refresh_token"`
	Kind         string `json:"ms_account_kind"`
}

func encodeCredential(kind accounts.MsAccountKind, refresh string) ([]byte, error) {
	return json.Marshal(credential{RefreshToken: refresh, Kind: string(kind)})
}

func decodeCredential(raw []byte) (accounts.MsAccountKind, string, error) {
	var c credential
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", "", err
	}
	return accounts.MsAccountKind(c.Kind), c.RefreshToken, nil
}

// Provider opens and connects Microsoft 365 mailboxes through Graph.
type Provider struct {
	OAuth driven.MicrosoftOAuth
	Graph *GraphClient
	// MaxRawMessageBytes bounds GetRawMessage; zero means the default.
	MaxRawMessageBytes int64
}

var (
	_ driven.MailProvider       = (*Provider)(nil)
	_ driven.OAuthMailConnector = (*Provider)(nil)
)

func (p *Provider) maxRaw() int64 {
	if p.MaxRawMessageBytes > 0 {
		return p.MaxRawMessageBytes
	}
	return DefaultMaxRawMessageBytes
}

// Open refreshes the access token. The refreshed credential is always handed
// back for persisting, as the pre-provider code always rewrote it: that write
// is also what returns an account in an error state to connected.
func (p *Provider) Open(ctx context.Context, _ driven.AccountRow, raw []byte) (driven.Mailbox, []byte, error) {
	kind, refresh, err := decodeCredential(raw)
	if err != nil {
		return nil, nil, err
	}
	tok, err := p.OAuth.RefreshAccessToken(ctx, kind, refresh)
	if err != nil {
		return nil, nil, fmt.Errorf("refresh token: %w", err)
	}
	next := refresh
	if tok.RefreshToken != "" {
		next = tok.RefreshToken
	}
	rotated, err := encodeCredential(kind, next)
	if err != nil {
		return nil, nil, err
	}
	return &mailbox{graph: p.Graph, token: tok.AccessToken, maxRaw: p.maxRaw()}, rotated, nil
}

func (p *Provider) AuthorizationURL(ctx context.Context, state string, opts driven.ConnectOptions) (string, error) {
	return p.OAuth.AuthorizationURL(ctx, opts.MsAccountKind, state)
}

func (p *Provider) Complete(ctx context.Context, code string, opts driven.ConnectOptions) (*driven.ConnectedMailbox, error) {
	tok, err := p.OAuth.ExchangeCode(ctx, opts.MsAccountKind, code)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh_token")
	}
	prof, err := p.Graph.GetMe(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("graph me: %w", err)
	}
	cred, err := encodeCredential(opts.MsAccountKind, tok.RefreshToken)
	if err != nil {
		return nil, err
	}
	email := prof.Mail
	if email == "" {
		email = prof.UserPrincipalName
	}
	label := email
	if label == "" {
		label = "Microsoft"
	}
	tenant := prof.TenantID
	return &driven.ConnectedMailbox{
		Email:         email,
		DefaultLabel:  label,
		TenantID:      &tenant,
		MsAccountKind: opts.MsAccountKind,
		Credential:    cred,
	}, nil
}

// mailbox is an opened Graph mailbox.
type mailbox struct {
	graph  *GraphClient
	token  string
	maxRaw int64
}

var capabilities = driven.MailboxCapabilities{
	IncrementalSync:   true,
	ServerSideForward: true,
	ServerSideReply:   true,
	ReportsRemovals:   true,
	StableMessageIDs:  true,
}

func (p *Provider) Capabilities() driven.MailboxCapabilities { return capabilities }

func (m *mailbox) Capabilities() driven.MailboxCapabilities { return capabilities }

func (m *mailbox) ListChanges(ctx context.Context, cursor string, pageSize int) (*driven.MailChangePage, error) {
	return m.graph.ListInboxDelta(ctx, m.token, cursor, pageSize)
}

func (m *mailbox) GetRawMessage(ctx context.Context, providerMessageID string) ([]byte, error) {
	return m.graph.GetRawMessage(ctx, m.token, providerMessageID, m.maxRaw)
}

func (m *mailbox) Reply(ctx context.Context, providerMessageID, body string) error {
	return m.graph.ReplyToMessage(ctx, m.token, providerMessageID, body)
}

// Forward is server-side: the original and its attachments never leave
// Microsoft. Resolving the immutable id happens before anything is sent, so a
// failure there is reported as not sent and is safe to retry.
func (m *mailbox) Forward(ctx context.Context, providerMessageID, to, comment string) error {
	id, err := m.graph.ResolveGraphMessageID(ctx, m.token, providerMessageID)
	if err != nil {
		return fmt.Errorf("%w: resolve message id: %v", driven.ErrMailNotSent, err)
	}
	return m.graph.ForwardMessage(ctx, m.token, id, strings.TrimSpace(to), comment)
}
