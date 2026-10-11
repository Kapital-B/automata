package auth

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// GoogleOAuth is Google Identity OAuth2 (sign-in) for linking users.
type GoogleOAuth interface {
	AuthorizationURL(ctx context.Context, state string) (string, error)
	ExchangeCode(ctx context.Context, code string) (GoogleTokenResult, error)
}

// GoogleTokenResult holds subject and email from Google sign-in.
type GoogleTokenResult struct {
	ProviderSubject string // id_token "sub"
	Email           string
}

// OAuthStateRepository stores one-time OAuth state values (mail connect + auth flows).
type OAuthStateRepository interface {
	InsertOAuthState(ctx context.Context, state, flow string, userID *uuid.UUID, payloadJSON string, createdAt time.Time) error
	TakeOAuthState(ctx context.Context, state string) (flow string, userID *uuid.UUID, payloadJSON string, ok bool, err error)
	DeleteExpiredStates(ctx context.Context, before time.Time) error
}

// UserRepository persists app users and external identities.
type UserRepository interface {
	CreateUser(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time) error
	// CreateUserWithHomeOrg inserts the user, a Personal organisation, owner membership,
	// and optional password/oauth identity in one transaction. identityProvider empty skips identity.
	CreateUserWithHomeOrg(ctx context.Context, id uuid.UUID, email string, passwordHash *string, now time.Time, identityProvider, identitySubject, identityEmail string) (orgID uuid.UUID, err error)
	GetUserByEmail(ctx context.Context, email string) (id uuid.UUID, passwordHash *string, err error)
	GetUserByID(ctx context.Context, id uuid.UUID) (email string, err error)
	GetHomeOrganisationID(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	FindIdentity(ctx context.Context, provider, providerSubject string) (userID uuid.UUID, ok bool, err error)
	AttachIdentity(ctx context.Context, id uuid.UUID, userID uuid.UUID, provider, providerSubject, emailAtLink string, now time.Time) error
}

// AuthSessionRepository stores opaque refresh tokens (SHA-256 hash at rest).
type AuthSessionRepository interface {
	InsertAuthSession(ctx context.Context, sessionID, userID uuid.UUID, tokenHash string, createdAt, expiresAt time.Time) error
	// ConsumeAuthSession deletes the row if token_hash matches and expires_at is in the future; returns user id.
	ConsumeAuthSession(ctx context.Context, tokenHash string) (userID uuid.UUID, ok bool, err error)
}
