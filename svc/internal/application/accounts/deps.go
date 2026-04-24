package accounts

import (
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// Deps groups dependencies for account application services.
type Deps struct {
	Accounts    driven.AccountRepository
	OAuthState  driven.OAuthStateRepository
	JobRuns     driven.JobRunRepository
	OAuth       driven.MicrosoftOAuth
	Graph       driven.MicrosoftGraph
	Vault       driven.TokenVault
	Dashboard   string
	SuccessPath string
	ErrorPath   string
	StateTTL    time.Duration
}
