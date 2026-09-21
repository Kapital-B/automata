package http

import (
	"errors"
	"net/http"
	"time"

	appjobs "github.com/Kapital-B/automata/svc/internal/application/jobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// extractProject is the manual "Check now" verb. It queues the same chain the
// debounce queues, so the button and the automatic path cannot drift.
//
// A run already in flight is success, not conflict: the caller asked for the
// project to be up to date, and it is about to be.
func (h *Handlers) extractProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.JobEnqueuer == nil || h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "extraction not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	detail, err := h.ProjectSvc.Get(r.Context(), uid, projectID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if detail == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	pid := projectID
	rec, err := h.JobEnqueuer.Enqueue(r.Context(), driven.CreateJobInput{
		JobType:       appjobs.TypeInterpretProject,
		UserID:        uid,
		TriggerKind:   driven.JobTriggerAPI,
		RemainingJobs: []string{appjobs.TypeReconcileProject},
		Payload:       driven.JobPayload{ProjectID: &pid},
		Now:           time.Now().UTC(),
	})
	switch {
	case err == nil:
		out := map[string]any{"status": "queued"}
		if rec != nil {
			out["job_id"] = rec.ID.String()
			out["chain_id"] = rec.ChainID.String()
		}
		writeJSON(w, http.StatusAccepted, out)
	case errors.Is(err, driven.ErrJobLockHeld), errors.Is(err, driven.ErrJobConflict):
		// Already running or already queued — the caller's intent is satisfied.
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "already_running"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}
