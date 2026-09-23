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

// mailboxConnectFlow is the OAuth state flow for connecting any provider's
// mailbox. The value predates provider support, which is why it says m365:
// oauth_states.flow carries an unnamed CHECK over a fixed list, and widening
// an unnamed CHECK is what broke the DSQL deploys on PR #9. The provider rides
// in the state's payload instead. That is no weaker — the state is an
// unguessable key to a row we wrote, so the provider in it is ours, not the
// client's — and the separation that matters, sign-in versus mailbox connect,
// is still enforced by the flow.
const mailboxConnectFlow = "m365_mail"

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
	if err := s.deps.OAuthState.InsertOAuthState(ctx, st, mailboxConnectFlow, &userID, payload, time.Now().UTC()); err != nil {
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
	if !ok || stateUserID == nil || flow != mailboxConnectFlow {
		return nil, ErrInvalidOAuthState
	}
	provider, kind, labelHint, err := DecodeMailboxOAuthPayload(payloadJSON)
	if err != nil {
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
	// Reconnecting a mailbox that is already here — after its credentials
	// expired, say — refreshes that account rather than adding a duplicate,
	// which would sync the same mail twice and orphan the original's rules
	// and history.
	if existing, err := s.findAccount(ctx, *stateUserID, provider, connected.Email); err != nil {
		return nil, err
	} else if existing != nil {
		tenant := connected.TenantID
		if tenant == nil {
			tenant = existing.GraphTenantID
		}
		if err := s.deps.Accounts.UpdateAccountTokens(ctx, *stateUserID, existing.ID, cipher, connected.Email, tenant, existing.MsalHomeAccountID, "connected", nil); err != nil {
			return nil, err
		}
		return &CompleteOAuthResult{AccountID: existing.ID}, nil
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

// findAccount returns the user's account for this provider and address.
func (s *Service) findAccount(ctx context.Context, userID uuid.UUID, provider, email string) (*driven.AccountRow, error) {
	if strings.TrimSpace(email) == "" {
		return nil, nil
	}
	rows, err := s.deps.Accounts.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if ProviderKey(rows[i].Provider) == provider && strings.EqualFold(rows[i].PrimaryEmail, email) {
			return &rows[i], nil
		}
	}
	return nil, nil
}

func (s *Service) Disconnect(ctx context.Context, userID uuid.UUID, id uuid.UUID) error {
	return s.deps.Accounts.DeleteAccount(ctx, userID, id)
}
