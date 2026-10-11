package accounts

import (
	"time"

	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
)

// Deps groups dependencies for account application services.
type Deps struct {
	Accounts   domainaccounts.AccountRepository
	OAuthState domainauth.OAuthStateRepository
	JobRuns    domainjobs.JobRunRepository
	// Connectors are the OAuth mailbox connect flows, keyed by provider.
	Connectors map[string]domainaccounts.OAuthMailConnector
	// PasswordConnectors connect mailboxes from typed-in credentials.
	PasswordConnectors map[string]domainaccounts.PasswordMailConnector
	// Mailboxes reports what each provider's mailboxes can do.
	Mailboxes   *MailboxOpener
	Vault       domainshared.TokenVault
	Dashboard   string
	SuccessPath string
	ErrorPath   string
	StateTTL    time.Duration
}
