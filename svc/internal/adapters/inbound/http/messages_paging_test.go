package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestListMessagesKeysetPaging(t *testing.T) {
	db, err := sql.Open("sqlite", "file:messagespaging?mode=memory&cache=shared")
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
	h := &Handlers{
		Log: slog.Default(), AuthSvc: authSvc,
		Users: repo, Messages: repo, Projects: repo, Assignments: repo,
		JWTSecret: jwtSecret, JWTTTL: time.Hour,
	}
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	userID, tokens := registerAndLogin(t, authSvc, "paging@example.com", "password123")
	ctx := context.Background()
	accountID := uuid.New()
	if err := repo.InsertAccount(ctx, driven.AccountRow{
		UserID: userID, ID: accountID, Label: "Work", Provider: "m365",
		MsAccountKind: "work", PrimaryEmail: "paging@example.com", ConnectionStatus: "connected",
	}, []byte("tok")); err != nil {
		t.Fatal(err)
	}
	insert := func(at time.Time) {
		t.Helper()
		id := uuid.New()
		if err := repo.UpsertMessage(ctx, driven.MessageRow{
			ID: id, AccountID: accountID, ProviderMessageID: id.String(),
			ReceivedAt: at, Subject: "s", FromJSON: `{}`,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Sub-second timestamps: the cursor must round-trip through the JSON
	// received_at without losing precision, or the boundary row repeats.
	base := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		insert(base.Add(-time.Duration(i)*time.Second - 123456*time.Microsecond))
	}

	type item struct {
		ID         string `json:"id"`
		ReceivedAt string `json:"received_at"`
	}
	list := func(q url.Values) (int, []item) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/messages?"+q.Encode(), nil)
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out []item
		if res.StatusCode == http.StatusOK {
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode, out
	}

	seen := map[string]bool{}
	q := url.Values{"limit": {"2"}}
	for page := 0; page < 5; page++ {
		status, got := list(q)
		if status != http.StatusOK {
			t.Fatalf("page %d status %d", page, status)
		}
		if len(got) == 0 {
			break
		}
		for _, m := range got {
			if seen[m.ID] {
				t.Fatalf("message %s repeated on page %d", m.ID, page)
			}
			seen[m.ID] = true
		}
		insert(base.Add(time.Duration(page+1) * time.Minute))
		last := got[len(got)-1]
		q.Set("before_received_at", last.ReceivedAt)
		q.Set("before_id", last.ID)
	}
	if len(seen) != 5 {
		t.Fatalf("paged through %d messages, want 5", len(seen))
	}

	valid := base.Format(time.RFC3339Nano)
	for name, q := range map[string]url.Values{
		"negative offset":   {"offset": {"-1"}},
		"id without time":   {"before_id": {uuid.NewString()}},
		"time without id":   {"before_received_at": {valid}},
		"bad time":          {"before_received_at": {"yesterday"}, "before_id": {uuid.NewString()}},
		"bad id":            {"before_received_at": {valid}, "before_id": {"nope"}},
		"offset and cursor": {"offset": {"2"}, "before_received_at": {valid}, "before_id": {uuid.NewString()}},
	} {
		if status, _ := list(q); status != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, status)
		}
	}
}
