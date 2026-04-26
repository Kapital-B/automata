package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handlers holds wired application services for HTTP.
type Handlers struct {
	Log             *slog.Logger
	AccountSvc      *appaccounts.Service
	SyncSvc         *appmessages.SyncService
	CategorizeSvc   *appmessages.CategorizeService
	SummarizeSvc    *appmessages.SummarizeService
	AuthSvc         *auth.Service
	Accounts        driven.AccountRepository
	Messages        driven.MessageRepository
	JobRuns         driven.JobRunRepository
	Summaries       driven.SummaryRepository
	OAuthStates     driven.OAuthStateRepository
	Users           driven.UserRepository
	Dashboard       string
	SuccessPath     string
	ErrorPath       string
	AuthSuccessPath string
	AuthErrorPath   string
	StateTTL        time.Duration
	JWTSecret       []byte
	JWTTTL          time.Duration
	DefaultUserID   uuid.UUID // dev fallback when no Bearer token
	JobQueue        *asynqadapter.QueueClient
}

func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	if h.Log != nil {
		r.Use(requestLogMiddleware(h.Log))
	}
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
	r.Post("/api/accounts/{id}/summaries/refresh", h.refreshSummary)
	r.Get("/api/categories", h.listCategories)
	r.Post("/api/categories", h.createCategory)
	r.Patch("/api/categories/{id}", h.updateCategory)
	r.Delete("/api/categories/{id}", h.deleteCategory)
	r.Get("/api/runs", h.listRuns)
	r.Get("/api/runs/{id}", h.getRun)
	r.Get("/api/summaries", h.listSummaries)
	r.Post("/api/action-items/{id}/done", h.markActionItemDone)
	r.Post("/api/fyi/{id}/dismiss", h.dismissFYI)
	r.Get("/api/settings/summaries", h.getSummarySettings)
	r.Patch("/api/settings/summaries", h.updateSummarySettings)
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
		ID               string  `json:"id"`
		Label            string  `json:"label"`
		Provider         string  `json:"provider"`
		MsAccountKind    string  `json:"ms_account_kind"`
		PrimaryEmail     string  `json:"primary_email"`
		ConnectionStatus string  `json:"connection_status"`
		LastError        *string `json:"last_error,omitempty"`
		LastSyncedAt     *string `json:"last_synced_at,omitempty"`
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
		"id":                row.ID.String(),
		"label":             row.Label,
		"provider":          row.Provider,
		"ms_account_kind":   string(row.MsAccountKind),
		"primary_email":     row.PrimaryEmail,
		"connection_status": row.ConnectionStatus,
		"last_error":        row.LastError,
		"last_synced_at":    lastSync,
		"graph_tenant_id":   row.GraphTenantID,
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
	if h.SyncSvc == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync queue not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if h.JobQueue == nil {
		res, err := h.SyncSvc.SyncInbox(r.Context(), uid, id)
		if err != nil {
			h.Log.Error("sync", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"job_run_id":        res.JobRunID.String(),
			"messages_upserted": res.MessagesUpserted,
			"status":            "success",
		})
		return
	}
	jobID := uuid.New()
	started := time.Now().UTC()
	if err := h.JobRuns.InsertJobRun(r.Context(), jobID, id, "sync", "api", "pending", started, time.Time{}, nil, `{"queued":true}`); err != nil {
		h.Log.Error("create sync run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	err = h.JobQueue.EnqueueSync(r.Context(), asynqadapter.TaskPayload{
		SchemaVersion: 1,
		RunID:         jobID,
		UserID:        uid,
		AccountID:     id,
		TriggerKind:   "api",
	})
	if err != nil {
		msg := err.Error()
		_ = h.JobRuns.UpdateJobRunStatus(r.Context(), jobID, "failed", timePtrHTTP(time.Now().UTC()), &msg, `{"queued":false}`)
		h.Log.Error("enqueue sync", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_run_id": jobID.String(),
		"status":     "queued",
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
	rows, err := h.Messages.ListCategoryDefinitions(r.Context(), userIDOrEmpty(r))
	if err != nil {
		h.Log.Error("list categories", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]categoryItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, toCategoryItem(row))
	}
	writeJSON(w, http.StatusOK, out)
}

type categoryItem struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Definition  string `json:"definition"`
	SortOrder   int    `json:"sort_order"`
}

func toCategoryItem(row driven.CategoryDefinitionRow) categoryItem {
	return categoryItem{
		ID:          row.ID.String(),
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Definition:  row.Definition,
		SortOrder:   row.SortOrder,
	}
}

type categoryUpsertBody struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Definition  string `json:"definition"`
	SortOrder   int    `json:"sort_order"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,62}[a-z0-9]$|^[a-z0-9]$`)

func normalizeCategoryInput(body categoryUpsertBody) (categoryUpsertBody, error) {
	body.Slug = strings.ToLower(strings.TrimSpace(body.Slug))
	body.DisplayName = strings.TrimSpace(body.DisplayName)
	body.Definition = strings.TrimSpace(body.Definition)
	if !slugPattern.MatchString(body.Slug) {
		return body, errors.New("slug must be 1-64 chars: lowercase letters, digits, '_' or '-'")
	}
	if body.DisplayName == "" {
		return body, errors.New("display_name is required")
	}
	if len(body.Definition) > 280 {
		return body, errors.New("definition is too long (max 280 chars)")
	}
	return body, nil
}

func (h *Handlers) createCategory(w http.ResponseWriter, r *http.Request) {
	var body categoryUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body, err := normalizeCategoryInput(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	uid := userIDOrEmpty(r)
	now := time.Now().UTC()
	row := driven.CategoryDefinitionRow{
		ID:          uuid.New(),
		UserID:      uid,
		Slug:        body.Slug,
		DisplayName: body.DisplayName,
		Definition:  body.Definition,
		SortOrder:   body.SortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.Messages.CreateCategoryDefinition(r.Context(), row); err != nil {
		h.Log.Error("create category", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "create failed"})
		return
	}
	writeJSON(w, http.StatusCreated, toCategoryItem(row))
}

func (h *Handlers) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body categoryUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	body, err = normalizeCategoryInput(body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	uid := userIDOrEmpty(r)
	cur, err := h.Messages.GetCategoryDefinitionByID(r.Context(), uid, id)
	if err != nil {
		h.Log.Error("get category for update", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if cur == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	cur.Slug = body.Slug
	cur.DisplayName = body.DisplayName
	cur.Definition = body.Definition
	cur.SortOrder = body.SortOrder
	cur.UpdatedAt = time.Now().UTC()
	if err := h.Messages.UpdateCategoryDefinition(r.Context(), *cur); err != nil {
		h.Log.Error("update category", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "update failed"})
		return
	}
	writeJSON(w, http.StatusOK, toCategoryItem(*cur))
}

func (h *Handlers) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	cur, err := h.Messages.GetCategoryDefinitionByID(r.Context(), uid, id)
	if err != nil {
		h.Log.Error("get category for delete", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if cur == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	replRaw := strings.TrimSpace(r.URL.Query().Get("replacement_id"))
	if replRaw == "" {
		n, err := h.Messages.CountMessageCategoriesByCategory(r.Context(), uid, id)
		if err != nil {
			h.Log.Error("count category usage", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if n > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category is in use; pass replacement_id"})
			return
		}
	} else {
		replID, err := uuid.Parse(replRaw)
		if err != nil || replID == id {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid replacement_id"})
			return
		}
		repl, err := h.Messages.GetCategoryDefinitionByID(r.Context(), uid, replID)
		if err != nil {
			h.Log.Error("get replacement category", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if repl == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "replacement category not found"})
			return
		}
		if _, err := h.Messages.ReassignMessageCategories(r.Context(), uid, id, replID); err != nil {
			h.Log.Error("reassign categories", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reassign failed"})
			return
		}
	}
	if err := h.Messages.DeleteCategoryDefinition(r.Context(), uid, id); err != nil {
		h.Log.Error("delete category", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "delete failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) categorizeAccount(w http.ResponseWriter, r *http.Request) {
	if h.CategorizeSvc == nil || h.JobRuns == nil {
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
	uid := userIDOrEmpty(r)
	if h.JobQueue == nil {
		res, err := h.CategorizeSvc.CategorizeAccount(r.Context(), uid, id, appmessages.CategorizeOptions{
			Recategorize: body.Recategorize,
		})
		if err != nil {
			h.Log.Error("categorize", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"job_run_id":           res.JobRunID.String(),
			"messages_categorized": res.MessagesCategorized,
			"recategorize":         body.Recategorize,
			"status":               "success",
		})
		return
	}
	jobID := uuid.New()
	started := time.Now().UTC()
	meta := fmt.Sprintf(`{"queued":true,"recategorize":%t}`, body.Recategorize)
	if err := h.JobRuns.InsertJobRun(r.Context(), jobID, id, "categorize", "api", "pending", started, time.Time{}, nil, meta); err != nil {
		h.Log.Error("create categorize run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	err = h.JobQueue.EnqueueCategorize(r.Context(), asynqadapter.TaskPayload{
		SchemaVersion: 1,
		RunID:         jobID,
		UserID:        uid,
		AccountID:     id,
		TriggerKind:   "api",
		Recategorize:  body.Recategorize,
	})
	if err != nil {
		msg := err.Error()
		_ = h.JobRuns.UpdateJobRunStatus(r.Context(), jobID, "failed", timePtrHTTP(time.Now().UTC()), &msg, fmt.Sprintf(`{"queued":false,"recategorize":%t}`, body.Recategorize))
		h.Log.Error("enqueue categorize", "err", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_run_id":   jobID.String(),
		"recategorize": body.Recategorize,
		"status":       "queued",
	})
}

func (h *Handlers) refreshSummary(w http.ResponseWriter, r *http.Request) {
	if h.SummarizeSvc == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summarize service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if h.JobQueue == nil {
		res, err := h.SummarizeSvc.SummarizeAccount(r.Context(), uid, id, appmessages.SummarizeOptions{})
		if err != nil {
			h.Log.Error("summarize", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job_run_id": res.JobRunID.String(), "status": "success"})
		return
	}
	jobID := uuid.New()
	started := time.Now().UTC()
	if err := h.JobRuns.InsertJobRun(r.Context(), jobID, id, "summarize", "api", "pending", started, time.Time{}, nil, `{"queued":true}`); err != nil {
		h.Log.Error("create summarize run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	err = h.JobQueue.EnqueueSummarize(r.Context(), asynqadapter.TaskPayload{
		SchemaVersion: 1,
		RunID:         jobID,
		UserID:        uid,
		AccountID:     id,
		TriggerKind:   "api",
	})
	if err != nil {
		msg := err.Error()
		_ = h.JobRuns.UpdateJobRunStatus(r.Context(), jobID, "failed", timePtrHTTP(time.Now().UTC()), &msg, `{"queued":false}`)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_run_id": jobID.String(), "status": "queued"})
}

func (h *Handlers) listSummaries(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	uid := userIDOrEmpty(r)
	var accountID *uuid.UUID
	if aid := strings.TrimSpace(r.URL.Query().Get("account_id")); aid != "" {
		id, err := uuid.Parse(aid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
			return
		}
		accountID = &id
	}
	snaps, err := h.Summaries.ListSummarySnapshots(r.Context(), uid, accountID, 1)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if len(snaps) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot":      nil,
			"action_items":  []any{},
			"fyi":           []any{},
		})
		return
	}
	snap := snaps[0]
	items, err := h.Summaries.ListOpenActionItems(r.Context(), uid, accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	fyiRows, err := h.Summaries.ListFYIByRun(r.Context(), uid, snap.RunID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type actionItem struct {
		ID         string  `json:"id"`
		AccountID  string  `json:"account_id"`
		MessageID  string  `json:"message_id"`
		Text       string  `json:"text"`
		DueAt      *string `json:"due_at,omitempty"`
		IsOverdue  bool    `json:"is_overdue"`
	}
	outItems := make([]actionItem, 0, len(items))
	now := time.Now().UTC()
	for _, it := range items {
		var dueAt *string
		overdue := false
		if it.DueAt != nil {
			s := it.DueAt.UTC().Format(time.RFC3339Nano)
			dueAt = &s
			overdue = it.DueAt.Before(now)
		}
		outItems = append(outItems, actionItem{
			ID: it.ID.String(), AccountID: it.AccountID.String(), MessageID: it.MessageID.String(), Text: it.Text, DueAt: dueAt, IsOverdue: overdue,
		})
	}
	type fyiItem struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
	}
	outFYI := make([]fyiItem, 0, len(fyiRows))
	for _, it := range fyiRows {
		outFYI = append(outFYI, fyiItem{ID: it.ID.String(), AccountID: it.AccountID.String(), MessageID: it.MessageID.String(), Text: it.Text})
	}
	var snapAccountID *string
	if snap.AccountID != nil {
		s := snap.AccountID.String()
		snapAccountID = &s
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": map[string]any{
			"id":              snap.ID.String(),
			"account_id":      snapAccountID,
			"run_id":          snap.RunID.String(),
			"window_start":    snap.WindowStart.UTC().Format(time.RFC3339Nano),
			"window_end":      snap.WindowEnd.UTC().Format(time.RFC3339Nano),
			"general_summary": snap.GeneralSummary,
			"created_at":      snap.CreatedAt.UTC().Format(time.RFC3339Nano),
		},
		"action_items": outItems,
		"fyi":          outFYI,
	})
}

func (h *Handlers) markActionItemDone(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.Summaries.MarkActionItemDone(r.Context(), userIDOrEmpty(r), id, time.Now().UTC()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) dismissFYI(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.Summaries.DeleteFYI(r.Context(), userIDOrEmpty(r), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) getSummarySettings(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	row, err := h.Summaries.GetSummarySettings(r.Context(), userIDOrEmpty(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"include_category_slugs": []string{},
			"exclude_category_slugs": []string{},
			"chunk_size":             12,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"include_category_slugs": row.IncludeCategorySlugs,
		"exclude_category_slugs": row.ExcludeCategorySlugs,
		"chunk_size":             row.ChunkSize,
		"updated_at":             row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	})
}

func (h *Handlers) updateSummarySettings(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	var body struct {
		Include []string `json:"include_category_slugs"`
		Exclude []string `json:"exclude_category_slugs"`
		ChunkSize int `json:"chunk_size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	row := driven.SummarySettingsRow{
		UserID:               userIDOrEmpty(r),
		IncludeCategorySlugs: body.Include,
		ExcludeCategorySlugs: body.Exclude,
		ChunkSize:            body.ChunkSize,
		UpdatedAt:            time.Now().UTC(),
	}
	if row.ChunkSize <= 0 {
		row.ChunkSize = 12
	}
	if err := h.Summaries.UpsertSummarySettings(r.Context(), row); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
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

func timePtrHTTP(t time.Time) *time.Time {
	return &t
}
