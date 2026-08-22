package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listProjectDecisions(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.DecisionSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "decisions not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	rows, err := h.DecisionSvc.List(r.Context(), uid, projectID, status)
	if err != nil {
		writeDecisionError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, decisionJSON(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) createProjectDecision(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.DecisionSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "decisions not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Statement           string `json:"statement"`
		Confirm             bool   `json:"confirm"`
		IssueID             *string `json:"issue_id"`
		AssigneeUserID      *string `json:"assignee_user_id"`
		AssigneeContactID   *string `json:"assignee_contact_id"`
		SupersedesDecisionID *string `json:"supersedes_decision_id"`
		Evidence            []struct {
			MessageID    *string `json:"message_id"`
			ManualItemID *string `json:"manual_item_id"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	in := appdecisions.CreateInput{Statement: body.Statement, Confirm: body.Confirm}
	if body.IssueID != nil && strings.TrimSpace(*body.IssueID) != "" {
		id, err := uuid.Parse(*body.IssueID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad issue_id"})
			return
		}
		in.IssueID = &id
	}
	if body.AssigneeUserID != nil && strings.TrimSpace(*body.AssigneeUserID) != "" {
		id, err := uuid.Parse(*body.AssigneeUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_user_id"})
			return
		}
		in.AssigneeUserID = &id
	}
	if body.AssigneeContactID != nil && strings.TrimSpace(*body.AssigneeContactID) != "" {
		id, err := uuid.Parse(*body.AssigneeContactID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_contact_id"})
			return
		}
		in.AssigneeContactID = &id
	}
	if body.SupersedesDecisionID != nil && strings.TrimSpace(*body.SupersedesDecisionID) != "" {
		id, err := uuid.Parse(*body.SupersedesDecisionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad supersedes_decision_id"})
			return
		}
		in.SupersedesID = &id
	}
	for _, e := range body.Evidence {
		ref := appdecisions.EvidenceRef{}
		if e.MessageID != nil && strings.TrimSpace(*e.MessageID) != "" {
			id, err := uuid.Parse(*e.MessageID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad evidence message_id"})
				return
			}
			ref.MessageID = &id
		}
		if e.ManualItemID != nil && strings.TrimSpace(*e.ManualItemID) != "" {
			id, err := uuid.Parse(*e.ManualItemID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad evidence manual_item_id"})
				return
			}
			ref.ManualItemID = &id
		}
		in.Evidence = append(in.Evidence, ref)
	}
	view, err := h.DecisionSvc.Create(r.Context(), uid, projectID, in)
	if err != nil {
		writeDecisionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, decisionJSON(*view))
}

func (h *Handlers) confirmDecision(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.DecisionSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "decisions not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.DecisionSvc.Confirm(r.Context(), uid, id)
	if err != nil {
		writeDecisionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisionJSON(*view))
}

func (h *Handlers) withdrawDecision(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.DecisionSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "decisions not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.DecisionSvc.Withdraw(r.Context(), uid, id)
	if err != nil {
		writeDecisionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisionJSON(*view))
}

func (h *Handlers) patchDecision(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.DecisionSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "decisions not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Statement         *string `json:"statement"`
		AssigneeUserID    *string `json:"assignee_user_id"`
		AssigneeContactID *string `json:"assignee_contact_id"`
		ClearAssignee     bool    `json:"clear_assignee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	in := appdecisions.PatchInput{Statement: body.Statement, ClearAssignee: body.ClearAssignee}
	if body.AssigneeUserID != nil && strings.TrimSpace(*body.AssigneeUserID) != "" {
		uid2, err := uuid.Parse(*body.AssigneeUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_user_id"})
			return
		}
		in.AssigneeUserID = &uid2
	}
	if body.AssigneeContactID != nil && strings.TrimSpace(*body.AssigneeContactID) != "" {
		cid, err := uuid.Parse(*body.AssigneeContactID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_contact_id"})
			return
		}
		in.AssigneeContactID = &cid
	}
	view, err := h.DecisionSvc.Patch(r.Context(), uid, id, in)
	if err != nil {
		writeDecisionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, decisionJSON(*view))
}

func writeDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appdecisions.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, appdecisions.ErrNotProposed), errors.Is(err, appdecisions.ErrInvalidStatus),
		errors.Is(err, appdecisions.ErrBadAssignee), errors.Is(err, appdecisions.ErrBadEvidence),
		errors.Is(err, appdecisions.ErrWrongProject):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func decisionJSON(v appdecisions.DecisionView) map[string]any {
	d := v.Decision
	out := map[string]any{
		"id":              d.ID.String(),
		"organisation_id": d.OrganisationID.String(),
		"project_id":      d.ProjectID.String(),
		"statement":       d.Statement,
		"status":          d.Status,
		"source":          d.Source,
		"created_at":      d.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      d.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if d.IssueID != nil {
		out["issue_id"] = d.IssueID.String()
	}
	if d.DecidedAt != nil {
		out["decided_at"] = d.DecidedAt.UTC().Format(time.RFC3339Nano)
	}
	if d.AssigneeUserID != nil {
		out["assignee_user_id"] = d.AssigneeUserID.String()
	}
	if d.AssigneeContactID != nil {
		out["assignee_contact_id"] = d.AssigneeContactID.String()
	}
	if d.Confidence != nil {
		out["confidence"] = *d.Confidence
	}
	if d.SupersedesDecisionID != nil {
		out["supersedes_decision_id"] = d.SupersedesDecisionID.String()
	}
	if d.CreatedByUserID != nil {
		out["created_by_user_id"] = d.CreatedByUserID.String()
	}
	ev := make([]map[string]any, 0, len(v.Evidence))
	for _, e := range v.Evidence {
		row := map[string]any{"id": e.ID.String(), "decision_id": e.DecisionID.String(), "added_at": e.AddedAt.UTC().Format(time.RFC3339Nano)}
		if e.MessageID != nil {
			row["message_id"] = e.MessageID.String()
		}
		if e.ManualItemID != nil {
			row["manual_item_id"] = e.ManualItemID.String()
		}
		ev = append(ev, row)
	}
	out["evidence"] = ev
	return out
}
