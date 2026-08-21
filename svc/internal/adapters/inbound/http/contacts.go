package http

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listContacts(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ContactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contacts not configured"})
		return
	}
	filter := driven.ContactListFilter{Query: strings.TrimSpace(r.URL.Query().Get("q"))}
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
	rows, err := h.ContactSvc.List(r.Context(), uid, filter)
	if err != nil {
		h.Log.Error("list contacts", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		item := map[string]any{
			"id":              c.ID.String(),
			"organisation_id": c.OrganisationID.String(),
			"display_name":    c.DisplayName,
			"created_at":      c.CreatedAt.UTC().Format(time.RFC3339Nano),
			"updated_at":      c.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}
		if c.Company != nil {
			item["company"] = *c.Company
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getContact(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ContactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contacts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	detail, err := h.ContactSvc.Get(r.Context(), uid, id)
	if err != nil {
		h.Log.Error("get contact", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if detail == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	c := detail.Contact
	idents := make([]map[string]any, 0, len(detail.Identities))
	for _, i := range detail.Identities {
		idents = append(idents, map[string]any{
			"id":               i.ID.String(),
			"kind":             i.Kind,
			"value_normalized": i.ValueNormalized,
			"value_raw":        i.ValueRaw,
			"created_at":       i.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	recent := make([]map[string]string, 0, len(detail.RecentMessages))
	for _, m := range detail.RecentMessages {
		recent = append(recent, map[string]string{
			"message_id": m.MessageID.String(),
			"account_id": m.AccountID.String(),
		})
	}
	suggestions := make([]map[string]any, 0, len(detail.SuggestedMerges))
	for _, s := range detail.SuggestedMerges {
		suggestions = append(suggestions, map[string]any{
			"id":           s.ID.String(),
			"display_name": s.DisplayName,
		})
	}
	resp := map[string]any{
		"id":               c.ID.String(),
		"organisation_id":  c.OrganisationID.String(),
		"display_name":     c.DisplayName,
		"created_at":       c.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":       c.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"identities":       idents,
		"recent_messages":  recent,
		"suggested_merges": suggestions,
	}
	if c.Company != nil {
		resp["company"] = *c.Company
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handlers) createContact(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ContactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contacts not configured"})
		return
	}
	var body struct {
		DisplayName string  `json:"display_name"`
		Company     *string `json:"company"`
		Identities  []struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		} `json:"identities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := appcontacts.CreateContactInput{
		DisplayName: body.DisplayName,
		Company:     body.Company,
	}
	for _, ident := range body.Identities {
		in.Identities = append(in.Identities, struct {
			Kind  string
			Value string
		}{Kind: ident.Kind, Value: ident.Value})
	}
	row, err := h.ContactSvc.Create(r.Context(), uid, in)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity already exists"})
			return
		}
		h.Log.Error("create contact", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              row.ID.String(),
		"organisation_id": row.OrganisationID.String(),
		"display_name":    row.DisplayName,
		"company":         row.Company,
		"created_at":      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
}

func (h *Handlers) addContactIdentity(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ContactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contacts not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row, err := h.ContactSvc.AddIdentity(r.Context(), uid, id, body.Kind, body.Value)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "identity already exists"})
			return
		}
		if strings.Contains(msg, "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":               row.ID.String(),
		"kind":             row.Kind,
		"value_normalized": row.ValueNormalized,
		"value_raw":        row.ValueRaw,
		"created_at":       row.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
}

func (h *Handlers) mergeContacts(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.ContactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contacts not configured"})
		return
	}
	survivorID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		SourceContactID string `json:"source_contact_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sourceID, err := uuid.Parse(strings.TrimSpace(body.SourceContactID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad source_contact_id"})
		return
	}
	if err := h.ContactSvc.Merge(r.Context(), uid, survivorID, sourceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "unique") || strings.Contains(msg, "conflict") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
