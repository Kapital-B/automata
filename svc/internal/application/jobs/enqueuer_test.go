package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func TestEnqueueMapsAliasesOnlyAtEnqueue(t *testing.T) {
	ctx := context.Background()
	store := memoryjobs.NewStore()
	userID := uuid.New()
	accountID := uuid.New()
	now := time.Now().UTC()

	enqueuer := Enqueuer{Store: store}
	job, err := enqueuer.Enqueue(ctx, driven.CreateJobInput{
		JobType:       "auto-draft",
		UserID:        userID,
		AccountID:     &accountID,
		TriggerKind:   driven.JobTriggerAPI,
		ChainID:       uuid.New(),
		RemainingJobs: []string{"forward"},
		Now:           now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.JobType != TypeDraftSuggest {
		t.Fatalf("expected alias to map to %q, got %q", TypeDraftSuggest, job.JobType)
	}
	if len(job.RemainingJobs) != 1 || job.RemainingJobs[0] != TypeForwardRules {
		t.Fatalf("expected remaining alias to map to %q, got %#v", TypeForwardRules, job.RemainingJobs)
	}
}

func TestEnqueueChainUsesDeterministicScheduleIDs(t *testing.T) {
	ctx := context.Background()
	store := memoryjobs.NewStore()
	userID := uuid.New()
	accountID := uuid.New()
	scheduleID := uuid.New()
	when := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)

	enqueuer := Enqueuer{Store: store}
	job, err := enqueuer.EnqueueChain(ctx, userID, &accountID, driven.JobTriggerSchedule, DefaultMailboxChain, driven.JobPayload{}, &scheduleID, &when)
	if err != nil {
		t.Fatal(err)
	}

	expectedChainID := DeterministicChainID(scheduleID, when.Format(time.RFC3339))
	expectedJobID := DeterministicScheduleJobID(scheduleID, when.Format(time.RFC3339), TypeSync)
	if job.ChainID != expectedChainID {
		t.Fatalf("expected deterministic chain id %s, got %s", expectedChainID, job.ChainID)
	}
	if job.ID != expectedJobID {
		t.Fatalf("expected deterministic job id %s, got %s", expectedJobID, job.ID)
	}
}
