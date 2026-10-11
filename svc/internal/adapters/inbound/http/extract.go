package http

import (
	"github.com/go-chi/chi/v5"
	"net/http"

	"github.com/google/uuid"
)

func (h *Handlers) extractProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.JobRequests == nil || h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "extraction not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	result, err := h.Extraction.SubmitExtraction(r.Context(), uid, id)
	if err != nil {
		writeOperationError(w, err)
		return
	}
	out := map[string]any{"status": "queued"}
	if result.AlreadyRunning {
		out["status"] = "already_running"
	} else if result.RunID != uuid.Nil {
		out["job_id"] = result.RunID.String()
		out["chain_id"] = result.ChainID.String()
	}
	writeJSON(w, http.StatusAccepted, out)
}
