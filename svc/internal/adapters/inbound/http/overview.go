package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	appoverview "github.com/Kapital-B/automata/svc/internal/application/overview"
	"github.com/google/uuid"
)

// overview serves the Home metric cards and project list in one request.
func (h *Handlers) overview(w http.ResponseWriter, r *http.Request) {
	// The auth middleware always injects a user id, falling back to the dev
	// DefaultUserID, so a missing token arrives as uuid.Nil rather than as a
	// missing value. Treat that as unauthenticated instead of querying for it.
	uid, ok := UserIDFromContext(r.Context())
	if !ok || uid == uuid.Nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.OverviewSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "overview not configured"})
		return
	}
	res, err := h.OverviewSvc.Get(r.Context(), uid)
	if err != nil {
		h.Log.Error("overview", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	projects := make([]map[string]any, 0, len(res.Projects))
	for _, p := range res.Projects {
		row := map[string]any{
			"id":              p.ID.String(),
			"code":            p.Code,
			"name":            p.Name,
			"attention_count": p.AttentionCount,
		}
		if p.LastActivityAt != nil {
			row["last_activity_at"] = p.LastActivityAt.UTC().Format(time.RFC3339Nano)
		}
		projects = append(projects, row)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"counts": map[string]any{
			"needs_you":           res.Counts.NeedsYou,
			"triage_unassigned":   res.Counts.TriageUnassigned,
			"triage_provisional":  res.Counts.TriageProvisional,
			"open_contradictions": res.Counts.OpenContradictions,
			"provisional_facts":   res.Counts.ProvisionalFacts,
			"proposed_decisions":  res.Counts.ProposedDecisions,
			"active_projects":     res.Counts.ActiveProjects,
		},
		"projects": projects,
	})
}

// activity serves the cross-project change feed.
func (h *Handlers) activity(w http.ResponseWriter, r *http.Request) {
	// The auth middleware always injects a user id, falling back to the dev
	// DefaultUserID, so a missing token arrives as uuid.Nil rather than as a
	// missing value. Treat that as unauthenticated instead of querying for it.
	uid, ok := UserIDFromContext(r.Context())
	if !ok || uid == uuid.Nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.OverviewSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "overview not configured"})
		return
	}

	in := appoverview.ActivityInput{}
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad limit"})
			return
		}
		in.Limit = n
	}
	if v := strings.TrimSpace(q.Get("before")); v != "" {
		at, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad before"})
			return
		}
		utc := at.UTC()
		in.Before = &utc
	}
	if v := strings.TrimSpace(q.Get("before_id")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad before_id"})
			return
		}
		in.BeforeID = &id
	}
	if v := strings.TrimSpace(q.Get("project_id")); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad project_id"})
			return
		}
		in.ProjectID = &id
	}
	// An unknown kind is rejected rather than ignored: silently returning
	// everything would look like the filter worked.
	for _, raw := range q["kind"] {
		for _, kind := range strings.Split(raw, ",") {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			if !appoverview.ValidKind(kind) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad kind: " + kind})
				return
			}
			in.Kinds = append(in.Kinds, kind)
		}
	}

	page, err := h.OverviewSvc.Activity(r.Context(), uid, in)
	if err != nil {
		h.Log.Error("activity", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	items := make([]map[string]any, 0, len(page.Items))
	for _, it := range page.Items {
		row := map[string]any{
			"kind":         it.Kind,
			"occurred_at":  it.OccurredAt.UTC().Format(time.RFC3339Nano),
			"project_id":   it.ProjectID.String(),
			"project_code": it.ProjectCode,
			"project_name": it.ProjectName,
			"title":        it.Title,
			"ref_type":     it.RefType,
			"ref_id":       it.RefID.String(),
		}
		if it.Source != "" {
			row["source"] = it.Source
		}
		items = append(items, row)
	}
	out := map[string]any{"items": items}
	if page.NextBefore != nil && page.NextID != nil {
		out["next_before"] = page.NextBefore.UTC().Format(time.RFC3339Nano)
		out["next_before_id"] = page.NextID.String()
	}
	writeJSON(w, http.StatusOK, out)
}
