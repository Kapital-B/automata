package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

func TestPromoteJobRunToRunning(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db, 15*time.Minute)
	userID := uuid.MustParse("a0000001-0000-4000-8000-000000000001")
	accountID := uuid.New()
	runID := uuid.New()
	startedPending := time.Now().UTC().Add(-time.Minute)

	err = repo.InsertAccount(ctx, driven.AccountRow{
		UserID:           userID,
		ID:               accountID,
		Label:            "t",
		Provider:         "m365",
		MsAccountKind:    domainacc.KindWork,
		PrimaryEmail:     "t@example.com",
		ConnectionStatus: "connected",
	}, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.InsertJobRun(ctx, runID, accountID, "sync", "api", "pending", startedPending, time.Time{}, nil, `{"queued":true}`); err != nil {
		t.Fatal(err)
	}

	if err := repo.PromoteJobRunToRunning(ctx, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	row := db.QueryRowContext(ctx, `
		SELECT status, started_at, meta_json, finished_at FROM job_runs WHERE id = ?`, runID.String())
	var status, startedAt, meta string
	var finished sql.NullString
	if err := row.Scan(&status, &startedAt, &meta, &finished); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("status = %q want running", status)
	}
	if meta != `{"queued":true}` {
		t.Fatalf("meta_json should be preserved, got %q", meta)
	}
	if finished.Valid && finished.String != "" {
		t.Fatalf("finished_at should be null, got %v", finished.String)
	}
	parsedStart, err := parseTime(startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !parsedStart.Equal(startedPending) {
		t.Fatalf("started_at should stay enqueue time for pending promotion, got %v want %v", parsedStart, startedPending)
	}

	// Idempotent: second promote ok
	if err := repo.PromoteJobRunToRunning(ctx, runID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	fin := time.Now().UTC()
	_ = repo.UpdateJobRunStatus(ctx, runID, "success", &fin, nil, `{}`)
	if err := repo.PromoteJobRunToRunning(ctx, runID, time.Now().UTC()); err == nil {
		t.Fatal("Promote on terminal run should fail")
	}
}
