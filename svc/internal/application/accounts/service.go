package accounts

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
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

func (s *Service) StartConnect(ctx context.Context, in StartConnectInput) (*StartConnectOutput, error) {
	if in.Provider != "" && strings.ToLower(in.Provider) != "m365" {
		return nil, fmt.Errorf("unsupported provider")
	}
	if !in.MsAccountKind.Valid() {
		return nil, fmt.Errorf("invalid ms_account_kind")
	}
	st, err := randomState()
	if err != nil {
		return nil, err
	}
	if err := s.deps.OAuthState.InsertState(ctx, st, in.MsAccountKind, in.LabelHint, time.Now().UTC()); err != nil {
		return nil, err
	}
	authURL, err := s.deps.OAuth.AuthorizationURL(ctx, in.MsAccountKind, st)
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
	kind, labelHint, ok, err := s.deps.OAuthState.TakeState(ctx, state)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidOAuthState
	}
	tok, err := s.deps.OAuth.ExchangeCode(ctx, kind, code)
	if err != nil {
		return nil, fmt.Errorf("exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return nil, fmt.Errorf("missing refresh_token")
	}
	prof, err := s.deps.Graph.GetMe(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("graph me: %w", err)
	}
	id := uuid.New()
	label := ""
	if labelHint != nil {
		label = *labelHint
	}
	if label == "" {
		label = prof.Mail
		if label == "" {
			label = prof.UserPrincipalName
		}
		if label == "" {
			label = "Microsoft"
		}
	}
	tenant := prof.TenantID
	payload, err := encodeRefreshPayload(kind, tok.RefreshToken)
	if err != nil {
		return nil, err
	}
	cipher, err := s.deps.Vault.Encrypt(payload)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	row := driven.AccountRow{
		ID:               id,
		Label:            label,
		Provider:         "m365",
		MsAccountKind:    kind,
		GraphTenantID:    &tenant,
		PrimaryEmail:     prof.Mail,
		ConnectionStatus: "connected",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if row.PrimaryEmail == "" {
		row.PrimaryEmail = prof.UserPrincipalName
	}
	if err := s.deps.Accounts.InsertAccount(ctx, row, cipher); err != nil {
		return nil, err
	}
	return &CompleteOAuthResult{AccountID: id}, nil
}

func (s *Service) Disconnect(ctx context.Context, id uuid.UUID) error {
	return s.deps.Accounts.DeleteAccount(ctx, id)
}
