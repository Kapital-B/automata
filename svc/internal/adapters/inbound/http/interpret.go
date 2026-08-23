package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) interpretProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.InterpretSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "interpret not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		AccountID           *string  `json:"account_id"`
		MessageIDs          []string `json:"message_ids"`
		ManualItemIDs       []string `json:"manual_item_ids"`
		ConnectorMessageIDs []string `json:"connector_message_ids"`
		Async               *bool    `json:"async"`
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if body.Async != nil && *body.Async {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "async interpret not supported in 2b"})
		return
	}
	in := appinterpret.RunInput{Trigger: "api"}
	if body.AccountID != nil && strings.TrimSpace(*body.AccountID) != "" {
		aid, err := uuid.Parse(*body.AccountID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
			return
		}
		in.AccountID = &aid
	}
	for _, idStr := range body.MessageIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
			return
		}
		in.MessageIDs = append(in.MessageIDs, id)
	}
	for _, idStr := range body.ManualItemIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad manual_item_id"})
			return
		}
		in.ManualItemIDs = append(in.ManualItemIDs, id)
	}
	for _, idStr := range body.ConnectorMessageIDs {
		id, err := uuid.Parse(strings.TrimSpace(idStr))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad connector_message_id"})
			return
		}
		in.ConnectorMessageIDs = append(in.ConnectorMessageIDs, id)
	}
	view, err := h.InterpretSvc.Run(r.Context(), uid, projectID, in)
	if err != nil {
		writeInterpretError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, interpretationJSON(*view))
}

func (h *Handlers) listProjectInterpretations(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.InterpretSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "interpret not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.InterpretSvc.ListPending(r.Context(), uid, projectID)
	if err != nil {
		writeInterpretError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, interpretationJSON(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) dismissInterpretation(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.InterpretSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "interpret not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.InterpretSvc.Dismiss(r.Context(), uid, id)
	if err != nil {
		writeInterpretError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, interpretationJSON(*view))
}

func writeInterpretError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appinterpret.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, appinterpret.ErrLLMUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm not configured"})
	case errors.Is(err, appinterpret.ErrMixedAccounts):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, appinterpret.ErrNothingToInterpret):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
	case errors.Is(err, appinterpret.ErrNotPending):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func interpretationJSON(v appinterpret.InterpretationView) map[string]any {
	row := v.Interpretation
	out := map[string]any{
		"id":              row.ID.String(),
		"organisation_id": row.OrganisationID.String(),
		"project_id":      row.ProjectID.String(),
		"status":          row.Status,
		"payload_json":    json.RawMessage(row.PayloadJSON),
		"reason":          row.Reason,
		"created_at":      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if row.AccountID != nil {
		out["account_id"] = row.AccountID.String()
	}
	if row.RunID != nil {
		out["run_id"] = row.RunID.String()
	}
	if row.Confidence != nil {
		out["confidence"] = *row.Confidence
	}
	sources := make([]map[string]any, 0, len(v.Sources))
	for _, src := range v.Sources {
		srow := map[string]any{"id": src.ID.String(), "interpretation_id": src.InterpretationID.String()}
		if src.MessageID != nil {
			srow["message_id"] = src.MessageID.String()
		}
		if src.ManualItemID != nil {
			srow["manual_item_id"] = src.ManualItemID.String()
		}
		if src.ConnectorMessageID != nil {
			srow["connector_message_id"] = src.ConnectorMessageID.String()
		}
		sources = append(sources, srow)
	}
	out["sources"] = sources
	candidates := make([]map[string]any, 0, len(v.Candidates))
	for _, c := range v.Candidates {
		crow := map[string]any{
			"kind":                  c.Kind,
			"confidence":            c.Confidence,
			"reason":                c.Reason,
			"message_ids":           c.MessageIDs,
			"manual_item_ids":       c.ManualItemIDs,
			"connector_message_ids": c.ConnectorMessageIDs,
		}
		if c.SubjectKey != "" {
			crow["subject_key"] = c.SubjectKey
		}
		if c.Label != "" {
			crow["label"] = c.Label
		}
		if c.Value != nil {
			crow["value"] = c.Value
		}
		if c.Unit != "" {
			crow["unit"] = c.Unit
		}
		if c.Statement != "" {
			crow["statement"] = c.Statement
		}
		candidates = append(candidates, crow)
	}
	out["candidates"] = candidates
	return out
}
