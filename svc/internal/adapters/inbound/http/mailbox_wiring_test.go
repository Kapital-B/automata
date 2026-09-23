package http

import (
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// m365Wiring builds the Microsoft provider the way composition does, so these
// tests exercise the real connect and open paths rather than a fake of them.
func m365Wiring(accounts driven.AccountRepository, vault driven.TokenVault, oauth *microsoft.OAuth, graph *microsoft.GraphClient) (map[string]driven.OAuthMailConnector, *appaccounts.MailboxOpener) {
	m365 := &microsoft.Provider{OAuth: oauth, Graph: graph}
	return map[string]driven.OAuthMailConnector{appaccounts.ProviderM365: m365},
		&appaccounts.MailboxOpener{
			Accounts:  accounts,
			Vault:     vault,
			Providers: map[string]driven.MailProvider{appaccounts.ProviderM365: m365},
		}
}
