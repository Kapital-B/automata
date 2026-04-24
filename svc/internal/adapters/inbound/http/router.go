package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
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
	Accounts    driven.AccountRepository
	Messages    driven.MessageRepository
	OAuthStates driven.OAuthStateRepository
	Dashboard   string
	SuccessPath string
	ErrorPath   string
	StateTTL    time.Duration
}

func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Get("/api/health", h.health)
	r.Get("/api/accounts", h.listAccounts)
	r.Post("/api/accounts", h.startConnect)
	r.Get("/api/accounts/callback", h.oauthCallback)
	r.Get("/api/accounts/{id}", h.getAccount)
	r.Delete("/api/accounts/{id}", h.deleteAccount)
	r.Post("/api/accounts/{id}/sync", h.syncAccount)
	r.Get("/api/messages", h.listMessages)
	r.Get("/api/messages/{id}", h.getMessage)
	return r
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Accounts.ListAccounts(r.Context())
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
	res, err := h.AccountSvc.StartConnect(r.Context(), appaccounts.StartConnectInput{
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
	row, _, err := h.Accounts.GetAccount(r.Context(), id)
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
	if err := h.AccountSvc.Disconnect(r.Context(), id); err != nil {
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
	res, err := h.SyncSvc.SyncInbox(r.Context(), id)
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
	aid := r.URL.Query().Get("account_id")
	if aid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account_id required"})
		return
	}
	id, err := uuid.Parse(aid)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad account_id"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	rows, err := h.Messages.ListMessagesByAccount(r.Context(), id, limit, offset)
	if err != nil {
		h.Log.Error("list messages", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type item struct {
		ID                string `json:"id"`
		AccountID         string `json:"account_id"`
		ProviderMessageID string `json:"provider_message_id"`
		Subject           string `json:"subject"`
		ReceivedAt        string `json:"received_at"`
		HasAttachments    bool   `json:"has_attachments"`
	}
	out := make([]item, 0, len(rows))
	for _, m := range rows {
		out = append(out, item{
			ID:                m.ID.String(),
			AccountID:         m.AccountID.String(),
			ProviderMessageID: m.ProviderMessageID,
			Subject:           m.Subject,
			ReceivedAt:        m.ReceivedAt.UTC().Format(time.RFC3339Nano),
			HasAttachments:    m.HasAttachments,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getMessage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	m, err := h.Messages.GetMessage(r.Context(), id)
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
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // keep & in URLs as real ampersands (not \u0026) for copy-paste from JSON
	_ = enc.Encode(v)
}
