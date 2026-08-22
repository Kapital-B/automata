package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) askProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectAISvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ask not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	ans, err := h.ProjectAISvc.Ask(r.Context(), uid, projectID, strings.TrimSpace(body.Question))
	if err != nil {
		switch {
		case errors.Is(err, appprojectai.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		case errors.Is(err, appprojectai.ErrLLMUnavailable):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		case errors.Is(err, appprojectai.ErrEmptyQuestion):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, ans)
}
