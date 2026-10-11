package auth

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
)

var AuthErrInvalidRefreshToken = errors.New("invalid refresh token")

var AuthErrEmailTaken = errors.New("email already registered")

// AuthTokenPair is issued after successful authentication.
type AuthTokenPair struct {
	AccessToken  string
	RefreshToken string
}

var AuthErrInvalidCredentials = errors.New("invalid email or password")

var AuthErrInvalidOAuthState = errors.New("invalid oauth state")

type AuthUseCases interface {
	IssueTokens(ctx context.Context, userID uuid.UUID) (AuthTokenPair, error)
	RefreshTokens(ctx context.Context, rawRefresh string) (AuthTokenPair, error)
	Register(ctx context.Context, email, password string) (uuid.UUID, error)
	LoginPassword(ctx context.Context, email, password string) (AuthTokenPair, error)
	StartMicrosoftLogin(ctx context.Context) (authURL, state string, err error)
	CompleteMicrosoftLogin(ctx context.Context, code, state string) (AuthTokenPair, error)
	StartGoogleLogin(ctx context.Context) (authURL, state string, err error)
	CompleteGoogleLogin(ctx context.Context, code, state string) (AuthTokenPair, error)
}
