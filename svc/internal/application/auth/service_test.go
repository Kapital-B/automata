package auth_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	_ "modernc.org/sqlite"
)

func TestRegisterCreatesHomeOrganisation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:authreg?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db, time.Minute)
	svc := auth.NewService(repo, repo, repo, nil, nil, []byte("abcdefghijklmnopqrstuvwxyz123456"), time.Hour, 24*time.Hour)
	id, err := svc.Register(context.Background(), "newuser@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	orgID, err := repo.GetHomeOrganisationID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if orgID.String() == "" {
		t.Fatal("missing home org")
	}
	var role string
	if err := db.QueryRow(`
		SELECT org_role FROM organisation_members WHERE organisation_id = ? AND user_id = ?
	`, orgID.String(), id.String()).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "owner" {
		t.Fatalf("role=%s", role)
	}
}
