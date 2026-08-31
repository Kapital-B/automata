package http

import (
	"bytes"
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

type fakeCategorizeLLM struct{}

func (f *fakeCategorizeLLM) ChatCompletion(ctx context.Context, messages []driven.LLMMessage) (*driven.LLMResponse, error) {
	return &driven.LLMResponse{Content: `{"schema_version":1,"category_slug":"important","confidence":0.91}`}, nil
}

func newCategorizeTestServer(t *testing.T) (*httptest.Server, *sqlite.Repository, uuid.UUID, uuid.UUID) {
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
	categorizeSvc := &appmessages.CategorizeService{Messages: repo, LLM: &fakeCategorizeLLM{}, JobRuns: repo}
	jwtSecret := []byte("abcdefghijklmnopqrstuvwxyz123456")
	devUser := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	authSvc := auth.NewService(repo, repo, repo, nil, nil, jwtSecret, time.Hour, 30*24*time.Hour)
	accountID := uuid.New()
	if err := repo.InsertAccount(context.Background(), driven.AccountRow{
		UserID:           devUser,
		ID:               accountID,
		Label:            "Work",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "work@example.com",
		ConnectionStatus: "connected",
	}, []byte("cipher")); err != nil {
		t.Fatal(err)
	}
	msgBody := "Need action soon."
	if err := repo.UpsertMessage(context.Background(), driven.MessageRow{
		ID:                uuid.New(),
		AccountID:         accountID,
		ProviderMessageID: "provider-1",
		ReceivedAt:        time.Now().UTC(),
		Subject:           "Action item",
		FromJSON:          `{"name":"Priya","address":"priya@example.com"}`,
		BodyText:          &msgBody,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	h := &Handlers{
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		AccountSvc: accountSvc, SyncSvc: syncSvc, CategorizeSvc: categorizeSvc, AuthSvc: authSvc,
		Accounts: repo, Messages: repo, JobRuns: repo, OAuthStates: repo, Users: repo,
		Dashboard: "http://localhost:5173", SuccessPath: "/ok", ErrorPath: "/err",
		JWTSecret: jwtSecret, JWTTTL: time.Hour, DefaultUserID: devUser, JobsInline: true,
	}
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, repo, devUser, accountID
}

func TestCategorizeEndpointAndMessageFilter(t *testing.T) {
	srv, _, _, accountID := newCategorizeTestServer(t)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/accounts/"+accountID.String()+"/categorize", bytes.NewBufferString(`{"recategorize":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("categorize status %d", res.StatusCode)
	}
	var catRes map[string]any
	if err := json.NewDecoder(res.Body).Decode(&catRes); err != nil {
		t.Fatal(err)
	}
	if catRes["job_run_id"] == "" {
		t.Fatalf("missing job_run_id: %+v", catRes)
	}
	if catRes["recategorize"] != true {
		t.Fatalf("expected recategorize true in response, got %+v", catRes["recategorize"])
	}

	res2, err := http.Get(srv.URL + "/api/messages?account_id=" + accountID.String() + "&category=important")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("messages status %d", res2.StatusCode)
	}
	var rows []map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one categorized message, got %d", len(rows))
	}
}

func TestListCategories(t *testing.T) {
	srv, _, _, _ := newCategorizeTestServer(t)
	res, err := http.Get(srv.URL + "/api/categories")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var categories []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&categories); err != nil {
		t.Fatal(err)
	}
	if len(categories) < 6 {
		t.Fatalf("expected seeded categories, got %d", len(categories))
	}
}

func TestCategoryCRUDEndpoints(t *testing.T) {
	srv, _, _, _ := newCategorizeTestServer(t)

	createReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/categories", bytes.NewBufferString(`{
		"slug":"travel",
		"display_name":"Travel",
		"definition":"Flights, hotels, trips, and bookings",
		"sort_order":75
	}`))
	if err != nil {
		t.Fatal(err)
	}
	createRes, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	defer createRes.Body.Close()
	if createRes.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d", createRes.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(createRes.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	catID, _ := created["id"].(string)
	if catID == "" {
		t.Fatalf("missing id in create response: %+v", created)
	}

	updateReq, err := http.NewRequest(http.MethodPatch, srv.URL+"/api/categories/"+catID, bytes.NewBufferString(`{
		"slug":"travel",
		"display_name":"Travel & Trips",
		"definition":"Booking confirmations and travel plans",
		"sort_order":76
	}`))
	if err != nil {
		t.Fatal(err)
	}
	updateRes, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatal(err)
	}
	defer updateRes.Body.Close()
	if updateRes.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", updateRes.StatusCode)
	}
}

func TestDeleteCategoryRequiresReplacementWhenInUse(t *testing.T) {
	srv, _, _, accountID := newCategorizeTestServer(t)
	// Categorize once so "important" is in active use.
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/accounts/"+accountID.String()+"/categorize", bytes.NewBufferString(`{"recategorize":true}`))
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("categorize status %d", res.StatusCode)
	}

	listRes, err := http.Get(srv.URL + "/api/categories")
	if err != nil {
		t.Fatal(err)
	}
	defer listRes.Body.Close()
	var categories []map[string]any
	if err := json.NewDecoder(listRes.Body).Decode(&categories); err != nil {
		t.Fatal(err)
	}
	var importantID, otherID string
	for _, c := range categories {
		slug, _ := c["slug"].(string)
		id, _ := c["id"].(string)
		if slug == "important" {
			importantID = id
		}
		if slug == "other" {
			otherID = id
		}
	}
	if importantID == "" || otherID == "" {
		t.Fatalf("missing required seeded categories")
	}

	delReq, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/categories/"+importantID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delRes, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	defer delRes.Body.Close()
	if delRes.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected bad request when deleting in-use category, got %d", delRes.StatusCode)
	}

	delReq2, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/categories/"+importantID+"?replacement_id="+otherID, nil)
	if err != nil {
		t.Fatal(err)
	}
	delRes2, err := http.DefaultClient.Do(delReq2)
	if err != nil {
		t.Fatal(err)
	}
	defer delRes2.Body.Close()
	if delRes2.StatusCode != http.StatusOK {
		t.Fatalf("expected successful delete with replacement, got %d", delRes2.StatusCode)
	}
}
