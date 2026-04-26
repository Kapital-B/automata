package http

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handlers holds wired application services for HTTP.
type Handlers struct {
	Log         *slog.Logger
	AccountSvc  *appaccounts.Service
	SyncSvc     *appmessages.SyncService
	CategorizeSvc *appmessages.CategorizeService
	AuthSvc     *auth.Service
	Accounts    driven.AccountRepository
	Messages    driven.MessageRepository
	JobRuns     driven.JobRunRepository
	OAuthStates driven.OAuthStateRepository
	Users       driven.UserRepository
	Dashboard   string
	SuccessPath string
	ErrorPath   string
	AuthSuccessPath string
	AuthErrorPath   string
	StateTTL    time.Duration
	JWTSecret      []byte
	JWTTTL         time.Duration
	DefaultUserID  uuid.UUID // dev fallback when no Bearer token
}

func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Use(optionalAuthMiddleware(h.JWTSecret, h.DefaultUserID))
	r.Get("/api/health", h.health)

	r.Post("/api/auth/register", h.register)
	r.Post("/api/auth/login", h.loginPassword)
	r.Post("/api/auth/refresh", h.authRefresh)
	r.Get("/api/auth/microsoft", h.authMicrosoftStart)
	r.Get("/api/auth/microsoft/callback", h.authMicrosoftCallback)
	r.Get("/api/auth/google", h.authGoogleStart)
	r.Get("/api/auth/google/callback", h.authGoogleCallback)
	r.Get("/api/me", h.me)

	r.Get("/api/accounts", h.listAccounts)
	r.Post("/api/accounts", h.startConnect)
	r.Get("/api/accounts/callback", h.oauthCallback)
	r.Get("/api/accounts/{id}", h.getAccount)
	r.Delete("/api/accounts/{id}", h.deleteAccount)
	r.Post("/api/accounts/{id}/sync", h.syncAccount)
	r.Post("/api/accounts/{id}/categorize", h.categorizeAccount)
	r.Get("/api/categories", h.listCategories)
	r.Get("/api/runs", h.listRuns)
	r.Get("/api/runs/{id}", h.getRun)
	r.Get("/api/messages", h.listMessages)
	r.Get("/api/messages/{id}", h.getMessage)
	return r
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	uid := userIDOrEmpty(r)
	rows, err := h.Accounts.ListAccounts(r.Context(), uid)
	if err != nil {
		h.Log.Error("list accounts", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type item struct {
		ID                 string  `json:"id"`
		Label              string  `json:"label"`
		Provider           string  `json:"provider"`
		MsAccountKind      string  `json:"ms_account_kind"`
		PrimaryEmail       string  `json:"primary_email"`
		ConnectionStatus   string  `json:"connection_status"`
		LastError          *string `json:"last_error,omitempty"`
		LastSyncedAt       *string `json:"last_synced_at,omitempty"`
	}
	out := make([]item, 0, len(rows))
	for _, a := range rows {
		it := item{
			ID:               a.ID.String(),
			Label:            a.Label,
			Provider:         a.Provider,
			MsAccountKind:    string(a.MsAccountKind),
			PrimaryEmail:     a.PrimaryEmail,
			ConnectionStatus: a.ConnectionStatus,
			LastError:        a.LastError,
		}
		if a.LastSyncedAt != nil {
			s := a.LastSyncedAt.UTC().Format(time.RFC3339Nano)
			it.LastSyncedAt = &s
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

type startConnectBody struct {
	Provider      string  `json:"provider"`
	MsAccountKind string  `json:"ms_account_kind"`
	Label         *string `json:"label"`
}

func (h *Handlers) startConnect(w http.ResponseWriter, r *http.Request) {
	if h.AccountSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account service not configured"})
		return
	}
	var body startConnectBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	kind := domainacc.MsAccountKind(body.MsAccountKind)
	res, err := h.AccountSvc.StartConnect(r.Context(), userIDOrEmpty(r), appaccounts.StartConnectInput{
		Provider:      body.Provider,
		MsAccountKind: kind,
		LabelHint:     body.Label,
	})
	if err != nil {
		h.Log.Warn("start connect", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_url": res.AuthorizationURL,
		"state":             res.State,
	})
}

func (h *Handlers) oauthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		code := oauthErrorCode(errParam, q.Get("error_subcode"))
		h.redirectError(w, r, code)
		return
	}
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		h.redirectError(w, r, "invalid_state")
		return
	}
	res, err := h.AccountSvc.CompleteOAuth(r.Context(), code, state)
	if err != nil {
		if errors.Is(err, appaccounts.ErrInvalidOAuthState) {
			h.redirectError(w, r, "invalid_state")
			return
		}
		h.Log.Error("oauth complete", "err", err)
		h.redirectError(w, r, "token_exchange_failed")
		return
	}
	target := h.Dashboard + h.SuccessPath + "?account_id=" + url.QueryEscape(res.AccountID.String())
	http.Redirect(w, r, target, http.StatusFound)
}

func oauthErrorCode(errParam, sub string) string {
	switch errParam {
	case "access_denied":
		if strings.Contains(strings.ToLower(sub+" "+errParam), "admin") {
			return "admin_consent_required"
		}
		return "access_denied"
	default:
		return "access_denied"
	}
}

func (h *Handlers) redirectError(w http.ResponseWriter, r *http.Request, code string) {
	target := h.Dashboard + h.ErrorPath + "?code=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handlers) getAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	row, _, err := h.Accounts.GetAccount(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		h.Log.Error("get account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var lastSync *string
	if row.LastSyncedAt != nil {
		s := row.LastSyncedAt.UTC().Format(time.RFC3339Nano)
		lastSync = &s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 row.ID.String(),
		"label":              row.Label,
		"provider":           row.Provider,
		"ms_account_kind":    string(row.MsAccountKind),
		"primary_email":      row.PrimaryEmail,
		"connection_status":  row.ConnectionStatus,
		"last_error":         row.LastError,
		"last_synced_at":     lastSync,
		"graph_tenant_id":    row.GraphTenantID,
	})
}

func (h *Handlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.AccountSvc.Disconnect(r.Context(), userIDOrEmpty(r), id); err != nil {
		h.Log.Error("delete account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) syncAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	res, err := h.SyncSvc.SyncInbox(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		h.Log.Error("sync", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_run_id":          res.JobRunID.String(),
		"messages_upserted":   res.MessagesUpserted,
		"status":              "success",
	})
}

func (h *Handlers) listMessages(w http.ResponseWriter, r *http.Request) {
	filter := driven.MessageListFilter{Category: r.URL.Query().Get("category")}
	if aid := r.URL.Query().Get("account_id"); aid != "" {
		id, err := uuid.Parse(aid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
			return
		}
		filter.AccountID = &id
	}
	if v := r.URL.Query().Get("since"); v != "" {
		ts, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad since"})
			return
		}
		filter.Since = &ts
	}
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
	rows, err := h.Messages.ListMessages(r.Context(), userIDOrEmpty(r), filter)
	if err != nil {
		h.Log.Error("list messages", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type item struct {
		ID                 string          `json:"id"`
		AccountID          string          `json:"account_id"`
		ProviderMessageID  string          `json:"provider_message_id"`
		Subject            string          `json:"subject"`
		ReceivedAt         string          `json:"received_at"`
		HasAttachments     bool            `json:"has_attachments"`
		FromJSON           json.RawMessage `json:"from_json"`
		BodyText           *string         `json:"body_text,omitempty"`
		Preview            string          `json:"preview"`
		CategorySlug       *string         `json:"category_slug,omitempty"`
		CategoryConfidence *float64        `json:"category_confidence,omitempty"`
	}
	out := make([]item, 0, len(rows))
	for _, m := range rows {
		preview := ""
		if m.BodyText != nil {
			preview = messagePreview(*m.BodyText, 160)
		}
		out = append(out, item{
			ID:                 m.ID.String(),
			AccountID:          m.AccountID.String(),
			ProviderMessageID:  m.ProviderMessageID,
			Subject:            m.Subject,
			ReceivedAt:         m.ReceivedAt.UTC().Format(time.RFC3339Nano),
			HasAttachments:     m.HasAttachments,
			FromJSON:           json.RawMessage(m.FromJSON),
			BodyText:           m.BodyText,
			Preview:            preview,
			CategorySlug:       m.CategorySlug,
			CategoryConfidence: m.CategoryConfidence,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func messagePreview(body string, maxChars int) string {
	preview := body
	if looksLikeHTML(preview) {
		preview = stripHTML(preview)
	}
	preview = strings.Join(strings.Fields(preview), " ")
	if maxChars > 0 && len(preview) > maxChars {
		return preview[:maxChars]
	}
	return preview
}

func looksLikeHTML(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<!doctype") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "<head") ||
		strings.Contains(lower, "<body") ||
		strings.Contains(lower, "<div") ||
		strings.Contains(lower, "<span") ||
		strings.Contains(lower, "<p") ||
		strings.Contains(lower, "<a ") ||
		strings.Contains(lower, "<img") ||
		strings.Contains(lower, "<ul") ||
		strings.Contains(lower, "<li") ||
		strings.Contains(lower, "<table") ||
		strings.Contains(lower, "<br")
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			if b.Len() > 0 {
				b.WriteRune(' ')
			}
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return s
	}
	return html.UnescapeString(out)
}

func (h *Handlers) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	m, err := h.Messages.GetMessage(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		h.Log.Error("get message", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                  m.ID.String(),
		"account_id":          m.AccountID.String(),
		"provider_message_id": m.ProviderMessageID,
		"subject":             m.Subject,
		"received_at":         m.ReceivedAt.UTC().Format(time.RFC3339Nano),
		"from_json":           json.RawMessage(m.FromJSON),
		"body_text":           m.BodyText,
		"has_attachments":     m.HasAttachments,
		"category_slug":       m.CategorySlug,
		"category_confidence": m.CategoryConfidence,
	})
}

func (h *Handlers) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Messages.ListCategoryDefinitions(r.Context())
	if err != nil {
		h.Log.Error("list categories", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type item struct {
		ID          string `json:"id"`
		Slug        string `json:"slug"`
		DisplayName string `json:"display_name"`
		SortOrder   int    `json:"sort_order"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		out = append(out, item{
			ID:          row.ID.String(),
			Slug:        row.Slug,
			DisplayName: row.DisplayName,
			SortOrder:   row.SortOrder,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) categorizeAccount(w http.ResponseWriter, r *http.Request) {
	if h.CategorizeSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "categorize service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		Recategorize bool `json:"recategorize"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	res, err := h.CategorizeSvc.CategorizeAccount(r.Context(), userIDOrEmpty(r), id, appmessages.CategorizeOptions{
		Recategorize: body.Recategorize,
	})
	if err != nil {
		h.Log.Error("categorize", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_run_id":            res.JobRunID.String(),
		"messages_categorized":  res.MessagesCategorized,
		"recategorize":          body.Recategorize,
		"status":                "success",
	})
}

func (h *Handlers) listRuns(w http.ResponseWriter, r *http.Request) {
	if h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
	uid := userIDOrEmpty(r)
	filter := driven.JobRunListFilter{
		JobType: r.URL.Query().Get("job_type"),
	}
	if aid := r.URL.Query().Get("account_id"); aid != "" {
		id, err := uuid.Parse(aid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
			return
		}
		filter.AccountID = &id
	}
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
	rows, err := h.JobRuns.ListJobRuns(r.Context(), uid, filter)
	if err != nil {
		h.Log.Error("list runs", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type item struct {
		ID              string          `json:"id"`
		AccountID       *string         `json:"account_id,omitempty"`
		AccountLabel    *string         `json:"account_label,omitempty"`
		JobType         string          `json:"job_type"`
		Trigger         string          `json:"trigger"`
		Status          string          `json:"status"`
		TimeWindowStart *string         `json:"time_window_start,omitempty"`
		TimeWindowEnd   *string         `json:"time_window_end,omitempty"`
		StartedAt       string          `json:"started_at"`
		FinishedAt      *string         `json:"finished_at,omitempty"`
		ErrorMessage    *string         `json:"error_message,omitempty"`
		Meta            json.RawMessage `json:"meta_json"`
	}
	out := make([]item, 0, len(rows))
	for _, row := range rows {
		var accountID *string
		if row.AccountID != nil {
			s := row.AccountID.String()
			accountID = &s
		}
		var twStart *string
		if row.TimeWindowStart != nil {
			s := row.TimeWindowStart.UTC().Format(time.RFC3339Nano)
			twStart = &s
		}
		var twEnd *string
		if row.TimeWindowEnd != nil {
			s := row.TimeWindowEnd.UTC().Format(time.RFC3339Nano)
			twEnd = &s
		}
		var finishedAt *string
		if row.FinishedAt != nil {
			s := row.FinishedAt.UTC().Format(time.RFC3339Nano)
			finishedAt = &s
		}
		meta := json.RawMessage("{}")
		if row.MetaJSON != "" {
			meta = json.RawMessage(row.MetaJSON)
		}
		out = append(out, item{
			ID:              row.ID.String(),
			AccountID:       accountID,
			AccountLabel:    row.AccountLabel,
			JobType:         row.JobType,
			Trigger:         row.TriggerKind,
			Status:          row.Status,
			TimeWindowStart: twStart,
			TimeWindowEnd:   twEnd,
			StartedAt:       row.StartedAt.UTC().Format(time.RFC3339Nano),
			FinishedAt:      finishedAt,
			ErrorMessage:    row.ErrorMessage,
			Meta:            meta,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getRun(w http.ResponseWriter, r *http.Request) {
	if h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	row, err := h.JobRuns.GetJobRun(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		h.Log.Error("get run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var accountID *string
	if row.AccountID != nil {
		s := row.AccountID.String()
		accountID = &s
	}
	var twStart *string
	if row.TimeWindowStart != nil {
		s := row.TimeWindowStart.UTC().Format(time.RFC3339Nano)
		twStart = &s
	}
	var twEnd *string
	if row.TimeWindowEnd != nil {
		s := row.TimeWindowEnd.UTC().Format(time.RFC3339Nano)
		twEnd = &s
	}
	var finishedAt *string
	if row.FinishedAt != nil {
		s := row.FinishedAt.UTC().Format(time.RFC3339Nano)
		finishedAt = &s
	}
	meta := json.RawMessage("{}")
	if row.MetaJSON != "" {
		meta = json.RawMessage(row.MetaJSON)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                row.ID.String(),
		"account_id":        accountID,
		"account_label":     row.AccountLabel,
		"job_type":          row.JobType,
		"trigger":           row.TriggerKind,
		"status":            row.Status,
		"time_window_start": twStart,
		"time_window_end":   twEnd,
		"started_at":        row.StartedAt.UTC().Format(time.RFC3339Nano),
		"finished_at":       finishedAt,
		"error_message":     row.ErrorMessage,
		"meta_json":         meta,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // keep & in URLs as real ampersands (not \u0026) for copy-paste from JSON
	_ = enc.Encode(v)
}

func userIDOrEmpty(r *http.Request) uuid.UUID {
	uid, _ := UserIDFromContext(r.Context())
	return uid
}
