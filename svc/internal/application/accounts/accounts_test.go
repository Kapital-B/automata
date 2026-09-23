package accounts_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
)

type stubBox struct{ driven.Mailbox }

// stubProvider opens mailboxes on command.
type stubProvider struct {
	openErr error
	rotated []byte
}

func (p *stubProvider) Open(ctx context.Context, account driven.AccountRow, credential []byte) (driven.Mailbox, []byte, error) {
	if p.openErr != nil {
		return nil, nil, p.openErr
	}
	return stubBox{}, p.rotated, nil
}

// stubConnector completes a connect as whoever it is told to be.
type stubConnector struct {
	email string
	cred  string
}

func (c *stubConnector) AuthorizationURL(ctx context.Context, state string, opts driven.ConnectOptions) (string, error) {
	return "https://idp.example/auth?state=" + state, nil
}

func (c *stubConnector) Complete(ctx context.Context, code string, opts driven.ConnectOptions) (*driven.ConnectedMailbox, error) {
	return &driven.ConnectedMailbox{Email: c.email, DefaultLabel: c.email, Credential: []byte(c.cred)}, nil
}

type fixture struct {
	repo   *sqlite.Repository
	vault  driven.TokenVault
	userID uuid.UUID
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, time.Minute)
	vault, err := security.NewAESGCMVault([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	if _, err := repo.CreateUserWithHomeOrg(context.Background(), userID, "u@example.com", nil, time.Now().UTC(), "password", userID.String(), "u@example.com"); err != nil {
		t.Fatal(err)
	}
	return &fixture{repo: repo, vault: vault, userID: userID}
}

func (f *fixture) account(t *testing.T, provider, email string) uuid.UUID {
	t.Helper()
	cipher, err := f.vault.Encrypt([]byte("stored-credential"))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	now := time.Now().UTC()
	if err := f.repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID: f.userID, ID: id, Label: email, Provider: provider, MsAccountKind: domainacc.KindWork,
		PrimaryEmail: email, ConnectionStatus: "connected", CreatedAt: now, UpdatedAt: now,
	}, cipher); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) opener(p driven.MailProvider) *appaccounts.MailboxOpener {
	return &appaccounts.MailboxOpener{Accounts: f.repo, Vault: f.vault, Providers: map[string]driven.MailProvider{"google": p, "m365": p}}
}

// A revoked token is not a transient failure. Marking the account expired is
// what turns it into a Reconnect prompt instead of a sync that fails forever.
func TestOpenMarksAnAccountWithRejectedCredentialsExpired(t *testing.T) {
	f := setup(t)
	id := f.account(t, "google", "owner@example.org")
	rejected := fmt.Errorf("%w: invalid_grant", driven.ErrCredentialsRejected)

	_, _, err := f.opener(&stubProvider{openErr: rejected}).Open(context.Background(), f.userID, id)
	if !errors.Is(err, driven.ErrCredentialsRejected) {
		t.Fatalf("err = %v", err)
	}
	row, cipher, err := f.repo.GetAccount(context.Background(), f.userID, id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConnectionStatus != "expired" || row.LastError == nil {
		t.Errorf("status = %q, last_error = %v; want expired with the reason", row.ConnectionStatus, row.LastError)
	}
	// The credential is kept for the reconnect to replace, not wiped.
	if plain, _ := f.vault.Decrypt(cipher); string(plain) != "stored-credential" {
		t.Errorf("credential changed to %q", plain)
	}
}

// Any other failure leaves the account alone: a network blip is not a reason
// to tell someone to reconnect.
func TestOpenLeavesAnAccountAloneOnOtherFailures(t *testing.T) {
	f := setup(t)
	id := f.account(t, "google", "owner@example.org")
	if _, _, err := f.opener(&stubProvider{openErr: errors.New("connection reset")}).Open(context.Background(), f.userID, id); err == nil {
		t.Fatal("expected the failure to surface")
	}
	row, _, _ := f.repo.GetAccount(context.Background(), f.userID, id)
	if row.ConnectionStatus != "connected" {
		t.Errorf("status = %q, want connected", row.ConnectionStatus)
	}
}

func TestOpenPersistsRotatedCredentials(t *testing.T) {
	f := setup(t)
	id := f.account(t, "google", "owner@example.org")
	if _, _, err := f.opener(&stubProvider{rotated: []byte("rotated")}).Open(context.Background(), f.userID, id); err != nil {
		t.Fatal(err)
	}
	_, cipher, _ := f.repo.GetAccount(context.Background(), f.userID, id)
	if plain, _ := f.vault.Decrypt(cipher); string(plain) != "rotated" {
		t.Errorf("credential = %q, want the rotated one", plain)
	}
}

func TestOpenRefusesAProviderWithNoAdapter(t *testing.T) {
	f := setup(t)
	id := f.account(t, "imap", "owner@example.org")
	opener := &appaccounts.MailboxOpener{Accounts: f.repo, Vault: f.vault, Providers: map[string]driven.MailProvider{}}
	if _, _, err := opener.Open(context.Background(), f.userID, id); !errors.Is(err, driven.ErrUnsupportedProvider) {
		t.Fatalf("err = %v, want ErrUnsupportedProvider", err)
	}
}

func (f *fixture) service(conn driven.OAuthMailConnector) *appaccounts.Service {
	return appaccounts.NewService(appaccounts.Deps{
		Accounts: f.repo, OAuthState: f.repo, Vault: f.vault,
		Connectors: map[string]driven.OAuthMailConnector{"google": conn},
		StateTTL:   time.Minute,
	})
}

func (f *fixture) connect(t *testing.T, svc *appaccounts.Service) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	start, err := svc.StartConnect(ctx, f.userID, appaccounts.StartConnectInput{Provider: "google"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.CompleteOAuth(ctx, "code", start.State)
	if err != nil {
		t.Fatal(err)
	}
	return res.AccountID
}

// Reconnecting after expiry must restore the account the user already has —
// its rules, assignments and history hang off that id — not add a second one
// that syncs the same mail again.
func TestReconnectRestoresTheExistingAccount(t *testing.T) {
	f := setup(t)
	svc := f.service(&stubConnector{email: "owner@example.org", cred: "first"})
	first := f.connect(t, svc)

	// Expire it, then reconnect with fresh credentials.
	msg := "revoked"
	_, cipher, _ := f.repo.GetAccount(context.Background(), f.userID, first)
	_ = f.repo.UpdateAccountTokens(context.Background(), f.userID, first, cipher, "owner@example.org", nil, nil, "expired", &msg)
	again := f.connect(t, f.service(&stubConnector{email: "Owner@Example.org", cred: "second"}))

	if again != first {
		t.Fatalf("reconnect created account %s, want %s restored", again, first)
	}
	rows, _ := f.repo.ListAccounts(context.Background(), f.userID)
	if len(rows) != 1 {
		t.Fatalf("accounts = %d, want 1", len(rows))
	}
	row, cipher, _ := f.repo.GetAccount(context.Background(), f.userID, first)
	if row.ConnectionStatus != "connected" || row.LastError != nil {
		t.Errorf("status = %q last_error = %v; want connected and cleared", row.ConnectionStatus, row.LastError)
	}
	if plain, _ := f.vault.Decrypt(cipher); string(plain) != "second" {
		t.Errorf("credential = %q, want the new one", plain)
	}
}

// Two different Google mailboxes are two accounts.
func TestSeveralGoogleMailboxesPerUser(t *testing.T) {
	f := setup(t)
	a := f.connect(t, f.service(&stubConnector{email: "work@example.org", cred: "a"}))
	b := f.connect(t, f.service(&stubConnector{email: "me@gmail.com", cred: "b"}))
	if a == b {
		t.Fatal("second mailbox overwrote the first")
	}
	rows, _ := f.repo.ListAccounts(context.Background(), f.userID)
	if len(rows) != 2 {
		t.Fatalf("accounts = %d, want 2", len(rows))
	}
	for _, r := range rows {
		if r.Provider != "google" {
			t.Errorf("provider = %q", r.Provider)
		}
		// RFC §5.1 option A: a placeholder the non-Microsoft path never reads.
		if r.MsAccountKind != domainacc.KindWork {
			t.Errorf("ms_account_kind = %q, want the placeholder", r.MsAccountKind)
		}
	}
}

// A sign-in state cannot complete a mailbox connect: signing in with Google
// must never attach a mailbox.
func TestSignInStateCannotConnectAMailbox(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	payload, _ := appaccounts.EncodeMailboxOAuthPayload("google", "", nil)
	if err := f.repo.InsertOAuthState(ctx, "signin-state", "auth_google", &f.userID, payload, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, err := f.service(&stubConnector{email: "x@example.org", cred: "x"}).CompleteOAuth(ctx, "code", "signin-state")
	if !errors.Is(err, appaccounts.ErrInvalidOAuthState) {
		t.Fatalf("err = %v, want ErrInvalidOAuthState", err)
	}
}

// A mailbox state for a provider this deployment has no connector for — one
// written before its client was unconfigured, say — is refused, not guessed.
func TestConnectStateForAnUnconfiguredProviderIsRefused(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	payload, _ := appaccounts.EncodeMailboxOAuthPayload("imap", "", nil)
	if err := f.repo.InsertOAuthState(ctx, "orphan-state", "m365_mail", &f.userID, payload, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, err := f.service(&stubConnector{email: "x@example.org", cred: "x"}).CompleteOAuth(ctx, "code", "orphan-state")
	if !errors.Is(err, appaccounts.ErrInvalidOAuthState) {
		t.Fatalf("err = %v, want ErrInvalidOAuthState", err)
	}
}
