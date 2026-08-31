package http

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestAuthRefreshRotatesTokens(t *testing.T) {
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
	oauth := &microsoft.OAuth{ClientID: "x", ClientSecret: "y", RedirectURI: "http://localhost:8080/cb"}
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
	defer srv.Close()

	regBody := `{"email":"refresh-test@example.com","password":"hunter2hunter2"}`
	res, err := http.Post(srv.URL+"/api/auth/register", "application/json", strings.NewReader(regBody))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register %d", res.StatusCode)
	}
	var regOut struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&regOut); err != nil {
		t.Fatal(err)
	}
	if regOut.RefreshToken == "" || regOut.AccessToken == "" {
		t.Fatal("missing tokens")
	}

	refreshPayload := `{"refresh_token":` + jsonString(regOut.RefreshToken) + `}`
	res2, err := http.Post(srv.URL+"/api/auth/refresh", "application/json", strings.NewReader(refreshPayload))
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("refresh %d", res2.StatusCode)
	}
	var pair2 struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&pair2); err != nil {
		t.Fatal(err)
	}
	if pair2.RefreshToken == regOut.RefreshToken {
		t.Fatal("expected new refresh token (rotation)")
	}
	// access_token may match previous if both JWTs fall in the same second (iat/exp are second-precision)

	res3, err := http.Post(srv.URL+"/api/auth/refresh", "application/json", strings.NewReader(refreshPayload))
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reuse old refresh want 401 got %d", res3.StatusCode)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
