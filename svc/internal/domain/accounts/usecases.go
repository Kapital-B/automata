package accounts

import (
	context "context"
	errors "errors"
	uuid "github.com/google/uuid"
	strings "strings"
)

// AccountProviderIMAP is any mailbox reached over IMAP and SMTP with a password.
const AccountProviderIMAP = "imap"

var AccountErrInvalidOAuthState = errors.New("invalid oauth state")

// AccountConnectWithPasswordInput is a typed-in mailbox connection.
type AccountConnectWithPasswordInput struct {
	Provider  string
	Request   PasswordConnectRequest
	LabelHint *string
}

type AccountStartConnectOutput struct {
	AuthorizationURL string
	State            string
}

type AccountCompleteOAuthResult struct {
	AccountID uuid.UUID
}

type AccountStartConnectInput struct {
	Provider      string
	MsAccountKind MsAccountKind
	LabelHint     *string
}

// AccountProviderKey normalises a stored provider value.
func AccountProviderKey(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return AccountProviderM365
	}
	return p
}

// AccountProviderM365 is the provider value for Microsoft 365 mailboxes, and what
// every row written before provider support means.
const AccountProviderM365 = "m365"

// AccountProviderInfo is a mailbox provider a user can connect here.
type AccountProviderInfo struct {
	Provider     string
	Connect      string
	Capabilities MailboxCapabilities
}

type AccountUseCases interface {
	StartConnect(ctx context.Context, userID uuid.UUID, in AccountStartConnectInput) (*AccountStartConnectOutput, error)
	CompleteOAuth(ctx context.Context, code, state string) (*AccountCompleteOAuthResult, error)
	ConnectWithPassword(ctx context.Context, userID uuid.UUID, in AccountConnectWithPasswordInput) (uuid.UUID, error)
	Disconnect(ctx context.Context, userID uuid.UUID, id uuid.UUID) error
	Providers() []AccountProviderInfo
	Capabilities(provider string) (MailboxCapabilities, bool)
}
