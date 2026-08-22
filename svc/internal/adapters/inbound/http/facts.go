package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listProjectFacts(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	include := appfacts.ListInclude{}
	for _, part := range strings.Split(r.URL.Query().Get("include"), ",") {
		switch strings.TrimSpace(strings.ToLower(part)) {
		case "proposed":
			include.Proposed = true
		case "history":
			include.History = true
		}
	}
	rows, err := h.FactSvc.List(r.Context(), uid, projectID, include)
	if err != nil {
		writeFactError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, factJSON(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getProjectCurrentPosition(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	pos, err := h.FactSvc.CurrentPosition(r.Context(), uid, projectID)
	if err != nil {
		writeFactError(w, err)
		return
	}
	facts := make([]map[string]any, 0, len(pos.Facts))
	for _, f := range pos.Facts {
		row := map[string]any{
			"fact_id":        f.FactID.String(),
			"subject_key":    f.SubjectKey,
			"label":          f.Label,
			"version_id":     f.VersionID.String(),
			"value_json":     json.RawMessage(f.ValueJSON),
			"value_text":     f.ValueText,
			"evidence_count": f.EvidenceCount,
		}
		if f.Unit != nil {
			row["unit"] = *f.Unit
		}
		facts = append(facts, row)
	}
	decisions := make([]map[string]any, 0)
	if h.DecisionSvc != nil {
		views, err := h.DecisionSvc.ListAcceptedRecent(r.Context(), uid, projectID, 10)
		if err != nil {
			writeFactError(w, err)
			return
		}
		for _, v := range views {
			d := v.Decision
			row := map[string]any{
				"decision_id":    d.ID.String(),
				"statement":      d.Statement,
				"status":         d.Status,
				"evidence_count": len(v.Evidence),
			}
			if d.DecidedAt != nil {
				row["decided_at"] = d.DecidedAt.UTC().Format(time.RFC3339Nano)
			}
			decisions = append(decisions, row)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"facts":     facts,
		"decisions": decisions,
	})
}

func (h *Handlers) createProjectFact(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		SubjectKey          string          `json:"subject_key"`
		Label               string          `json:"label"`
		Value               json.RawMessage `json:"value"`
		Unit                *string         `json:"unit"`
		IssueID             *string         `json:"issue_id"`
		Confirm             bool            `json:"confirm"`
		SupersedesVersionID *string         `json:"supersedes_version_id"`
		Evidence            []struct {
			MessageID    *string `json:"message_id"`
			ManualItemID *string `json:"manual_item_id"`
		} `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	var value any
	if len(body.Value) > 0 {
		if err := json.Unmarshal(body.Value, &value); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad value"})
			return
		}
	}
	in := appfacts.CreateInput{
		SubjectKey: body.SubjectKey,
		Label:      body.Label,
		Value:      value,
		Unit:       body.Unit,
		Confirm:    body.Confirm,
	}
	if body.IssueID != nil && strings.TrimSpace(*body.IssueID) != "" {
		id, err := uuid.Parse(*body.IssueID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad issue_id"})
			return
		}
		in.IssueID = &id
	}
	if body.SupersedesVersionID != nil && strings.TrimSpace(*body.SupersedesVersionID) != "" {
		id, err := uuid.Parse(*body.SupersedesVersionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad supersedes_version_id"})
			return
		}
		in.SupersedesVersionID = &id
	}
	for _, ev := range body.Evidence {
		ref := appfacts.EvidenceRef{}
		if ev.MessageID != nil && strings.TrimSpace(*ev.MessageID) != "" {
			id, err := uuid.Parse(*ev.MessageID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
				return
			}
			ref.MessageID = &id
		}
		if ev.ManualItemID != nil && strings.TrimSpace(*ev.ManualItemID) != "" {
			id, err := uuid.Parse(*ev.ManualItemID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad manual_item_id"})
				return
			}
			ref.ManualItemID = &id
		}
		in.Evidence = append(in.Evidence, ref)
	}
	view, err := h.FactSvc.Create(r.Context(), uid, projectID, in)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, factJSON(*view))
}

func (h *Handlers) getFact(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.FactSvc.Get(r.Context(), uid, id)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, factJSON(*view))
}

func (h *Handlers) confirmFactVersion(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		SupersedesVersionID *string `json:"supersedes_version_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	in := appfacts.ConfirmInput{}
	if body.SupersedesVersionID != nil && strings.TrimSpace(*body.SupersedesVersionID) != "" {
		sid, err := uuid.Parse(*body.SupersedesVersionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad supersedes_version_id"})
			return
		}
		in.SupersedesVersionID = &sid
	}
	view, err := h.FactSvc.Confirm(r.Context(), uid, id, in)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, factJSON(*view))
}

func (h *Handlers) rejectFactVersion(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	view, err := h.FactSvc.Reject(r.Context(), uid, id)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, factJSON(*view))
}

func (h *Handlers) addFactEvidence(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	ref := appfacts.EvidenceRef{}
	if body.MessageID != nil && strings.TrimSpace(*body.MessageID) != "" {
		mid, err := uuid.Parse(*body.MessageID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
			return
		}
		ref.MessageID = &mid
	}
	if body.ManualItemID != nil && strings.TrimSpace(*body.ManualItemID) != "" {
		mid, err := uuid.Parse(*body.ManualItemID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad manual_item_id"})
			return
		}
		ref.ManualItemID = &mid
	}
	view, err := h.FactSvc.AddEvidence(r.Context(), uid, id, ref)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, factJSON(*view))
}

func (h *Handlers) removeFactEvidence(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.FactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "facts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	evidenceID, err := uuid.Parse(chi.URLParam(r, "evidenceID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad evidence id"})
		return
	}
	view, err := h.FactSvc.RemoveEvidence(r.Context(), uid, id, evidenceID)
	if err != nil {
		writeFactError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, factJSON(*view))
}

func writeFactError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appfacts.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, appfacts.ErrSupersedeRequired), errors.Is(err, appfacts.ErrInvalidSupersedes):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, appfacts.ErrInvalidSubjectKey),
		errors.Is(err, appfacts.ErrInvalidStatus),
		errors.Is(err, appfacts.ErrInvalidEvidence),
		errors.Is(err, appfacts.ErrWrongProject),
		errors.Is(err, appfacts.ErrNotProposed):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func factJSON(v appfacts.FactView) map[string]any {
	f := v.Fact
	out := map[string]any{
		"id":              f.ID.String(),
		"organisation_id": f.OrganisationID.String(),
		"project_id":      f.ProjectID.String(),
		"subject_key":     f.SubjectKey,
		"label":           f.Label,
		"created_at":      f.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      f.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if f.IssueID != nil {
		out["issue_id"] = f.IssueID.String()
	}
	versions := make([]map[string]any, 0, len(v.Versions))
	for _, vv := range v.Versions {
		ver := vv.Version
		row := map[string]any{
			"id":         ver.ID.String(),
			"fact_id":    ver.FactID.String(),
			"status":     ver.Status,
			"value_json": json.RawMessage(ver.ValueJSON),
			"value_text": ver.ValueText,
			"source":     ver.Source,
			"created_at": ver.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if ver.Unit != nil {
			row["unit"] = *ver.Unit
		}
		if ver.Confidence != nil {
			row["confidence"] = *ver.Confidence
		}
		if ver.SupersedesVersionID != nil {
			row["supersedes_version_id"] = ver.SupersedesVersionID.String()
		}
		if ver.SupersededByVersionID != nil {
			row["superseded_by_version_id"] = ver.SupersededByVersionID.String()
		}
		if ver.SupersededAt != nil {
			row["superseded_at"] = ver.SupersededAt.UTC().Format(time.RFC3339Nano)
		}
		if ver.CreatedByUserID != nil {
			row["created_by_user_id"] = ver.CreatedByUserID.String()
		}
		evidence := make([]map[string]any, 0, len(vv.Evidence))
		for _, ev := range vv.Evidence {
			erow := map[string]any{
				"id":              ev.ID.String(),
				"fact_version_id": ev.FactVersionID.String(),
				"added_at":        ev.AddedAt.UTC().Format(time.RFC3339Nano),
			}
			if ev.MessageID != nil {
				erow["message_id"] = ev.MessageID.String()
			}
			if ev.ManualItemID != nil {
				erow["manual_item_id"] = ev.ManualItemID.String()
			}
			evidence = append(evidence, erow)
		}
		row["evidence"] = evidence
		versions = append(versions, row)
	}
	out["versions"] = versions
	return out
}
