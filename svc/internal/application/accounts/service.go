package accounts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
)

// Service orchestrates account connection.
type Service struct {
	deps Deps
}

func NewService(d Deps) *Service {
	return &Service{deps: d}
}

type StartConnectInput struct {
	Provider      string
	MsAccountKind domainacc.MsAccountKind
	LabelHint     *string
}

type StartConnectOutput struct {
	AuthorizationURL string
	State            string
}

// mailFlow is the OAuth state flow for connecting a provider's mailbox. Each
// provider gets its own, so a callback can only complete the connect it
// started. Microsoft keeps the value it has always used.
func mailFlow(provider string) string {
	if provider == ProviderM365 {
		return "m365_mail"
	}
	return provider + "_mail"
}

// placeholderMsAccountKind fills ms_account_kind for non-Microsoft accounts.
// The column is NOT NULL under an unnamed CHECK over Microsoft's two values,
// and widening an unnamed CHECK is what broke the DSQL deploys on PR #9, so
// rather than migrate it the value is written and never read: nothing outside
// the Microsoft provider consults it (RFC multi-provider mail §5.1, option A).
const placeholderMsAccountKind = domainacc.KindWork

func connectOptions(provider string, kind domainacc.MsAccountKind) (driven.ConnectOptions, error) {
	if provider != ProviderM365 {
		return driven.ConnectOptions{}, nil
	}
	if !kind.Valid() || kind == domainacc.KindCommon {
		return driven.ConnectOptions{}, fmt.Errorf("invalid ms_account_kind")
	}
	return driven.ConnectOptions{MsAccountKind: kind}, nil
}

func (s *Service) StartConnect(ctx context.Context, userID uuid.UUID, in StartConnectInput) (*StartConnectOutput, error) {
	provider := ProviderKey(in.Provider)
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
	if err := s.deps.OAuthState.InsertOAuthState(ctx, st, mailFlow(provider), &userID, payload, time.Now().UTC()); err != nil {
		return nil, err
	}
	authURL, err := conn.AuthorizationURL(ctx, st, opts)
	if err != nil {
		return nil, err
	}
	return &StartConnectOutput{AuthorizationURL: authURL, State: st}, nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

var ErrInvalidOAuthState = errors.New("invalid oauth state")

type CompleteOAuthResult struct {
	AccountID uuid.UUID
}

func (s *Service) CompleteOAuth(ctx context.Context, code, state string) (*CompleteOAuthResult, error) {
	flow, stateUserID, payloadJSON, ok, err := s.deps.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return nil, err
	}
	if !ok || stateUserID == nil {
		return nil, ErrInvalidOAuthState
	}
	provider, kind, labelHint, err := DecodeMailboxOAuthPayload(payloadJSON)
	if err != nil || flow != mailFlow(provider) {
		return nil, ErrInvalidOAuthState
	}
	conn, ok := s.deps.Connectors[provider]
	if !ok {
		return nil, ErrInvalidOAuthState
	}
	connected, err := conn.Complete(ctx, code, driven.ConnectOptions{MsAccountKind: kind})
	if err != nil {
		return nil, err
	}
	label := ""
	if labelHint != nil {
		label = *labelHint
	}
	if label == "" {
		label = connected.DefaultLabel
	}
	cipher, err := s.deps.Vault.Encrypt(connected.Credential)
	if err != nil {
		return nil, err
	}
	rowKind := connected.MsAccountKind
	if provider != ProviderM365 {
		rowKind = placeholderMsAccountKind
	}
	id := uuid.New()
	now := time.Now().UTC()
	row := driven.AccountRow{
		UserID:           *stateUserID,
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
		return nil, err
	}
	return &CompleteOAuthResult{AccountID: id}, nil
}

func (s *Service) Disconnect(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.deps.Accounts.DeleteAccount(ctx, userID, id)
}
