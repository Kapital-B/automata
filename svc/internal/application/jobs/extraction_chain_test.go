package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func extractionChain() []string {
	return []string{appjobs.TypeInterpretProject, appjobs.TypeReconcileProject}
}

// Extraction is a chain: interpret produces candidates and reconcile applies
// them. Running only the first half is why correspondence never became facts.
func TestExtractionChainIsValidAndOrdered(t *testing.T) {
	store := memoryjobs.NewStore()
	enq := &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()}
	ctx := context.Background()
	userID, projectID := uuid.New(), uuid.New()

	rec, err := enq.EnqueueChain(ctx, userID, nil, driven.JobTriggerAPI, extractionChain(),
		driven.JobPayload{ProjectID: &projectID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec.JobType != appjobs.TypeInterpretProject {
		t.Errorf("first step = %q, want interpret_project", rec.JobType)
	}
	if len(rec.RemainingJobs) != 1 || rec.RemainingJobs[0] != appjobs.TypeReconcileProject {
		t.Errorf("remaining = %v, want [reconcile_project]", rec.RemainingJobs)
	}
	if rec.Payload.ProjectID == nil || *rec.Payload.ProjectID != projectID {
		t.Errorf("payload project = %v, want %s", rec.Payload.ProjectID, projectID)
	}
}

// A burst of assignments must produce one extraction run, not one per item.
func TestExtractionCoalescesPerProject(t *testing.T) {
	store := memoryjobs.NewStore()
	enq := &appjobs.Enqueuer{Store: store, Registry: appjobs.DefaultRegistry()}
	ctx := context.Background()
	userID, projectID := uuid.New(), uuid.New()

	first, err := enq.EnqueueChain(ctx, userID, nil, driven.JobTriggerAPI, extractionChain(),
		driven.JobPayload{ProjectID: &projectID}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	conflicts := 0
	for i := 0; i < 9; i++ {
		if _, err := enq.EnqueueChain(ctx, userID, nil, driven.JobTriggerAPI, extractionChain(),
			driven.JobPayload{ProjectID: &projectID}, nil, nil); err != nil {
			if errors.Is(err, driven.ErrJobLockHeld) {
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
	if _, err := enq.EnqueueChain(ctx, userID, nil, driven.JobTriggerAPI, extractionChain(),
		driven.JobPayload{ProjectID: &other}, nil, nil); err != nil {
		t.Fatalf("a second project must not be blocked: %v", err)
	}
	_ = first
}

func TestExtractionLockIsKeyedByProject(t *testing.T) {
	reg := appjobs.DefaultRegistry()
	def := reg.MustGet(appjobs.TypeInterpretProject)
	if !def.RequiresLock {
		t.Fatal("interpret_project must take a lock so extraction coalesces")
	}
	if def.LockScope != "project" {
		t.Errorf("lock scope = %q, want project", def.LockScope)
	}
}
