package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
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
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func newRunsTestServer(t *testing.T) (*httptest.Server, *sqlite.Repository, uuid.UUID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	key := []byte("12345678901234567890123456789012")
	vault, err := security.NewAESGCMVault(key)
	if err != nil {
		t.Fatal(err)
	}
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
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		AccountSvc: accountSvc, SyncSvc: syncSvc, AuthSvc: authSvc,
		Accounts: repo, Messages: repo, JobRuns: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err",
		JWTSecret: jwtSecret, JWTTTL: time.Hour, DefaultUserID: devUser,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, repo, devUser
}

func insertAccountForUser(t *testing.T, repo *sqlite.Repository, userID, accountID uuid.UUID, label string) {
	t.Helper()
	err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            label,
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     label + "@example.com",
		ConnectionStatus: "connected",
	}, []byte("cipher"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunsListAndFilter(t *testing.T) {
	srv, repo, devUser := newRunsTestServer(t)
	otherUser := uuid.MustParse("b0000001-0000-4000-8000-000000000002")
	devAccount := uuid.New()
	otherAccount := uuid.New()

	insertAccountForUser(t, repo, devUser, devAccount, "dev")
	insertAccountForUser(t, repo, otherUser, otherAccount, "other")

	now := time.Now().UTC()
	older := now.Add(-time.Hour)
	err := repo.InsertJobRun(context.Background(), uuid.New(), devAccount, "sync", "api", "success", now, now, nil, `{"messages_upserted":2}`)
	if err != nil {
		t.Fatal(err)
	}
	err = repo.InsertJobRun(context.Background(), uuid.New(), devAccount, "categorize", "schedule", "success", older, older, nil, `{"messages":4}`)
	if err != nil {
		t.Fatal(err)
	}
	err = repo.InsertJobRun(context.Background(), uuid.New(), otherAccount, "sync", "api", "success", now, now, nil, `{"messages_upserted":9}`)
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/api/runs")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var all []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&all); err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 runs for default user got %d", len(all))
	}
	if all[0]["job_type"] != "sync" {
		t.Fatalf("newest first expected sync got %v", all[0]["job_type"])
	}

	res2, err := http.Get(srv.URL + "/api/runs?account_id=" + devAccount.String() + "&job_type=sync")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res2.StatusCode)
	}
	var filtered []map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("want 1 filtered run got %d", len(filtered))
	}
	if filtered[0]["job_type"] != "sync" {
		t.Fatalf("unexpected filtered job_type %v", filtered[0]["job_type"])
	}
}

func TestGetRunNotFoundOrNotOwned(t *testing.T) {
	srv, repo, devUser := newRunsTestServer(t)
	otherUser := uuid.MustParse("b0000001-0000-4000-8000-000000000002")
	devAccount := uuid.New()
	otherAccount := uuid.New()
	insertAccountForUser(t, repo, devUser, devAccount, "dev")
	insertAccountForUser(t, repo, otherUser, otherAccount, "other")

	otherRunID := uuid.New()
	err := repo.InsertJobRun(context.Background(), otherRunID, otherAccount, "sync", "api", "success", time.Now().UTC(), time.Now().UTC(), nil, `{}`)
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/api/runs/" + uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("missing run status %d", res.StatusCode)
	}

	res2, err := http.Get(srv.URL + "/api/runs/" + otherRunID.String())
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("not-owned run status %d", res2.StatusCode)
	}
}
