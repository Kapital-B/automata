package accounts

import (
	"context"
	"errors"
	"fmt"

	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"github.com/google/uuid"
)

const ProviderM365 = domainaccounts.AccountProviderM365

// ProviderGoogle covers Gmail and Google Workspace, which are one mailbox.
const ProviderGoogle = "google"

const ProviderIMAP = domainaccounts.AccountProviderIMAP

var ProviderKey = domainaccounts.AccountProviderKey

// MailboxOpener turns an account into an open mailbox. It is the one place
// that knows credentials are encrypted at rest, which provider decodes them,
// and that a provider may hand back rotated credentials to persist. Services
// above it never see a token.
type MailboxOpener struct {
	Accounts  domainaccounts.AccountRepository
	Vault     domainshared.TokenVault
	Providers map[string]domainaccounts.MailProvider
}

// Open returns a ready mailbox and the account it belongs to.
func (o *MailboxOpener) Open(ctx context.Context, userID, accountID uuid.UUID) (domainaccounts.Mailbox, *domainaccounts.AccountRow, error) {
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
		return nil, row, fmt.Errorf("%w: %s", domainaccounts.ErrUnsupportedProvider, row.Provider)
	}
	raw, err := o.Vault.Decrypt(cipher)
	if err != nil {
		return nil, row, err
	}
	box, rotated, err := provider.Open(ctx, *row, raw)
	if errors.Is(err, domainaccounts.ErrCredentialsRejected) {
		// Say so on the account rather than let every sync fail the same
		// way: expired is what the UI turns into a Reconnect prompt. The
		// stored credential is kept; a reconnect replaces it.
		msg := err.Error()
		_ = o.Accounts.UpdateAccountTokens(ctx, userID, accountID, cipher, row.PrimaryEmail, row.GraphTenantID, row.MsalHomeAccountID, "expired", &msg)
		return nil, row, err
	}
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

// Capabilities reports what a provider's mailboxes can do, without opening
// one. ok is false for a provider with no adapter registered.
func (o *MailboxOpener) Capabilities(provider string) (caps domainaccounts.MailboxCapabilities, ok bool) {
	if o == nil {
		return caps, false
	}
	p, ok := o.Providers[ProviderKey(provider)]
	if !ok {
		return caps, false
	}
	return p.Capabilities(), true
}
