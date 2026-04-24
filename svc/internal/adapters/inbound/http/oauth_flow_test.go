package http

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/microsoft"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// fakeAccessToken is a JWT-shaped string with a tid claim for Graph tenant parsing.
func fakeAccessToken(tenantID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]string{"tid": tenantID})
	pbody := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + pbody + ".x"
}

func newFakeMicrosoftStack(t *testing.T, wantCode string) (idp *httptest.Server, graph *httptest.Server) {
	t.Helper()

	idp = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/oauth2/v2.0/token") {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		q, _ := url.ParseQuery(string(body))
		if q.Get("grant_type") == "authorization_code" && q.Get("code") != wantCode {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fakeAccessToken("11111111-2222-3333-4444-555555555555"),
			"refresh_token": "fake-refresh-token",
			"expires_in":    3600,
		})
	}))

	graph = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1.0/me" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{
				"mail":              "user@example.com",
				"userPrincipalName": "user@example.com",
			})
		case strings.HasPrefix(r.URL.Path, "/v1.0/me/mailFolders/inbox/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	return idp, graph
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestOAuthStartConnectAndCallbackCreatesAccount(t *testing.T) {
	const authCode = "fake-auth-code-xyz"

	idp, graphSrv := newFakeMicrosoftStack(t, authCode)
	defer idp.Close()
	defer graphSrv.Close()

	db := openTestDB(t)
	key := []byte("12345678901234567890123456789012")
	vault, err := security.NewAESGCMVault(key)
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)

	oauth := &microsoft.OAuth{
		ClientID:      "test-client-id",
		ClientSecret:  "test-secret",
		BaseAuthority: idp.URL,
		HTTPClient:    idp.Client(),
	}
	graphCl := &microsoft.GraphClient{
		APIRoot:    graphSrv.URL + "/v1.0",
		HTTPClient: graphSrv.Client(),
	}

	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts: repo, OAuthState: repo, JobRuns: repo,
		OAuth: oauth, Graph: graphCl, Vault: vault,
		Dashboard: "http://dashboard.test", SuccessPath: "/connected", ErrorPath: "/error", StateTTL: 15 * time.Minute,
	})
	syncSvc := &appmessages.SyncService{
		Accounts: repo, Messages: repo, OAuth: oauth, Graph: graphCl, Vault: vault, JobRuns: repo,
	}
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	h := &Handlers{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AccountSvc: accountSvc, SyncSvc: syncSvc,
		Accounts: repo, Messages: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://dashboard.test", SuccessPath: "/connected", ErrorPath: "/error",
		JWTSecret: jwtSecret, JWTTTL: time.Hour, DefaultUserID: devUser,
	}
	api := httptest.NewServer(h.Routes())
	defer api.Close()
	oauth.RedirectURI = api.URL + "/api/accounts/callback"

	startBody := `{"provider":"m365","ms_account_kind":"work","label":"Work"}`
	res, err := http.Post(api.URL+"/api/accounts", "application/json", strings.NewReader(startBody))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("start connect status %d", res.StatusCode)
	}
	var startOut struct {
		AuthorizationURL string `json:"authorization_url"`
		State            string `json:"state"`
	}
	if err := json.NewDecoder(res.Body).Decode(&startOut); err != nil {
		t.Fatal(err)
	}
	if startOut.State == "" || startOut.AuthorizationURL == "" {
		t.Fatalf("missing fields: %+v", startOut)
	}
	u, err := url.Parse(startOut.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("state") != startOut.State {
		t.Fatalf("state mismatch in auth url")
	}
	if !strings.HasPrefix(startOut.AuthorizationURL, idp.URL) {
		t.Fatalf("auth url should use fake idp: %s", startOut.AuthorizationURL)
	}

	cb := api.URL + "/api/accounts/callback?code=" + url.QueryEscape(authCode) + "&state=" + url.QueryEscape(startOut.State)
	req, err := http.NewRequest(http.MethodGet, cb, nil)
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.oauthCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("callback status %d body %s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://dashboard.test/connected?account_id=") {
		t.Fatalf("unexpected redirect: %s", loc)
	}
	q, _ := url.Parse(loc)
	aid := q.Query().Get("account_id")
	if _, err := uuid.Parse(aid); err != nil {
		t.Fatalf("bad account_id: %s", aid)
	}

	res2, err := http.Get(api.URL + "/api/accounts")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("list accounts %d", res2.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 account got %d", len(list))
	}
	if list[0]["primary_email"] != "user@example.com" {
		t.Fatalf("email: %v", list[0]["primary_email"])
	}
	if list[0]["ms_account_kind"] != "work" {
		t.Fatalf("kind: %v", list[0]["ms_account_kind"])
	}
}

func TestOAuthCallbackInvalidStateRedirectsError(t *testing.T) {
	idp, graphSrv := newFakeMicrosoftStack(t, "any")
	defer idp.Close()
	defer graphSrv.Close()

	db := openTestDB(t)
	key := []byte("12345678901234567890123456789012")
	vault, _ := security.NewAESGCMVault(key)
	repo := sqlite.NewRepository(db, 15*time.Minute)
	oauth := &microsoft.OAuth{
		ClientID: "c", ClientSecret: "s", BaseAuthority: idp.URL, HTTPClient: idp.Client(),
	}
	graphCl := &microsoft.GraphClient{APIRoot: graphSrv.URL + "/v1.0", HTTPClient: graphSrv.Client()}
	accountSvc := appaccounts.NewService(appaccounts.Deps{
		Accounts: repo, OAuthState: repo, JobRuns: repo,
		OAuth: oauth, Graph: graphCl, Vault: vault,
		Dashboard: "http://dashboard.test", SuccessPath: "/ok", ErrorPath: "/err", StateTTL: 15 * time.Minute,
	})
	syncSvc := &appmessages.SyncService{Accounts: repo, Messages: repo, OAuth: oauth, Graph: graphCl, Vault: vault, JobRuns: repo}
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	h := &Handlers{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AccountSvc: accountSvc, SyncSvc: syncSvc,
		Accounts: repo, Messages: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://dashboard.test", SuccessPath: "/ok", ErrorPath: "/err",
		JWTSecret: jwtSecret, JWTTTL: time.Hour, DefaultUserID: devUser,
	}
	api := httptest.NewServer(h.Routes())
	defer api.Close()
	oauth.RedirectURI = api.URL + "/api/accounts/callback"

	req, _ := http.NewRequest(http.MethodGet, api.URL+"/api/accounts/callback?code=x&state=nonexistent-state", nil)
	rr := httptest.NewRecorder()
	h.oauthCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "code=invalid_state") {
		t.Fatalf("want invalid_state redirect, got %s", loc)
	}
}

func TestOAuthCallbackAccessDeniedRedirects(t *testing.T) {
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	h := &Handlers{
		Dashboard: "http://d.test", ErrorPath: "/err",
		JWTSecret: jwtSecret, DefaultUserID: devUser,
	}
	req, _ := http.NewRequest(http.MethodGet, "/cb?error=access_denied", nil)
	rr := httptest.NewRecorder()
	h.oauthCallback(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("status %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Location"), "code=access_denied") {
		t.Fatalf("got %s", rr.Header().Get("Location"))
	}
}
