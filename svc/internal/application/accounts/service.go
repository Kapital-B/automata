package accounts

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"fmt"
	"strings"
	"time"

	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// Service orchestrates account connection.
type Service struct {
	deps Deps
}

func NewService(d Deps) *Service {
	return &Service{deps: d}
}

// mailboxConnectFlow is the OAuth state flow for connecting any provider's
// mailbox. The value predates provider support, which is why it says m365:
// oauth_states.flow carries an unnamed CHECK over a fixed list, and widening
// an unnamed CHECK is what broke the DSQL deploys on PR #9. The provider rides
// in the state's payload instead. That is no weaker — the state is an
// unguessable key to a row we wrote, so the provider in it is ours, not the
// client's — and the separation that matters, sign-in versus mailbox connect,
// is still enforced by the flow.
const mailboxConnectFlow = "m365_mail"

func connectOptions(provider string, kind domainacc.MsAccountKind) (domainaccounts.ConnectOptions, error) {
	if provider != domainaccounts.AccountProviderM365 {
		return domainaccounts.ConnectOptions{}, nil
	}
	if !kind.Valid() || kind == domainacc.KindCommon {
		return domainaccounts.ConnectOptions{}, fmt.Errorf("invalid ms_account_kind")
	}
	return domainaccounts.ConnectOptions{MsAccountKind: kind}, nil
}

func (s *Service) StartConnect(ctx context.Context, userID uuid.UUID, in domainacc.AccountStartConnectInput) (*domainacc.AccountStartConnectOutput, error) {
	provider := domainaccounts.AccountProviderKey(in.Provider)
	conn, ok := s.deps.Connectors[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider")
	}
	opts, err := connectOptions(provider, in.MsAccountKind)
	if err != nil {
		return nil, err
	}
	st, err := randomState()
	if err != nil {
		return nil, err
	}
	payload, err := EncodeMailboxOAuthPayload(provider, opts.MsAccountKind, in.LabelHint)
	if err != nil {
		return nil, err
	}
	if err := s.deps.OAuthState.InsertOAuthState(ctx, st, mailboxConnectFlow, &userID, payload, time.Now().UTC()); err != nil {
		return nil, err
	}
	authURL, err := conn.AuthorizationURL(ctx, st, opts)
	if err != nil {
		return nil, err
	}
	return &domainacc.AccountStartConnectOutput{AuthorizationURL: authURL, State: st}, nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (*domainaccounts.AccountCompleteOAuthResult, error) {
	flow, stateUserID, payloadJSON, ok, err := s.deps.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return nil, err
	}
	if !ok || stateUserID == nil || flow != mailboxConnectFlow {
		return nil, domainacc.AccountErrInvalidOAuthState
	}
	provider, kind, labelHint, err := DecodeMailboxOAuthPayload(payloadJSON)
	if err != nil {
		return nil, domainacc.AccountErrInvalidOAuthState
	}
	conn, ok := s.deps.Connectors[provider]
	if !ok {
		return nil, domainaccounts.AccountErrInvalidOAuthState
	}
	connected, err := conn.Complete(ctx, code, domainaccounts.ConnectOptions{MsAccountKind: kind})
	if err != nil {
		return nil, err
	}
	id, err := s.saveConnected(ctx, *stateUserID, provider, connected, labelHint)
	if err != nil {
		return nil, err
	}
	return &domainacc.AccountCompleteOAuthResult{AccountID: id}, nil
}

// ConnectWithPassword verifies typed-in credentials against the provider and
// stores the mailbox. A failure the user can fix wraps
// domainaccounts.ErrConnectRejected and nothing is stored.
func (s *Service) ConnectWithPassword(ctx context.Context, userID uuid.UUID, in domainaccounts.AccountConnectWithPasswordInput) (uuid.UUID, error) {
	conn, ok := s.deps.PasswordConnectors[in.Provider]
	if !ok {
		return uuid.Nil, fmt.Errorf("%w: %s", domainaccounts.ErrUnsupportedProvider, in.Provider)
	}
	connected, err := conn.Connect(ctx, in.Request)
	if err != nil {
		return uuid.Nil, err
	}
	return s.saveConnected(ctx, userID, in.Provider, connected, in.LabelHint)
}

// saveConnected stores a verified mailbox, or refreshes the credentials of
// the matching account when this is a reconnect.
func (s *Service) saveConnected(ctx context.Context, userID uuid.UUID, provider string, connected *domainaccounts.ConnectedMailbox, labelHint *string) (uuid.UUID, error) {
	label := ""
	if labelHint != nil {
		label = strings.TrimSpace(*labelHint)
	}
	if label == "" {
		label = connected.DefaultLabel
	}
	cipher, err := s.deps.Vault.Encrypt(connected.Credential)
	if err != nil {
		return uuid.Nil, err
	}
	// Reconnecting a mailbox that is already here — after its credentials
	// expired, say — refreshes that account rather than adding a duplicate,
	// which would sync the same mail twice and orphan the original's rules
	// and history.
	if existing, err := s.findAccount(ctx, userID, provider, connected.Email); err != nil {
		return uuid.Nil, err
	} else if existing != nil {
		tenant := connected.TenantID
		if tenant == nil {
			tenant = existing.GraphTenantID
		}
		if err := s.deps.Accounts.UpdateAccountTokens(ctx, userID, existing.ID, cipher, connected.Email, tenant, existing.MsalHomeAccountID, "connected", nil); err != nil {
			return uuid.Nil, err
		}
		return existing.ID, nil
	}
	rowKind := connected.MsAccountKind
	if provider != domainaccounts.AccountProviderM365 {
		rowKind = domainaccounts.KindWork
	}
	id := uuid.New()
	now := time.Now().UTC()
	row := domainaccounts.AccountRow{
		UserID:           userID,
		ID:               id,
		Label:            label,
		Provider:         provider,
		MsAccountKind:    rowKind,
		GraphTenantID:    connected.TenantID,
		PrimaryEmail:     connected.Email,
		ConnectionStatus: "connected",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.deps.Accounts.InsertAccount(ctx, row, cipher); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// findAccount returns the user's account for this provider and address.
func (s *Service) findAccount(ctx context.Context, userID uuid.UUID, provider, email string) (*domainaccounts.AccountRow, error) {
	if strings.TrimSpace(email) == "" {
		return nil, nil
	}
	rows, err := s.deps.Accounts.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if domainacc.AccountProviderKey(rows[i].Provider) == provider && strings.EqualFold(rows[i].PrimaryEmail, email) {
			return &rows[i], nil
		}
	}
	return nil, nil
}

func (s *Service) Disconnect(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.deps.Accounts.DeleteAccount(ctx, userID, id)
}

// Connect methods a provider can be added by.
const (
	ConnectOAuth    = "oauth"
	ConnectPassword = "password"
)

// Providers lists what can be connected, in a stable order, so the UI offers
// only what this deployment is configured for.
func (s *Service) Providers() []domainacc.AccountProviderInfo {
	var out []domainacc.AccountProviderInfo
	add := func(provider, connect string) {
		caps, _ := s.deps.Mailboxes.Capabilities(provider)
		out = append(out, domainacc.AccountProviderInfo{Provider: provider, Connect: connect, Capabilities: caps})
	}
	for _, p := range []string{domainaccounts.AccountProviderM365, ProviderGoogle, domainacc.AccountProviderIMAP} {
		if _, ok := s.deps.Connectors[p]; ok {
			add(p, ConnectOAuth)
		} else if _, ok := s.deps.PasswordConnectors[p]; ok {
			add(p, ConnectPassword)
		}
	}
	return out
}

// Capabilities reports what an account's provider can do; ok is false when
// the provider is not configured here.
func (s *Service) Capabilities(provider string) (domainaccounts.MailboxCapabilities, bool) {
	return s.deps.Mailboxes.Capabilities(provider)
}
