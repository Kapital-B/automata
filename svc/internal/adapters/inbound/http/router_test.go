package http

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	"github.com/google/uuid"
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
	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts: repo, OAuthState: repo, JobRuns: repo, OAuth: oauth, Graph: graph, Vault: vault,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err", StateTTL: 0,
	})
	syncSvc := &appmessages.SyncService{Accounts: repo, Messages: repo, OAuth: oauth, Graph: graph, Vault: vault, JobRuns: repo}
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	h := &Handlers{
		Log: slog.Default(), AccountSvc: accountSvc, SyncSvc: syncSvc, AuthSvc: authSvc,
		Accounts: repo, Messages: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err",
		JWTSecret: jwtSecret, JWTTTL: time.Hour, DefaultUserID: devUser,
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
