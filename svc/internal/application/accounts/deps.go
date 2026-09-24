package accounts

import (
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// Deps groups dependencies for account application services.
type Deps struct {
	Accounts   driven.AccountRepository
	OAuthState driven.OAuthStateRepository
	JobRuns    driven.JobRunRepository
	// Connectors are the OAuth mailbox connect flows, keyed by provider.
	Connectors map[string]driven.OAuthMailConnector
	// PasswordConnectors connect mailboxes from typed-in credentials.
	PasswordConnectors map[string]driven.PasswordMailConnector
	// Mailboxes reports what each provider's mailboxes can do.
	Mailboxes   *MailboxOpener
	Vault       driven.TokenVault
	Dashboard   string
	SuccessPath string
	ErrorPath   string
	StateTTL    time.Duration
}
