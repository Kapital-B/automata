package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) reconcileProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ReconcileSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reconcile not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		InterpretationIDs []string `json:"interpretation_ids"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	in := appreconcile.ReconcileInput{}
	for _, idStr := range body.InterpretationIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad interpretation_id"})
			return
		}
		in.InterpretationIDs = append(in.InterpretationIDs, id)
	}
	res, err := h.ReconcileSvc.Run(r.Context(), uid, projectID, in)
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handlers) listProjectContradictions(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ReconcileSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reconcile not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, err := h.ReconcileSvc.ListContradictions(r.Context(), uid, projectID, status)
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, contradictionJSON(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) resolveContradiction(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ReconcileSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reconcile not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Resolution          string  `json:"resolution"`
		KeepFactVersionID   *string `json:"keep_fact_version_id"`
		RejectFactVersionID *string `json:"reject_fact_version_id"`
		ResolutionNote      string  `json:"resolution_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	in := appreconcile.ResolveInput{Resolution: body.Resolution, ResolutionNote: body.ResolutionNote}
	if body.KeepFactVersionID != nil && strings.TrimSpace(*body.KeepFactVersionID) != "" {
		kid, err := uuid.Parse(*body.KeepFactVersionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad keep_fact_version_id"})
			return
		}
		in.KeepFactVersionID = &kid
	}
	view, err := h.ReconcileSvc.Resolve(r.Context(), uid, id, in)
	if err != nil {
		writeReconcileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, contradictionJSON(*view))
}

func writeReconcileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appreconcile.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, appreconcile.ErrNothingToReconcile):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, appreconcile.ErrInvalidResolution), errors.Is(err, appreconcile.ErrNotOpen):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func contradictionJSON(v appreconcile.ContradictionView) map[string]any {
	c := v.Contradiction
	out := map[string]any{
		"id":              c.ID.String(),
		"organisation_id": c.OrganisationID.String(),
		"project_id":      c.ProjectID.String(),
		"status":          c.Status,
		"summary":         c.Summary,
		"created_at":      c.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      c.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if c.ResolutionNote != nil {
		out["resolution_note"] = *c.ResolutionNote
	}
	if c.ResolvedAt != nil {
		out["resolved_at"] = c.ResolvedAt.UTC().Format(time.RFC3339Nano)
	}
	if c.ResolvedByUserID != nil {
		out["resolved_by_user_id"] = c.ResolvedByUserID.String()
	}
	sides := make([]map[string]any, 0, len(v.Sides))
	for _, s := range v.Sides {
		row := map[string]any{"id": s.ID.String(), "contradiction_id": s.ContradictionID.String()}
		if s.FactVersionID != nil {
			row["fact_version_id"] = s.FactVersionID.String()
		}
		if s.DecisionID != nil {
			row["decision_id"] = s.DecisionID.String()
		}
		sides = append(sides, row)
	}
	out["sides"] = sides
	return out
}
