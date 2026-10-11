package auth

import (
	"context"
	"errors"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

type AccessTokenIssuer interface {
	Issue(userID uuid.UUID) (string, error)
}
type AccessTokenVerifier interface {
	Verify(token string) (uuid.UUID, error)
}
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash, password string) error
}
type ExternalIdentity struct {
	Subject string
	Email   string
}
type IdentityProvider interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeIdentity(ctx context.Context, code string) (ExternalIdentity, error)
}
