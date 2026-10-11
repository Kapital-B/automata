package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"github.com/google/uuid"
)

func extractionChain() []string {
	return []string{domainjobs.TypeInterpretProject, domainjobs.TypeReconcileProject}
}

// Extraction is a chain: interpret produces candidates and reconcile applies
// them. Running only the first half is why correspondence never became facts.
func TestExtractionChainIsValidAndOrdered(t *testing.T) {
	store := memoryjobs.NewStore()
	enq := &appjobs.Enqueuer{Store: store, Registry: domainjobs.DefaultRegistry()}
	ctx := context.Background()
	userID, projectID := uuid.New(), uuid.New()

	rec, err := enq.EnqueueChain(ctx, userID, nil, domainjobs.JobTriggerAPI, extractionChain(),
		domainjobs.JobPayload{ProjectID: &projectID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec.JobType != domainjobs.TypeInterpretProject {
		t.Errorf("first step = %q, want interpret_project", rec.JobType)
	}
	if len(rec.RemainingJobs) != 1 || rec.RemainingJobs[0] != domainjobs.TypeReconcileProject {
		t.Errorf("remaining = %v, want [reconcile_project]", rec.RemainingJobs)
	}
	if rec.Payload.ProjectID == nil || *rec.Payload.ProjectID != projectID {
		t.Errorf("payload project = %v, want %s", rec.Payload.ProjectID, projectID)
	}
}

// A burst of assignments must produce one extraction run, not one per item.
func TestExtractionCoalescesPerProject(t *testing.T) {
	store := memoryjobs.NewStore()
	enq := &appjobs.Enqueuer{Store: store, Registry: domainjobs.DefaultRegistry()}
	ctx := context.Background()
	userID, projectID := uuid.New(), uuid.New()

	first, err := enq.EnqueueChain(ctx, userID, nil, domainjobs.JobTriggerAPI, extractionChain(),
		domainjobs.JobPayload{ProjectID: &projectID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	conflicts := 0
	for i := 0; i < 9; i++ {
		if _, err := enq.EnqueueChain(ctx, userID, nil, domainjobs.JobTriggerAPI, extractionChain(),
			domainjobs.JobPayload{ProjectID: &projectID}, nil, nil); err != nil {
			if errors.Is(err, domainjobs.ErrJobLockHeld) {
				conflicts++
				continue
			}
			t.Fatal(err)
		}
	}
	if conflicts != 9 {
		t.Fatalf("9 further assignments should have coalesced, got %d conflicts", conflicts)
	}

	// A different project is unaffected by the first project's lock.
	other := uuid.New()
	if _, err := enq.EnqueueChain(ctx, userID, nil, domainjobs.JobTriggerAPI, extractionChain(),
		domainjobs.JobPayload{ProjectID: &other}, nil, nil); err != nil {
		t.Fatalf("a second project must not be blocked: %v", err)
	}
	_ = first
}

func TestExtractionLockIsKeyedByProject(t *testing.T) {
	reg := domainjobs.DefaultRegistry()
	def := reg.MustGet(domainjobs.TypeInterpretProject)
	if !def.RequiresLock {
		t.Fatal("interpret_project must take a lock so extraction coalesces")
	}
	if def.LockScope != "project" {
		t.Errorf("lock scope = %q, want project", def.LockScope)
	}
}
