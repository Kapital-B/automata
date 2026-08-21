package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	filter := driven.ProjectListFilter{IncludeArchived: r.URL.Query().Get("include_archived") == "true"}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad limit"})
			return
		}
		filter.Limit = n
	}
	rows, err := h.ProjectSvc.List(r.Context(), uid, filter)
	if err != nil {
		h.Log.Error("list projects", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, p := range rows {
		out = append(out, projectJSON(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	detail, err := h.ProjectSvc.Get(r.Context(), uid, id)
	if err != nil {
		h.Log.Error("get project", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if detail == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	resp := projectJSON(detail.Project)
	if detail.Member != nil {
		resp["member"] = memberJSON(*detail.Member)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) createProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	var body struct {
		Name        string   `json:"name"`
		Code        string   `json:"code"`
		Description *string  `json:"description"`
		Client      *string  `json:"client"`
		Keywords    []string `json:"keywords"`
		Member      *struct {
			Role              string  `json:"role"`
			Discipline        *string `json:"discipline"`
			Responsibilities  *string `json:"responsibilities"`
			CurrentScope      *string `json:"current_scope"`
			ApprovalAuthority *string `json:"approval_authority"`
			OutOfScope        *string `json:"out_of_scope"`
		} `json:"member"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := appprojects.CreateProjectInput{
		Name: body.Name, Code: body.Code, Description: body.Description,
		Client: body.Client, Keywords: body.Keywords,
	}
	if body.Member != nil {
		in.MemberRole = body.Member.Role
		in.Discipline = body.Member.Discipline
		in.Responsibilities = body.Member.Responsibilities
		in.CurrentScope = body.Member.CurrentScope
		in.ApprovalAuthority = body.Member.ApprovalAuthority
		in.OutOfScope = body.Member.OutOfScope
	}
	row, err := h.ProjectSvc.Create(r.Context(), uid, in)
	if err != nil {
		switch {
		case errors.Is(err, appprojects.ErrInvalidCode):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project code"})
		case errors.Is(err, appprojects.ErrCodeTaken):
			writeJSON(w, http.StatusConflict, map[string]string{"error": "project code already exists"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusCreated, projectJSON(*row))
}

func (h *Handlers) updateProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
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
	if _, hasCode := body["code"]; hasCode {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "project code is immutable"})
		return
	}
	in := appprojects.UpdateProjectInput{}
	if raw, ok := body["name"]; ok {
		var name string
		_ = json.Unmarshal(raw, &name)
		in.Name = &name
	}
	if raw, ok := body["description"]; ok {
		var desc *string
		_ = json.Unmarshal(raw, &desc)
		in.Description = desc
	}
	if raw, ok := body["client"]; ok {
		var client *string
		_ = json.Unmarshal(raw, &client)
		in.Client = client
	}
	if raw, ok := body["keywords"]; ok {
		var kw []string
		_ = json.Unmarshal(raw, &kw)
		in.Keywords = &kw
	}
	if raw, ok := body["archived"]; ok {
		var archived bool
		_ = json.Unmarshal(raw, &archived)
		in.Archive = &archived
	}
	row, err := h.ProjectSvc.Update(r.Context(), uid, id, in)
	if err != nil {
		if errors.Is(err, appprojects.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, projectJSON(*row))
}

func (h *Handlers) updateProjectMember(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Role              *string `json:"role"`
		Discipline        *string `json:"discipline"`
		Responsibilities  *string `json:"responsibilities"`
		CurrentScope      *string `json:"current_scope"`
		ApprovalAuthority *string `json:"approval_authority"`
		OutOfScope        *string `json:"out_of_scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	m, err := h.ProjectSvc.UpdateMember(r.Context(), uid, id, appprojects.UpdateMemberInput{
		Role: body.Role, Discipline: body.Discipline, Responsibilities: body.Responsibilities,
		CurrentScope: body.CurrentScope, ApprovalAuthority: body.ApprovalAuthority, OutOfScope: body.OutOfScope,
	})
	if err != nil {
		if errors.Is(err, appprojects.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, memberJSON(*m))
}

func (h *Handlers) unassignedSummary(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	sum, err := h.ProjectSvc.UnassignedSummary(r.Context(), uid)
	if err != nil {
		h.Log.Error("unassigned summary", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unassigned": sum.Unassigned, "provisional": sum.Provisional})
}

func (h *Handlers) listUnassigned(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	filter := driven.UnassignedListFilter{Status: strings.TrimSpace(r.URL.Query().Get("status"))}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad limit"})
			return
		}
		filter.Limit = n
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad offset"})
			return
		}
		filter.Offset = n
	}
	items, err := h.ProjectSvc.ListUnassigned(r.Context(), uid, filter)
	if err != nil {
		h.Log.Error("list unassigned", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"kind":          "message",
			"message_id":    it.MessageID.String(),
			"account_id":    it.AccountID.String(),
			"account_label": it.AccountLabel,
			"subject":       it.Subject,
			"from_json":     jsonRawOrObject(it.FromJSON),
			"received_at":   it.ReceivedAt.UTC().Format(time.RFC3339Nano),
			"status":        it.Status,
			"reason":        it.Reason,
			"source":        it.Source,
		}
		if it.ConversationID != nil {
			row["conversation_id"] = *it.ConversationID
		}
		if it.ProjectID != nil {
			row["project_id"] = it.ProjectID.String()
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) assignMessageProject(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		ProjectID *string `json:"project_id"`
		Scope     string  `json:"scope"`
		Status    string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := appprojects.AssignInput{
		Scope:  domainprojects.AssignScope(strings.TrimSpace(body.Scope)),
		Status: domainprojects.AssignmentStatus(strings.TrimSpace(body.Status)),
	}
	if body.ProjectID != nil && strings.TrimSpace(*body.ProjectID) != "" {
		pid, err := uuid.Parse(strings.TrimSpace(*body.ProjectID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad project_id"})
			return
		}
		in.ProjectID = &pid
	}
	eff, err := h.ProjectSvc.AssignMessage(r.Context(), uid, id, in)
	if err != nil {
		switch {
		case errors.Is(err, appprojects.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		case errors.Is(err, appprojects.ErrConversationRequired):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conversation_required"})
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, effectiveJSON(eff))
}

func (h *Handlers) clearMessageOverride(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ProjectSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	eff, err := h.ProjectSvc.ClearOverride(r.Context(), uid, id)
	if err != nil {
		if errors.Is(err, appprojects.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, effectiveJSON(eff))
}

func projectJSON(p driven.ProjectRow) map[string]any {
	m := map[string]any{
		"id":              p.ID.String(),
		"organisation_id": p.OrganisationID.String(),
		"name":            p.Name,
		"code":            p.Code,
		"keywords":        p.Keywords,
		"created_at":      p.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if p.Description != nil {
		m["description"] = *p.Description
	}
	if p.Client != nil {
		m["client"] = *p.Client
	}
	if p.ArchivedAt != nil {
		m["archived_at"] = p.ArchivedAt.UTC().Format(time.RFC3339Nano)
	}
	return m
}

func memberJSON(m driven.ProjectMemberRow) map[string]any {
	out := map[string]any{
		"id":         m.ID.String(),
		"project_id": m.ProjectID.String(),
		"user_id":    m.UserID.String(),
		"role":       m.Role,
		"created_at": m.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": m.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if m.Discipline != nil {
		out["discipline"] = *m.Discipline
	}
	if m.Responsibilities != nil {
		out["responsibilities"] = *m.Responsibilities
	}
	if m.CurrentScope != nil {
		out["current_scope"] = *m.CurrentScope
	}
	if m.ApprovalAuthority != nil {
		out["approval_authority"] = *m.ApprovalAuthority
	}
	if m.OutOfScope != nil {
		out["out_of_scope"] = *m.OutOfScope
	}
	return out
}

func effectiveJSON(eff *driven.EffectiveAssignment) map[string]any {
	if eff == nil {
		return map[string]any{"status": "unassigned"}
	}
	out := map[string]any{
		"status":     eff.Status,
		"reason":     eff.Reason,
		"source":     eff.Source,
		"scope":      eff.Scope,
		"account_id": eff.AccountID.String(),
		"message_id": eff.MessageID.String(),
	}
	if eff.ProjectID != nil {
		out["project_id"] = eff.ProjectID.String()
	}
	if eff.ConversationID != nil {
		out["conversation_id"] = *eff.ConversationID
	}
	return out
}

func jsonRawOrObject(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return map[string]any{}
	}
	return v
}
