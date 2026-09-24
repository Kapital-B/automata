package driven

import (
	"context"

	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

// TokenPair holds OAuth tokens from the token endpoint.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// MicrosoftOAuth exchanges authorization codes and refreshes tokens.
type MicrosoftOAuth interface {
	AuthorizationURL(ctx context.Context, kind accounts.MsAccountKind, state string) (string, error)
	ExchangeCode(ctx context.Context, kind accounts.MsAccountKind, code string) (TokenPair, error)
	RefreshAccessToken(ctx context.Context, kind accounts.MsAccountKind, refreshToken string) (TokenPair, error)
}

// TokenVault encrypts refresh tokens at rest.
type TokenVault interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
