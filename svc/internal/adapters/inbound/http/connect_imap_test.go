package http

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/security"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// recordingPasswordConnector accepts one password and records what it saw.
type recordingPasswordConnector struct {
	password string
	got      driven.PasswordConnectRequest
}

func (c *recordingPasswordConnector) Connect(_ context.Context, req driven.PasswordConnectRequest) (*driven.ConnectedMailbox, error) {
	c.got = req
	if req.Password != c.password {
		return nil, driven.ConnectRejected("the IMAP server at %s refused the username or password", req.IMAP.Host)
	}
	return &driven.ConnectedMailbox{Email: req.Email, DefaultLabel: req.Email, Credential: []byte("cred")}, nil
}

func TestConnectIMAPStoresAVerifiedAccountAndReportsRejections(t *testing.T) {
	db := openTestDB(t)
	vault, err := security.NewAESGCMVault([]byte("12345678901234567890123456789012"))
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	conn := &recordingPasswordConnector{password: "right"}
	h := &Handlers{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AccountSvc: appaccounts.NewService(appaccounts.Deps{
			Accounts: repo, OAuthState: repo, Vault: vault,
			PasswordConnectors: map[string]driven.PasswordMailConnector{appaccounts.ProviderIMAP: conn},
		}),
		Accounts: repo, Users: repo,
		JWTSecret: []byte("abcdefghijklmnopqrstuvwxyz123456"), JWTTTL: time.Hour,
		DefaultUserID: uuid.MustParse("a0000001-0000-4000-8000-000000000001"),
	}
	api := httptest.NewServer(h.Routes())
	defer api.Close()

	post := func(password string) (int, map[string]string) {
		body := `{"email":"me@fastmail.com","password":"` + password + `",
			"imap":{"host":"imap.fastmail.com","port":993,"security":"tls"},
			"smtp":{"host":"smtp.fastmail.com","port":465,"security":"tls"},"label":"Personal"}`
		res, err := http.Post(api.URL+"/api/accounts/imap", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]string
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	status, out := post("wrong")
	if status != http.StatusUnprocessableEntity || !strings.Contains(out["error"], "refused the username or password") {
		t.Fatalf("wrong password: %d %v, want 422 naming the cause", status, out)
	}

	status, out = post("right")
	if status != http.StatusOK {
		t.Fatalf("connect: %d %v", status, out)
	}
	if conn.got.IMAP != (driven.MailServer{Host: "imap.fastmail.com", Port: 993, Security: "tls"}) || conn.got.SMTP.Port != 465 {
		t.Errorf("servers reached the connector as %+v / %+v", conn.got.IMAP, conn.got.SMTP)
	}
	rows, err := repo.ListAccounts(context.Background(), h.DefaultUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID.String() != out["account_id"] || rows[0].Provider != "imap" || rows[0].Label != "Personal" {
		t.Fatalf("accounts = %+v", rows)
	}
}
