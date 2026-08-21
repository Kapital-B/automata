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

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/google/uuid"
)

func TestCreateForwardRuleDefaultsDisabled(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	repo := sqlite.NewRepository(db, 15*time.Minute)
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID:           devUser,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceForwardAllowlist(ctx, devUser, []string{"dest@example.com"}); err != nil {
		t.Fatal(err)
	}

	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	h := &Handlers{
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts:   repo,
		Forwards:   repo,
		JWTSecret:  jwtSecret,
		DefaultUserID: devUser,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	payload := `{"name":"t","mode":"logic","condition_json":{"all":[]},"forward_to":"dest@example.com"}`
	res, err := http.Post(srv.URL+"/api/accounts/"+accountID.String()+"/forward-rules", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d: %s", res.StatusCode, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	rules, err := repo.ListForwardRules(ctx, devUser, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules len %d", len(rules))
	}
	if rules[0].Enabled {
		t.Fatal("expected new rule disabled by default")
	}
}

func TestCreateForwardRuleRejectsNonAllowlisted(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	repo := sqlite.NewRepository(db, 15*time.Minute)
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID:           devUser,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceForwardAllowlist(ctx, devUser, []string{"dest@example.com"}); err != nil {
		t.Fatal(err)
	}

	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	h := &Handlers{
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		Accounts:      repo,
		Forwards:      repo,
		JWTSecret:     jwtSecret,
		DefaultUserID: devUser,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	payload := `{"name":"t","mode":"logic","condition_json":{"all":[]},"forward_to":"evil@example.com"}`
	res, err := http.Post(srv.URL+"/api/accounts/"+accountID.String()+"/forward-rules", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
}

func TestForwardDestInAllowlist(t *testing.T) {
	rows := []driven.ForwardAllowlistRow{{Email: "A@Example.COM "}}
	if !forwardDestInAllowlist("a@example.com", rows) {
		t.Fatal("expected match")
	}
	if forwardDestInAllowlist("other@example.com", rows) {
		t.Fatal("expected no match")
	}
}
