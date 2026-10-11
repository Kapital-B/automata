package http

import (
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
)

// m365Wiring builds the Microsoft provider the way composition does, so these
// tests exercise the real connect and open paths rather than a fake of them.
func m365Wiring(accounts domainaccounts.AccountRepository, vault domainshared.TokenVault, oauth *microsoft.OAuth, graph *microsoft.GraphClient) (map[string]domainaccounts.OAuthMailConnector, *appaccounts.MailboxOpener) {
	m365 := &microsoft.Provider{OAuth: oauth, Graph: graph}
	return map[string]domainaccounts.OAuthMailConnector{appaccounts.ProviderM365: m365},
		&appaccounts.MailboxOpener{
			Accounts:  accounts,
			Vault:     vault,
			Providers: map[string]domainaccounts.MailProvider{appaccounts.ProviderM365: m365},
		}
}
