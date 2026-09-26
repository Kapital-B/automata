package http

import (
	"errors"
	"net/http"
	"time"

	apptodos "github.com/Kapital-B/automata/svc/internal/application/todos"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// todoJSON renders a to-do for one member. Only the owner gets the account
// and message ids: the mail behind a shared to-do stays private.
func todoJSON(t apptodos.Todo) map[string]any {
	out := map[string]any{
		"id":            t.ID.String(),
		"text":          t.Text,
		"owner_user_id": t.OwnerUserID.String(),
		"owner_label":   t.OwnerLabel,
		"is_mine":       t.Mine,
		"created_at":    t.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if t.DueAt != nil {
		out["due_at"] = t.DueAt.UTC().Format(time.RFC3339Nano)
	}
	if t.IssueID != nil {
		out["issue_id"] = t.IssueID.String()
		out["issue_title"] = t.IssueTitle
	}
	if t.AccountID != nil && t.MessageID != nil {
		out["account_id"] = t.AccountID.String()
		out["message_id"] = t.MessageID.String()
	}
	return out
}

func (h *Handlers) listProjectTodos(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.TodoSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "to-dos not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	todos, err := h.TodoSvc.ForProject(r.Context(), uid, projectID)
	if err != nil {
		h.writeTodoError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(todos))
	for _, t := range todos {
		out = append(out, todoJSON(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) completeProjectTodo(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.TodoSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "to-dos not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	todoID, err := uuid.Parse(chi.URLParam(r, "todoID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad to-do id"})
		return
	}
	if err := h.TodoSvc.Complete(r.Context(), uid, projectID, todoID); err != nil {
		h.writeTodoError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "done"})
}

func (h *Handlers) writeTodoError(w http.ResponseWriter, err error) {
	if errors.Is(err, apptodos.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	h.Log.Error("to-dos", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
}
