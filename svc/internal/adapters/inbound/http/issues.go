package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listProjectIssues(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.IssueSvc.List(r.Context(), uid, projectID)
	if err != nil {
		if errors.Is(err, appissues.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.Log.Error("list issues", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, issueJSON(v, false))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) suggestProjectIssue(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	in := appissues.SuggestInput{}
	if v := strings.TrimSpace(r.URL.Query().Get("account_id")); v != "" {
		aid, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
			return
		}
		in.AccountID = &aid
	}
	res, err := h.IssueSvc.Suggest(r.Context(), uid, projectID, in)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	refs := make([]map[string]any, 0, len(res.ItemRefs))
	for _, ref := range res.ItemRefs {
		row := map[string]any{}
		if ref.MessageID != nil {
			row["message_id"] = ref.MessageID.String()
		}
		if ref.ManualItemID != nil {
			row["manual_item_id"] = ref.ManualItemID.String()
		}
		refs = append(refs, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":      res.Title,
		"item_refs":  refs,
		"confidence": res.Confidence,
		"reason":     res.Reason,
	})
}

func (h *Handlers) createProjectIssue(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Title               string `json:"title"`
		CurrentPositionNote string `json:"current_position_note"`
		AssigneeUserID      *string `json:"assignee_user_id"`
		AssigneeContactID   *string `json:"assignee_contact_id"`
		ItemRefs            []struct {
			MessageID    *string `json:"message_id"`
			ManualItemID *string `json:"manual_item_id"`
		} `json:"item_refs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := appissues.CreateInput{Title: body.Title, CurrentPositionNote: body.CurrentPositionNote}
	if body.AssigneeUserID != nil && strings.TrimSpace(*body.AssigneeUserID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.AssigneeUserID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_user_id"})
			return
		}
		in.AssigneeUserID = &id
	}
	if body.AssigneeContactID != nil && strings.TrimSpace(*body.AssigneeContactID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.AssigneeContactID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_contact_id"})
			return
		}
		in.AssigneeContactID = &id
	}
	for _, ref := range body.ItemRefs {
		ir := appissues.ItemRef{}
		if ref.MessageID != nil && strings.TrimSpace(*ref.MessageID) != "" {
			id, err := uuid.Parse(strings.TrimSpace(*ref.MessageID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
				return
			}
			ir.MessageID = &id
		}
		if ref.ManualItemID != nil && strings.TrimSpace(*ref.ManualItemID) != "" {
			id, err := uuid.Parse(strings.TrimSpace(*ref.ManualItemID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad manual_item_id"})
				return
			}
			ir.ManualItemID = &id
		}
		in.ItemRefs = append(in.ItemRefs, ir)
	}
	view, err := h.IssueSvc.Create(r.Context(), uid, projectID, in)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, issueJSON(*view, true))
}

func (h *Handlers) getIssue(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.IssueSvc.Get(r.Context(), uid, id)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issueJSON(*view, true))
}

func (h *Handlers) updateIssue(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := appissues.UpdateInput{}
	if raw, ok := body["title"]; ok {
		var title string
		_ = json.Unmarshal(raw, &title)
		in.Title = &title
	}
	if raw, ok := body["current_position_note"]; ok {
		var note string
		_ = json.Unmarshal(raw, &note)
		in.CurrentPositionNote = &note
	}
	if raw, ok := body["status"]; ok {
		var status string
		_ = json.Unmarshal(raw, &status)
		in.Status = &status
	}
	_, hasUser := body["assignee_user_id"]
	_, hasContact := body["assignee_contact_id"]
	if hasUser || hasContact {
		in.SetAssignee = true
		var userRaw, contactRaw *string
		if hasUser {
			var s *string
			_ = json.Unmarshal(body["assignee_user_id"], &s)
			userRaw = s
		}
		if hasContact {
			var s *string
			_ = json.Unmarshal(body["assignee_contact_id"], &s)
			contactRaw = s
		}
		userEmpty := userRaw == nil || strings.TrimSpace(*userRaw) == ""
		contactEmpty := contactRaw == nil || strings.TrimSpace(*contactRaw) == ""
		if userEmpty && contactEmpty {
			in.ClearAssignee = true
		} else {
			if !userEmpty {
				uidParsed, err := uuid.Parse(strings.TrimSpace(*userRaw))
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_user_id"})
					return
				}
				in.AssigneeUserID = &uidParsed
			}
			if !contactEmpty {
				cid, err := uuid.Parse(strings.TrimSpace(*contactRaw))
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad assignee_contact_id"})
					return
				}
				in.AssigneeContactID = &cid
			}
		}
	}
	view, err := h.IssueSvc.Update(r.Context(), uid, id, in)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issueJSON(*view, true))
}

func (h *Handlers) addIssueItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		MessageID    *string `json:"message_id"`
		ManualItemID *string `json:"manual_item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	ref := appissues.ItemRef{}
	if body.MessageID != nil && strings.TrimSpace(*body.MessageID) != "" {
		mid, err := uuid.Parse(strings.TrimSpace(*body.MessageID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
			return
		}
		ref.MessageID = &mid
	}
	if body.ManualItemID != nil && strings.TrimSpace(*body.ManualItemID) != "" {
		mid, err := uuid.Parse(strings.TrimSpace(*body.ManualItemID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad manual_item_id"})
			return
		}
		ref.ManualItemID = &mid
	}
	view, err := h.IssueSvc.AddItem(r.Context(), uid, id, ref)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issueJSON(*view, true))
}

func (h *Handlers) removeIssueItem(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.IssueSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "issues not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad item id"})
		return
	}
	view, err := h.IssueSvc.RemoveItem(r.Context(), uid, id, itemID)
	if err != nil {
		writeIssueError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issueJSON(*view, true))
}

func writeIssueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appissues.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, appissues.ErrDualAssignee):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot set both assignees"})
	case errors.Is(err, appissues.ErrItemConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "item already linked to an issue"})
	case errors.Is(err, appissues.ErrWrongProject):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "item not on this project"})
	case errors.Is(err, appissues.ErrInvalidItemRef):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid item ref"})
	case errors.Is(err, appissues.ErrInvalidStatus):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
	case errors.Is(err, appissues.ErrLLMUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llm not configured"})
	case errors.Is(err, appissues.ErrNothingToSuggest):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "no unassigned correspondence to suggest from"})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func issueJSON(v appissues.IssueView, withItems bool) map[string]any {
	iss := v.Issue
	out := map[string]any{
		"id":                     iss.ID.String(),
		"organisation_id":        iss.OrganisationID.String(),
		"project_id":             iss.ProjectID.String(),
		"title":                  iss.Title,
		"current_position_note":  iss.CurrentPositionNote,
		"status":                 iss.Status,
		"awaiting_me":            v.AwaitingMe,
		"created_at":             iss.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":             iss.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if iss.AssigneeUserID != nil {
		out["assignee_user_id"] = iss.AssigneeUserID.String()
	}
	if iss.AssigneeContactID != nil {
		out["assignee_contact_id"] = iss.AssigneeContactID.String()
	}
	if withItems {
		items := make([]map[string]any, 0, len(v.Items))
		for _, t := range v.Items {
			row := map[string]any{
				"id":         t.Item.ID.String(),
				"issue_id":   t.Item.IssueID.String(),
				"source":     t.Source,
				"title":      t.Title,
				"snippet":    t.BodySnippet,
				"added_at":   t.Item.AddedAt.UTC().Format(time.RFC3339Nano),
			}
			if t.OccurredAt != nil {
				row["occurred_at"] = t.OccurredAt.UTC().Format(time.RFC3339Nano)
			}
			if t.Item.MessageID != nil {
				row["message_id"] = t.Item.MessageID.String()
			}
			if t.Item.ManualItemID != nil {
				row["manual_item_id"] = t.Item.ManualItemID.String()
				row["channel"] = t.Channel
			}
			if t.AccountID != nil {
				row["account_id"] = t.AccountID.String()
			}
			items = append(items, row)
		}
		out["items"] = items
	}
	return out
}
