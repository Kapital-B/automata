package accounts

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// ProviderM365 is the provider value for Microsoft 365 mailboxes, and what
// every row written before provider support means.
const ProviderM365 = "m365"

// ProviderKey normalises a stored provider value.
func ProviderKey(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		return ProviderM365
	}
	return p
}

// MailboxOpener turns an account into an open mailbox. It is the one place
// that knows credentials are encrypted at rest, which provider decodes them,
// and that a provider may hand back rotated credentials to persist. Services
// above it never see a token.
type MailboxOpener struct {
	Accounts  driven.AccountRepository
	Vault     driven.TokenVault
	Providers map[string]driven.MailProvider
}

// Open returns a ready mailbox and the account it belongs to.
func (o *MailboxOpener) Open(ctx context.Context, userID, accountID uuid.UUID) (driven.Mailbox, *driven.AccountRow, error) {
	if o == nil || o.Accounts == nil || o.Vault == nil {
		return nil, nil, fmt.Errorf("mailbox opener not configured")
	}
	row, cipher, err := o.Accounts.GetAccount(ctx, userID, accountID)
	if err != nil {
		return nil, nil, err
	}
	if row == nil {
		return nil, nil, fmt.Errorf("account not found")
	}
	if len(cipher) == 0 {
		return nil, row, fmt.Errorf("no tokens for account")
	}
	provider, ok := o.Providers[ProviderKey(row.Provider)]
	if !ok {
		return nil, row, fmt.Errorf("%w: %s", driven.ErrUnsupportedProvider, row.Provider)
	}
	raw, err := o.Vault.Decrypt(cipher)
	if err != nil {
		return nil, row, err
	}
	box, rotated, err := provider.Open(ctx, *row, raw)
	if err != nil {
		return nil, row, err
	}
	if rotated != nil {
		next, err := o.Vault.Encrypt(rotated)
		if err != nil {
			return nil, row, err
		}
		if err := o.Accounts.UpdateAccountTokens(ctx, userID, accountID, next, row.PrimaryEmail, row.GraphTenantID, row.MsalHomeAccountID, "connected", nil); err != nil {
			return nil, row, err
		}
	}
	return box, row, nil
}
