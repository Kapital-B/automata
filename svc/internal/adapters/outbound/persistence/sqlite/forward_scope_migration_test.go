package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Existing rules carry on just after the newest message forwarding had
// checked, so deploying the per-rule scope neither reaches back nor drops
// mail that was still waiting.
func TestForwardScopeMigrationContinuesWhereForwardingLeftOff(t *testing.T) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	userID, accountA, accountB := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := formatRFC3339(time.Now())
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db, time.Minute)
	uid, _ := uuid.Parse(userID)
	if _, err := repo.CreateUserWithHomeOrg(context.Background(), uid, "u@example.com", nil, time.Now().UTC(), "password", userID, "u@example.com"); err != nil {
		t.Fatal(err)
	}
	for _, acc := range []string{accountA, accountB} {
		id, _ := uuid.Parse(acc)
		if err := repo.InsertAccount(context.Background(), driven.AccountRow{
			UserID: uid, ID: id, Label: "x", Provider: "m365", MsAccountKind: "work", PrimaryEmail: "x@example.com", ConnectionStatus: "connected",
		}, []byte{0}); err != nil {
			t.Fatal(err)
		}
	}
	msg := func(account, received string, seen bool) {
		var seenAt any
		if seen {
			seenAt = now
		}
		mustExec(`INSERT INTO messages (id, account_id, provider_message_id, received_at, subject, from_json, created_at, updated_at, forward_seen_at)
			VALUES (?, ?, ?, ?, 's', '{}', ?, ?, ?)`, uuid.NewString(), account, uuid.NewString(), received, now, now, seenAt)
	}
	msg(accountA, "2026-09-01T10:00:00Z", true)
	msg(accountA, "2026-09-02T10:00:00.5Z", true)
	msg(accountA, "2026-09-03T10:00:00Z", false)
	rule := func(account, created string) string {
		id := uuid.NewString()
		mustExec(`INSERT INTO forward_rules (id, user_id, account_id, name, mode, condition_json, forward_to, enabled, created_at, updated_at)
			VALUES (?, ?, ?, 'r', 'logic', '{}', 'd@example.com', 1, ?, ?)`, id, userID, account, created, created)
		return id
	}
	ruleA := rule(accountA, "2026-08-01T00:00:00Z")
	ruleB := rule(accountB, "2026-08-05T00:00:00Z")
	mustExec(`UPDATE forward_rules SET apply_from = NULL`)

	if err := migrateForwardRuleScope(db); err != nil {
		t.Fatal(err)
	}
	get := func(id string) time.Time {
		var s string
		if err := db.QueryRow(`SELECT apply_from FROM forward_rules WHERE id = ?`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		v, err := parseTime(s)
		if err != nil {
			t.Fatalf("apply_from %q does not parse: %v", s, err)
		}
		return v
	}
	lastSeen := time.Date(2026, 9, 2, 10, 0, 0, 500_000_000, time.UTC)
	if a := get(ruleA); !a.After(lastSeen) || !a.Before(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("rule A starts %v, want just after %v", a, lastSeen)
	}
	if b := get(ruleB); !b.Equal(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("rule B starts %v, want its creation", b)
	}
}
