package auth

import (
	"context"
	"crypto/rand"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"

	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	"github.com/google/uuid"
)

const (
	flowAuthMicrosoft = "auth_microsoft"
	flowAuthGoogle    = "auth_google"
)

// Service handles registration, password login, and Microsoft/Google sign-in with email linking.
type Service struct {
	Users      domainauth.UserRepository
	Sessions   domainauth.AuthSessionRepository
	OAuthState domainauth.OAuthStateRepository
	MSAuth     domainauth.IdentityProvider
	Google     domainauth.GoogleOAuth
	Tokens     domainauth.AccessTokenIssuer
	Passwords  domainauth.PasswordHasher
	RefreshTTL time.Duration
}

// NewService constructs the auth service.
func NewService(users domainauth.UserRepository, sessions domainauth.AuthSessionRepository, states domainauth.OAuthStateRepository, msAuth domainauth.IdentityProvider, google domainauth.GoogleOAuth, tokens domainauth.AccessTokenIssuer, passwords domainauth.PasswordHasher, refreshTTL time.Duration) *Service {
	return &Service{
		Users:      users,
		Sessions:   sessions,
		OAuthState: states,
		MSAuth:     msAuth,
		Google:     google,
		Tokens:     tokens,
		Passwords:  passwords,
		RefreshTTL: refreshTTL,
	}
}

var ErrEmailTaken = domainauth.AuthErrEmailTaken
var ErrInvalidCredentials = domainauth.AuthErrInvalidCredentials
var ErrInvalidOAuthState = domainauth.AuthErrInvalidOAuthState
var ErrInvalidRefreshToken = domainauth.AuthErrInvalidRefreshToken

// IssueTokens creates a new access + refresh pair for an already-authenticated user (e.g. after register).
func (s *Service) IssueTokens(ctx context.Context, userID uuid.UUID) (TokenPair, error) {
	return s.issueTokenPair(ctx, userID)
}

func (s *Service) issueTokenPair(ctx context.Context, userID uuid.UUID) (TokenPair, error) {
	access, err := s.Tokens.Issue(userID)
	if err != nil {
		return TokenPair{}, err
	}
	rawRefresh, err := NewRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	hash := HashRefreshToken(rawRefresh)
	sid := uuid.New()
	now := time.Now().UTC()
	exp := now.Add(s.RefreshTTL)
	if err := s.Sessions.InsertAuthSession(ctx, sid, userID, hash, now, exp); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: rawRefresh}, nil
}

// RefreshTokens consumes a refresh token and returns a new pair (rotation).
func (s *Service) RefreshTokens(ctx context.Context, rawRefresh string) (TokenPair, error) {
	rawRefresh = strings.TrimSpace(rawRefresh)
	if rawRefresh == "" {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	hash := HashRefreshToken(rawRefresh)
	userID, ok, err := s.Sessions.ConsumeAuthSession(ctx, hash)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok {
		return TokenPair{}, ErrInvalidRefreshToken
	}
	return s.issueTokenPair(ctx, userID)
}

func (s *Service) Register(ctx context.Context, email, password string) (uuid.UUID, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 8 {
		return uuid.Nil, fmt.Errorf("invalid email or password")
	}
	hash, err := s.Passwords.Hash(password)
	if err != nil {
		return uuid.Nil, err
	}
	if _, _, err := s.Users.GetUserByEmail(ctx, email); err == nil {
		return uuid.Nil, ErrEmailTaken
	} else if !errors.Is(err, domainshared.ErrNotFound) {
		return uuid.Nil, err
	}
	id := uuid.New()
	now := time.Now().UTC()
	h := hash
	if _, err := s.Users.CreateUserWithHomeOrg(ctx, id, email, &h, now, "password", id.String(), email); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *Service) LoginPassword(ctx context.Context, email, password string) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	id, ph, err := s.Users.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domainshared.ErrNotFound) {
			return TokenPair{}, ErrInvalidCredentials
		}
		return TokenPair{}, err
	}
	if ph == nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	if s.Passwords.Verify(*ph, password) != nil {
		return TokenPair{}, ErrInvalidCredentials
	}
	return s.issueTokenPair(ctx, id)
}

func (s *Service) StartMicrosoftLogin(ctx context.Context) (authURL, state string, err error) {
	if s.MSAuth == nil {
		return "", "", fmt.Errorf("microsoft auth not configured")
	}
	st, err := randomState()
	if err != nil {
		return "", "", err
	}
	if err := s.OAuthState.InsertOAuthState(ctx, st, flowAuthMicrosoft, nil, "{}", time.Now().UTC()); err != nil {
		return "", "", err
	}
	u, err := s.MSAuth.AuthorizationURL(ctx, st)
	if err != nil {
		return "", "", err
	}
	return u, st, nil
}

func (s *Service) CompleteMicrosoftLogin(ctx context.Context, code, state string) (TokenPair, error) {
	flow, _, _, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok || flow != flowAuthMicrosoft {
		return TokenPair{}, ErrInvalidOAuthState
	}
	identity, err := s.MSAuth.ExchangeIdentity(ctx, code)
	if err != nil {
		return TokenPair{}, err
	}
	return s.finishExternalLogin(ctx, "microsoft", identity.Subject, identity.Email)
}

func (s *Service) StartGoogleLogin(ctx context.Context) (authURL, state string, err error) {
	if s.Google == nil {
		return "", "", fmt.Errorf("google auth not configured")
	}
	st, err := randomState()
	if err != nil {
		return "", "", err
	}
	if err := s.OAuthState.InsertOAuthState(ctx, st, flowAuthGoogle, nil, "{}", time.Now().UTC()); err != nil {
		return "", "", err
	}
	u, err := s.Google.AuthorizationURL(ctx, st)
	if err != nil {
		return "", "", err
	}
	return u, st, nil
}

func (s *Service) CompleteGoogleLogin(ctx context.Context, code, state string) (TokenPair, error) {
	flow, _, _, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return TokenPair{}, err
	}
	if !ok || flow != flowAuthGoogle {
		return TokenPair{}, ErrInvalidOAuthState
	}
	res, err := s.Google.ExchangeCode(ctx, code)
	if err != nil {
		return TokenPair{}, err
	}
	return s.finishExternalLogin(ctx, "google", res.ProviderSubject, res.Email)
}

func (s *Service) finishExternalLogin(ctx context.Context, provider, subject, email string) (TokenPair, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return TokenPair{}, fmt.Errorf("identity provider did not return email")
	}
	now := time.Now().UTC()
	if uid, ok, err := s.Users.FindIdentity(ctx, provider, subject); err == nil && ok {
		return s.issueTokenPair(ctx, uid)
	} else if err != nil {
		return TokenPair{}, err
	}
	existingID, _, err := s.Users.GetUserByEmail(ctx, email)
	if err == nil {
		if err := s.Users.AttachIdentity(ctx, uuid.New(), existingID, provider, subject, email, now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return s.issueTokenPair(ctx, existingID)
			}
			return TokenPair{}, err
		}
		return s.issueTokenPair(ctx, existingID)
	}
	if !errors.Is(err, domainshared.ErrNotFound) {
		return TokenPair{}, err
	}
	id := uuid.New()
	if _, err := s.Users.CreateUserWithHomeOrg(ctx, id, email, nil, now, provider, subject, email); err != nil {
		return TokenPair{}, err
	}
	return s.issueTokenPair(ctx, id)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
