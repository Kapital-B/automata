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
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	h := &Handlers{
		Log: slog.Default(), AccountSvc: accountSvc, SyncSvc: syncSvc, AuthSvc: authSvc,
		Accounts: repo, Messages: repo, JobRuns: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err",
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
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
