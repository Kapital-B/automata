package http

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	queries "github.com/Kapital-B/automata/svc/internal/application/queries"
	submissions "github.com/Kapital-B/automata/svc/internal/application/submissions"
	_ "modernc.org/sqlite"
)

func TestHealth(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	key := []byte("12345678901234567890123456789012")
	vault, _ := security.NewAESGCMVault(key)
	repo := sqlite.NewRepository(db, 15*time.Minute)
	oauth := &microsoft.OAuth{ClientID: "x", ClientSecret: "y", RedirectURI: "http://localhost:8080/api/accounts/callback"}
	graph := &microsoft.GraphClient{}
	connectors, mailboxes := m365Wiring(repo, vault, oauth, graph)
	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts: repo, OAuthState: repo, JobRuns: repo, Vault: vault, Connectors: connectors,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err", StateTTL: 0,
	})
	syncSvc := &appmessages.SyncService{Accounts: repo, Messages: repo, Mailboxes: mailboxes, JobRuns: repo}
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, security.JWT{Secret: jwtSecret, TTL: time.Hour}, security.BcryptHasher{}, 30*24*time.Hour)
	h := &Handlers{
		Log: slog.Default(), AccountSvc: accountSvc, SyncSvc: syncSvc, AuthSvc: authSvc,
		AccountsQueries: &queries.Service{Accounts: repo}, MessagesQueries: &queries.Service{Messages: repo}, JobRunsQueries: &queries.Service{JobRuns: repo}, UsersQueries: &queries.Service{Users: repo},
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err", TokenVerifier: security.JWT{Secret: jwtSecret}, AccountJobs: &submissions.Service{Accounts: repo, Runs: repo, Sync: syncSvc}, ConnectorJobs: &submissions.Service{Accounts: repo, Runs: repo, Sync: syncSvc}, Extraction: &submissions.Service{Accounts: repo, Runs: repo, Sync: syncSvc}, JobRequests: &submissions.Service{Accounts: repo, Runs: repo, Sync: syncSvc},
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
