package http

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestContactsIsolationAndMergeHTTP(t *testing.T) {
	db, err := sql.Open("sqlite", "file:contactshttp?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, 15*time.Minute)
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	contactSvc := &appcontacts.Service{Users: repo, Contacts: repo, Messages: repo}
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc, ContactSvc: contactSvc,
		Users: repo, Contacts: repo, Messages: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	idA, tokensA := registerAndLogin(t, authSvc, "a@example.com", "password123")
	_, tokensB := registerAndLogin(t, authSvc, "b@example.com", "password123")

	orgA, err := repo.GetHomeOrganisationID(context.Background(), idA)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	survivor, err := repo.ResolveEmailContact(context.Background(), orgA, "one@acme.com", "Alex", now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := repo.ResolveEmailContact(context.Background(), orgA, "two@acme.com", "Alex", now)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/contacts", nil)
	req.Header.Set("Authorization", "Bearer "+tokensB.AccessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("user B must not see user A contacts, got %d", len(list))
	}

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/contacts/"+survivor.String(), nil)
	req2.Header.Set("Authorization", "Bearer "+tokensB.AccessToken)
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for other user's contact, got %d", res2.StatusCode)
	}

	body, _ := json.Marshal(map[string]string{"source_contact_id": source.String()})
	req3, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/contacts/"+survivor.String()+"/merge", bytes.NewReader(body))
	req3.Header.Set("Authorization", "Bearer "+tokensA.AccessToken)
	req3.Header.Set("Content-Type", "application/json")
	res3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusOK {
		t.Fatalf("merge status %d", res3.StatusCode)
	}

	req4, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/me", nil)
	req4.Header.Set("Authorization", "Bearer "+tokensA.AccessToken)
	res4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatal(err)
	}
	defer res4.Body.Close()
	var me map[string]any
	if err := json.NewDecoder(res4.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	if me["home_organisation_id"] == nil || me["home_organisation_id"] == "" {
		t.Fatal("me should include home_organisation_id")
	}
}

func registerAndLogin(t *testing.T, svc *auth.Service, email, password string) (uuid.UUID, auth.TokenPair) {
	t.Helper()
	id, err := svc.Register(context.Background(), email, password)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := svc.LoginPassword(context.Background(), email, password)
	if err != nil {
		t.Fatal(err)
	}
	return id, pair
}
