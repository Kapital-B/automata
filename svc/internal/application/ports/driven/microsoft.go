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

// GraphProfile is returned from Graph /me.
type GraphProfile struct {
	Mail              string
	UserPrincipalName string
	TenantID          string // oid of tenant for work; consumers may use placeholder
}

// GraphMessage is a subset of Graph message JSON.
type GraphMessage struct {
	ID               string
	ConversationID   string
	ReceivedDateTime string
	Subject          string
	FromName         string
	FromAddress      string
	BodyPreview      string
	BodyContent      string
	BodyContentType  string // Text or HTML
	HasAttachments   bool
	ChangeKey        string
}

type GraphDeltaResult struct {
	Messages  []GraphMessage
	DeltaLink string
}

// MicrosoftGraph reads mailbox data.
type MicrosoftGraph interface {
	GetMe(ctx context.Context, accessToken string) (*GraphProfile, error)
	ListInboxMessages(ctx context.Context, accessToken string, top int) ([]GraphMessage, error)
	ListInboxDelta(ctx context.Context, accessToken string, deltaLink string, pageSize int) (*GraphDeltaResult, error)
	GetMessageBody(ctx context.Context, accessToken string, providerMessageID string) (*GraphMessage, error)
	SendMail(ctx context.Context, accessToken string, toEmail, subject, body string) error
	ReplyToMessage(ctx context.Context, accessToken string, providerMessageID string, body string) error
	// ForwardMessage forwards an existing mailbox message by Graph message id (server preserves body and attachments).
	// comment is optional introductory text above the forwarded content; use empty string for none.
	ForwardMessage(ctx context.Context, accessToken string, providerMessageID string, toEmail string, comment string) error
}

// TokenVault encrypts refresh tokens at rest.
type TokenVault interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
