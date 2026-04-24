package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	flowAuthMicrosoft = "auth_microsoft"
	flowAuthGoogle    = "auth_google"
)

// Service handles registration, password login, and Microsoft/Google sign-in with email linking.
type Service struct {
	Users      driven.UserRepository
	OAuthState driven.OAuthStateRepository
	MSAuth     driven.MicrosoftOAuth // configured with sign-in-only scopes + common authority
	Google     driven.GoogleOAuth
	JWTSecret  []byte
	JWTTTL     time.Duration
}

// NewService constructs the auth service.
func NewService(users driven.UserRepository, states driven.OAuthStateRepository, msAuth driven.MicrosoftOAuth, google driven.GoogleOAuth, jwtSecret []byte, ttl time.Duration) *Service {
	return &Service{
		Users:      users,
		OAuthState: states,
		MSAuth:     msAuth,
		Google:     google,
		JWTSecret:  jwtSecret,
		JWTTTL:     ttl,
	}
}

var ErrEmailTaken = errors.New("email already registered")
var ErrInvalidCredentials = errors.New("invalid email or password")
var ErrInvalidOAuthState = errors.New("invalid oauth state")

func (s *Service) Register(ctx context.Context, email, password string) (uuid.UUID, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || len(password) < 8 {
		return uuid.Nil, fmt.Errorf("invalid email or password")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return uuid.Nil, err
	}
	if _, _, err := s.Users.GetUserByEmail(ctx, email); err == nil {
		return uuid.Nil, ErrEmailTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, err
	}
	id := uuid.New()
	now := time.Now().UTC()
	h := string(hash)
	if err := s.Users.CreateUser(ctx, id, email, &h, now); err != nil {
		return uuid.Nil, err
	}
	if err := s.Users.AttachIdentity(ctx, uuid.New(), id, "password", id.String(), email, now); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func (s *Service) LoginPassword(ctx context.Context, email, password string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	id, ph, err := s.Users.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}
	if ph == nil {
		return "", ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(*ph), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}
	return security.SignJWT(s.JWTSecret, id, s.JWTTTL)
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
	u, err := s.MSAuth.AuthorizationURL(ctx, domainacc.KindCommon, st)
	if err != nil {
		return "", "", err
	}
	return u, st, nil
}

func (s *Service) CompleteMicrosoftLogin(ctx context.Context, code, state string) (token string, err error) {
	flow, _, _, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return "", err
	}
	if !ok || flow != flowAuthMicrosoft {
		return "", ErrInvalidOAuthState
	}
	tok, err := s.MSAuth.ExchangeCode(ctx, domainacc.KindCommon, code)
	if err != nil {
		return "", err
	}
	sub, email, err := parseMSAccessTokenClaims(tok.AccessToken)
	if err != nil {
		return "", err
	}
	return s.finishExternalLogin(ctx, "microsoft", sub, email)
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

func (s *Service) CompleteGoogleLogin(ctx context.Context, code, state string) (token string, err error) {
	flow, _, _, ok, err := s.OAuthState.TakeOAuthState(ctx, state)
	if err != nil {
		return "", err
	}
	if !ok || flow != flowAuthGoogle {
		return "", ErrInvalidOAuthState
	}
	res, err := s.Google.ExchangeCode(ctx, code)
	if err != nil {
		return "", err
	}
	return s.finishExternalLogin(ctx, "google", res.ProviderSubject, res.Email)
}

func (s *Service) finishExternalLogin(ctx context.Context, provider, subject, email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "", fmt.Errorf("identity provider did not return email")
	}
	now := time.Now().UTC()
	if uid, ok, err := s.Users.FindIdentity(ctx, provider, subject); err == nil && ok {
		return security.SignJWT(s.JWTSecret, uid, s.JWTTTL)
	} else if err != nil {
		return "", err
	}
	existingID, _, err := s.Users.GetUserByEmail(ctx, email)
	if err == nil {
		if err := s.Users.AttachIdentity(ctx, uuid.New(), existingID, provider, subject, email, now); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return security.SignJWT(s.JWTSecret, existingID, s.JWTTTL)
			}
			return "", err
		}
		return security.SignJWT(s.JWTSecret, existingID, s.JWTTTL)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id := uuid.New()
	if err := s.Users.CreateUser(ctx, id, email, nil, now); err != nil {
		return "", err
	}
	if err := s.Users.AttachIdentity(ctx, uuid.New(), id, provider, subject, email, now); err != nil {
		return "", err
	}
	return security.SignJWT(s.JWTSecret, id, s.JWTTTL)
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
