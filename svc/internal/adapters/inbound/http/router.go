package http

import (
	"context"
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

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	appaccounts "github.com/Kapital-B/automata/svc/internal/application/accounts"
	appattention "github.com/Kapital-B/automata/svc/internal/application/attention"
	"github.com/Kapital-B/automata/svc/internal/application/auth"
	appconnectors "github.com/Kapital-B/automata/svc/internal/application/connectors"
	appcontacts "github.com/Kapital-B/automata/svc/internal/application/contacts"
	appdecisions "github.com/Kapital-B/automata/svc/internal/application/decisions"
	appfacts "github.com/Kapital-B/automata/svc/internal/application/facts"
	appinterpret "github.com/Kapital-B/automata/svc/internal/application/interpret"
	appissues "github.com/Kapital-B/automata/svc/internal/application/issues"
	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	appprojectai "github.com/Kapital-B/automata/svc/internal/application/projectai"
	appprojects "github.com/Kapital-B/automata/svc/internal/application/projects"
	appreconcile "github.com/Kapital-B/automata/svc/internal/application/reconcile"
	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handlers holds wired application services for HTTP.
type Handlers struct {
	Log                  *slog.Logger
	AccountSvc           *appaccounts.Service
	ConnectorSvc         *appconnectors.Service
	SyncSvc              *appmessages.SyncService
	CategorizeSvc        *appmessages.CategorizeService
	SummarizeSvc         *appmessages.SummarizeService
	AutoDraftSvc         *appmessages.AutoDraftService
	DraftsSvc            *appmessages.DraftLifecycleService
	ForwardRulesSvc      *appmessages.ForwardRulesService
	AuthSvc              *auth.Service
	ContactSvc           *appcontacts.Service
	ProjectSvc           *appprojects.Service
	IssueSvc             *appissues.Service
	FactSvc              *appfacts.Service
	InterpretSvc         *appinterpret.Service
	ReconcileSvc         *appreconcile.Service
	DecisionSvc          *appdecisions.Service
	AttentionSvc         *appattention.Service
	ProjectAISvc         *appprojectai.Service
	Accounts             driven.AccountRepository
	Messages             driven.MessageRepository
	JobRuns              driven.JobRunRepository
	JobStore             driven.JobStore
	JobEnqueuer          driven.JobEnqueuer
	JobTerminalTTL       time.Duration
	JobsInline           bool
	Summaries            driven.SummaryRepository
	Forwards             driven.ForwardRepository
	Schedules            driven.ScheduleRepository
	OAuthStates          driven.OAuthStateRepository
	Users                driven.UserRepository
	Contacts             driven.ContactRepository
	Projects             driven.ProjectRepository
	Assignments          driven.AssignmentRepository
	Issues               driven.IssueRepository
	Dashboard            string
	SuccessPath          string
	ConnectorSuccessPath string
	ErrorPath            string
	AuthSuccessPath      string
	AuthErrorPath        string
	StateTTL             time.Duration
	JWTSecret            []byte
	JWTTTL               time.Duration
	DefaultUserID        uuid.UUID // dev fallback when no Bearer token
	JobQueue             *asynqadapter.QueueClient
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

	r.Get("/api/contacts", h.listContacts)
	r.Post("/api/contacts", h.createContact)
	r.Get("/api/contacts/{id}", h.getContact)
	r.Post("/api/contacts/{id}/identities", h.addContactIdentity)
	r.Post("/api/contacts/{id}/merge", h.mergeContacts)

	r.Get("/api/projects", h.listProjects)
	r.Post("/api/projects", h.createProject)
	r.Get("/api/projects/{id}", h.getProject)
	r.Patch("/api/projects/{id}", h.updateProject)
	r.Patch("/api/projects/{id}/member", h.updateProjectMember)
	r.Get("/api/projects/{id}/timeline", h.getProjectTimeline)
	r.Get("/api/projects/{id}/current-position", h.getProjectCurrentPosition)
	r.Get("/api/projects/{id}/facts", h.listProjectFacts)
	r.Post("/api/projects/{id}/facts", h.createProjectFact)
	r.Post("/api/projects/{id}/interpret", h.interpretProject)
	r.Get("/api/projects/{id}/interpretations", h.listProjectInterpretations)
	r.Post("/api/interpretations/{id}/dismiss", h.dismissInterpretation)
	r.Post("/api/projects/{id}/reconcile", h.reconcileProject)
	r.Get("/api/projects/{id}/contradictions", h.listProjectContradictions)
	r.Post("/api/contradictions/{id}/resolve", h.resolveContradiction)
	r.Get("/api/projects/{id}/decisions", h.listProjectDecisions)
	r.Post("/api/projects/{id}/decisions", h.createProjectDecision)
	r.Post("/api/decisions/{id}/confirm", h.confirmDecision)
	r.Post("/api/decisions/{id}/withdraw", h.withdrawDecision)
	r.Patch("/api/decisions/{id}", h.patchDecision)
	r.Get("/api/attention", h.listAttention)
	r.Get("/api/projects/{id}/attention", h.listProjectAttention)
	r.Post("/api/ask", h.askAcross)
	r.Post("/api/projects/{id}/ask", h.askProject)
	r.Get("/api/facts/{id}", h.getFact)
	r.Post("/api/fact-versions/{id}/confirm", h.confirmFactVersion)
	r.Post("/api/fact-versions/{id}/reject", h.rejectFactVersion)
	r.Post("/api/fact-versions/{id}/evidence", h.addFactEvidence)
	r.Delete("/api/fact-versions/{id}/evidence/{evidenceID}", h.removeFactEvidence)
	r.Get("/api/projects/{id}/issues", h.listProjectIssues)
	r.Post("/api/projects/{id}/issues/suggest", h.suggestProjectIssue)
	r.Post("/api/projects/{id}/issues", h.createProjectIssue)
	r.Get("/api/issues/{id}", h.getIssue)
	r.Patch("/api/issues/{id}", h.updateIssue)
	r.Post("/api/issues/{id}/items", h.addIssueItem)
	r.Delete("/api/issues/{id}/items/{itemID}", h.removeIssueItem)
	r.Get("/api/unassigned/summary", h.unassignedSummary)
	r.Get("/api/unassigned", h.listUnassigned)
	r.Post("/api/messages/{id}/project-assignment", h.assignMessageProject)
	r.Delete("/api/messages/{id}/project-assignment/override", h.clearMessageOverride)
	r.Post("/api/manual-items", h.createManualItem)
	r.Post("/api/manual-items/{id}/project-assignment", h.assignManualItemProject)

	r.Get("/api/accounts", h.listAccounts)
	r.Post("/api/accounts", h.startConnect)
	r.Get("/api/accounts/callback", h.oauthCallback)
	r.Get("/api/accounts/{id}", h.getAccount)
	r.Delete("/api/accounts/{id}", h.deleteAccount)
	r.Post("/api/accounts/{id}/sync", h.syncAccount)
	r.Post("/api/accounts/{id}/categorize", h.categorizeAccount)
	r.Post("/api/accounts/{id}/summaries/refresh", h.refreshSummary)
	r.Post("/api/accounts/{id}/drafts/generate", h.generateDrafts)
	r.Get("/api/categories", h.listCategories)
	r.Post("/api/categories", h.createCategory)
	r.Patch("/api/categories/{id}", h.updateCategory)
	r.Delete("/api/categories/{id}", h.deleteCategory)
	r.Get("/api/runs", h.listRuns)
	r.Get("/api/runs/{id}", h.getRun)
	r.Post("/api/runs/{id}/cancel", h.cancelRun)
	r.Get("/api/summaries", h.listSummaries)
	r.Post("/api/action-items/{id}/done", h.markActionItemDone)
	r.Post("/api/fyi/{id}/dismiss", h.dismissFYI)
	r.Get("/api/settings/summaries", h.getSummarySettings)
	r.Patch("/api/settings/summaries", h.updateSummarySettings)
	r.Get("/api/settings/schedules", h.getSchedules)
	r.Patch("/api/settings/schedules", h.updateSchedules)
	r.Get("/api/messages", h.listMessages)
	r.Get("/api/messages/{id}", h.getMessage)
	r.Post("/api/messages/{id}/forward", h.forwardMessage)
	r.Get("/api/forward-allowlist", h.getForwardAllowlist)
	r.Put("/api/forward-allowlist", h.putForwardAllowlist)
	r.Get("/api/accounts/{id}/forward-rules", h.listForwardRules)
	r.Post("/api/accounts/{id}/forward-rules", h.createForwardRule)
	r.Patch("/api/forward-rules/{id}", h.updateForwardRule)
	r.Delete("/api/forward-rules/{id}", h.deleteForwardRule)
	r.Post("/api/accounts/{id}/forward-rules/run", h.runForwardRules)
	r.Get("/api/connectors", h.listConnectors)
	r.Post("/api/connectors", h.startConnectorConnect)
	r.Get("/api/connectors/callback", h.connectorOAuthCallback)
	r.Delete("/api/connectors/{id}", h.deleteConnector)
	r.Post("/api/connectors/{id}/sync", h.syncConnector)
	r.Get("/api/connectors/{id}/bindings", h.listConnectorBindings)
	r.Post("/api/connectors/{id}/bindings", h.createConnectorBinding)
	r.Get("/api/drafts", h.listDrafts)
	r.Get("/api/drafts/{id}/attempts", h.listDraftAttempts)
	r.Patch("/api/drafts/{id}", h.saveDraft)
	r.Delete("/api/drafts/{id}", h.discardDraft)
	r.Post("/api/drafts/{id}/send", h.sendDraft)
	return r
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"llm":    h.CategorizeSvc != nil || (h.IssueSvc != nil && h.IssueSvc.HasLLM()) || (h.InterpretSvc != nil && h.InterpretSvc.HasLLM()),
	})
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
	if h.SyncSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync queue not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, id); err != nil {
		h.Log.Error("authorize sync account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.JobsInline {
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
	if h.JobEnqueuer != nil {
		job, err := h.enqueueAccountJob(r.Context(), uid, id, "sync", driven.JobPayload{})
		if err != nil {
			h.Log.Error("enqueue sync", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if h.JobQueue == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sync queue not configured"})
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
	if pid := r.URL.Query().Get("project_id"); pid != "" {
		id, err := uuid.Parse(pid)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad project_id"})
			return
		}
		filter.ProjectID = &id
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
	uid := userIDOrEmpty(r)
	rows, err := h.Messages.ListMessages(r.Context(), uid, filter)
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
		ConversationID     *string         `json:"conversation_id,omitempty"`
		ProjectID          *string         `json:"project_id,omitempty"`
	}
	out := make([]item, 0, len(rows))
	for _, m := range rows {
		preview := ""
		if m.BodyText != nil {
			preview = messagePreview(*m.BodyText, 160)
		}
		it := item{
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
			ConversationID:     m.ConversationID,
		}
		if h.Assignments != nil {
			if eff, err := h.Assignments.EffectiveAssignment(r.Context(), uid, m.ID); err == nil && eff != nil && eff.ProjectID != nil {
				s := eff.ProjectID.String()
				it.ProjectID = &s
			}
		}
		out = append(out, it)
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

type forwardMessageBody struct {
	ToEmail string `json:"to_email"`
	Comment string `json:"comment"`
}

func (h *Handlers) forwardMessage(w http.ResponseWriter, r *http.Request) {
	if h.ForwardRulesSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "forward service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body forwardMessageBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := userIDOrEmpty(r)
	err = h.ForwardRulesSvc.ManualForwardMessage(r.Context(), uid, id, body.ToEmail, body.Comment)
	if err != nil {
		msg := err.Error()
		switch {
		case msg == "to_email required" || msg == "invalid to_email" || msg == "forward_to not in allowlist":
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
			return
		case msg == "message not found" || msg == "account not found":
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg})
			return
		}
		if h.Log != nil {
			h.Log.Warn("manual forward", "err", err)
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "forward failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) listDrafts(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.Summaries.ListDraftSuggestions(r.Context(), uid, accountID, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type fromPayload struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var from fromPayload
		_ = json.Unmarshal([]byte(row.FromJSON), &from)
		out = append(out, map[string]any{
			"id":             row.ID.String(),
			"account_id":     row.AccountID.String(),
			"message_id":     row.MessageID.String(),
			"action_item_id": row.ActionItemID.String(),
			"run_id":         row.RunID.String(),
			"subject":        row.Subject,
			"body":           row.Body,
			"model":          row.Model,
			"to_name":        from.Name,
			"to_email":       from.Address,
			"created_at":     row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type forwardRuleUpsertBody struct {
	Name          string          `json:"name"`
	Mode          string          `json:"mode"`
	ConditionJSON json.RawMessage `json:"condition_json"`
	ForwardTo     string          `json:"forward_to"`
	Enabled       bool            `json:"enabled"`
}

// forwardRuleCreateBody defaults enabled to false when JSON omits "enabled".
type forwardRuleCreateBody struct {
	Name          string          `json:"name"`
	Mode          string          `json:"mode"`
	ConditionJSON json.RawMessage `json:"condition_json"`
	ForwardTo     string          `json:"forward_to"`
	Enabled       *bool           `json:"enabled"`
}

func forwardDestInAllowlist(dest string, rows []driven.ForwardAllowlistRow) bool {
	d := strings.ToLower(strings.TrimSpace(dest))
	if d == "" {
		return false
	}
	for _, r := range rows {
		if strings.ToLower(strings.TrimSpace(r.Email)) == d {
			return true
		}
	}
	return false
}

func (h *Handlers) getForwardAllowlist(w http.ResponseWriter, r *http.Request) {
	if h.Forwards == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "persistence not configured"})
		return
	}
	rows, err := h.Forwards.ListForwardAllowlist(r.Context(), userIDOrEmpty(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Email)
	}
	writeJSON(w, http.StatusOK, map[string]any{"emails": out})
}

func (h *Handlers) putForwardAllowlist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Emails []string `json:"emails"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := h.Forwards.ReplaceForwardAllowlist(r.Context(), userIDOrEmpty(r), body.Emails); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handlers) listForwardRules(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.Forwards.ListForwardRules(r.Context(), userIDOrEmpty(r), accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id":             row.ID.String(),
			"account_id":     row.AccountID.String(),
			"name":           row.Name,
			"mode":           row.Mode,
			"condition_json": json.RawMessage(row.ConditionJSON),
			"forward_to":     row.ForwardTo,
			"enabled":        row.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) createForwardRule(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body forwardRuleCreateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := userIDOrEmpty(r)
	allowRows, err := h.Forwards.ListForwardAllowlist(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	forwardTo := strings.ToLower(strings.TrimSpace(body.ForwardTo))
	if !forwardDestInAllowlist(forwardTo, allowRows) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "forward_to not in allowlist"})
		return
	}
	enabled := false
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	row := driven.ForwardRuleRow{
		ID:            uuid.New(),
		UserID:        uid,
		AccountID:     accountID,
		Name:          strings.TrimSpace(body.Name),
		Mode:          strings.ToLower(strings.TrimSpace(body.Mode)),
		ConditionJSON: string(body.ConditionJSON),
		ForwardTo:     forwardTo,
		Enabled:       enabled,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := h.Forwards.CreateForwardRule(r.Context(), row); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": row.ID.String()})
}

func (h *Handlers) updateForwardRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body forwardRuleUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := userIDOrEmpty(r)
	allowRows, err := h.Forwards.ListForwardAllowlist(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	forwardTo := strings.ToLower(strings.TrimSpace(body.ForwardTo))
	if !forwardDestInAllowlist(forwardTo, allowRows) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "forward_to not in allowlist"})
		return
	}
	row := driven.ForwardRuleRow{
		ID:            ruleID,
		UserID:        uid,
		Name:          strings.TrimSpace(body.Name),
		Mode:          strings.ToLower(strings.TrimSpace(body.Mode)),
		ConditionJSON: string(body.ConditionJSON),
		ForwardTo:     forwardTo,
		Enabled:       body.Enabled,
		UpdatedAt:     time.Now().UTC(),
	}
	if err := h.Forwards.UpdateForwardRule(r.Context(), row); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handlers) deleteForwardRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.Forwards.DeleteForwardRule(r.Context(), userIDOrEmpty(r), ruleID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) runForwardRules(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, accountID); err != nil {
		h.Log.Error("authorize forward rules account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if !h.JobsInline && h.JobEnqueuer != nil {
		job, err := h.enqueueAccountJob(r.Context(), uid, accountID, "forward_rules", driven.JobPayload{})
		if err != nil {
			h.Log.Error("enqueue forward_rules", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if !h.JobsInline && h.JobQueue != nil && h.JobRuns != nil {
		runID := uuid.New()
		_ = h.JobRuns.InsertJobRun(r.Context(), runID, accountID, "forward_rules", "api", "pending", time.Now().UTC(), time.Time{}, nil, `{"queued":true}`)
		err := h.JobQueue.EnqueueForwardRules(r.Context(), asynqadapter.TaskPayload{
			SchemaVersion: 1, RunID: runID, UserID: uid, AccountID: accountID, TriggerKind: "api",
		})
		if err != nil {
			msg := err.Error()
			_ = h.JobRuns.UpdateJobRunStatus(r.Context(), runID, "failed", timePtrHTTP(time.Now().UTC()), &msg, `{"queued":false}`)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_run_id": runID.String(), "status": "queued"})
		return
	}
	if h.ForwardRulesSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "forward-rules service not configured"})
		return
	}
	runID, err := h.ForwardRulesSvc.RunAccount(r.Context(), uid, accountID, appmessages.ForwardRulesOptions{Trigger: "api"})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job_run_id": runID.String(), "status": "success"})
}

func (h *Handlers) listDraftAttempts(w http.ResponseWriter, r *http.Request) {
	if h.Summaries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	draftID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.Summaries.ListSendAttemptsByDraft(r.Context(), userIDOrEmpty(r), draftID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id":                  row.ID.String(),
			"draft_id":            row.DraftID.String(),
			"account_id":          row.AccountID.String(),
			"message_id":          row.MessageID.String(),
			"status":              row.Status,
			"provider_message_id": row.ProviderMessageID,
			"error_message":       row.ErrorMessage,
			"created_at":          row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type draftUpdateBody struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func (h *Handlers) saveDraft(w http.ResponseWriter, r *http.Request) {
	if h.DraftsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "draft service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body draftUpdateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := h.DraftsSvc.SaveDraft(r.Context(), userIDOrEmpty(r), id, body.Subject, body.Body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handlers) discardDraft(w http.ResponseWriter, r *http.Request) {
	if h.DraftsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "draft service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.DraftsSvc.DiscardDraft(r.Context(), userIDOrEmpty(r), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) sendDraft(w http.ResponseWriter, r *http.Request) {
	if h.DraftsSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "draft service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.DraftsSvc.SendDraft(r.Context(), userIDOrEmpty(r), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, id); err != nil {
		h.Log.Error("authorize categorize account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.JobsInline {
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
	if h.JobEnqueuer != nil {
		job, err := h.enqueueAccountJob(r.Context(), uid, id, "categorize", driven.JobPayload{
			Recategorize: body.Recategorize,
		})
		if err != nil {
			h.Log.Error("enqueue categorize", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if h.JobQueue == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "categorize service not configured"})
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
	if h.SummarizeSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summarize service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, id); err != nil {
		h.Log.Error("authorize summarize account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.JobsInline {
		res, err := h.SummarizeSvc.SummarizeAccount(r.Context(), uid, id, appmessages.SummarizeOptions{})
		if err != nil {
			h.Log.Error("summarize", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"job_run_id": res.JobRunID.String(), "status": "success"})
		return
	}
	if h.JobEnqueuer != nil {
		job, err := h.enqueueAccountJob(r.Context(), uid, id, "summarize", driven.JobPayload{})
		if err != nil {
			h.Log.Error("enqueue summarize", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if h.JobQueue == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summarize service not configured"})
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

func (h *Handlers) generateDrafts(w http.ResponseWriter, r *http.Request) {
	if h.AutoDraftSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto-draft service not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	var body struct {
		MessageID string `json:"message_id"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
	}
	var messageID *uuid.UUID
	if strings.TrimSpace(body.MessageID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(body.MessageID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad message_id"})
			return
		}
		messageID = &parsed
	}
	if ok, err := h.authorizeAccount(r.Context(), uid, id); err != nil {
		h.Log.Error("authorize draft account", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.JobsInline {
		res, err := h.AutoDraftSvc.GenerateForAccount(r.Context(), uid, id, appmessages.AutoDraftOptions{
			OnlyUnseen: false,
			MessageID:  messageID,
			Force:      messageID != nil,
		})
		if err != nil {
			h.Log.Error("auto-draft", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"job_run_id":        res.JobRunID.String(),
			"drafts_generated":  res.DraftsGenerated,
			"action_items_seen": res.ActionItemsSeen,
			"status":            "success",
		})
		return
	}
	if h.JobEnqueuer != nil {
		job, err := h.enqueueAccountJob(r.Context(), uid, id, "draft_suggest", driven.JobPayload{
			MessageID: messageID,
			Force:     messageID != nil,
		})
		if err != nil {
			h.Log.Error("enqueue draft_suggest", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if h.JobQueue == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "auto-draft service not configured"})
		return
	}
	jobID := uuid.New()
	started := time.Now().UTC()
	if err := h.JobRuns.InsertJobRun(r.Context(), jobID, id, "draft_suggest", "api", "pending", started, time.Time{}, nil, `{"queued":true,"only_unseen":false}`); err != nil {
		h.Log.Error("create draft_suggest run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	err = h.JobQueue.EnqueueDraftSuggest(r.Context(), asynqadapter.TaskPayload{
		SchemaVersion: 1,
		RunID:         jobID,
		UserID:        uid,
		AccountID:     id,
		TriggerKind:   "api",
		MessageID:     messageID,
		Force:         messageID != nil,
	})
	if err != nil {
		msg := err.Error()
		_ = h.JobRuns.UpdateJobRunStatus(r.Context(), jobID, "failed", timePtrHTTP(time.Now().UTC()), &msg, `{"queued":false,"only_unseen":false}`)
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
			"snapshot":     nil,
			"action_items": []any{},
			"fyi":          []any{},
		})
		return
	}
	snap := snaps[0]
	items, err := h.Summaries.ListOpenActionItems(r.Context(), uid, accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	fyiRows, err := h.Summaries.ListOpenFYI(r.Context(), uid, accountID, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	type actionItem struct {
		ID        string  `json:"id"`
		AccountID string  `json:"account_id"`
		MessageID string  `json:"message_id"`
		Text      string  `json:"text"`
		CreatedAt string  `json:"created_at"`
		DueAt     *string `json:"due_at,omitempty"`
		IsOverdue bool    `json:"is_overdue"`
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
			ID: it.ID.String(), AccountID: it.AccountID.String(), MessageID: it.MessageID.String(), Text: it.Text, CreatedAt: it.CreatedAt.UTC().Format(time.RFC3339Nano), DueAt: dueAt, IsOverdue: overdue,
		})
	}
	type fyiItem struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
		CreatedAt string `json:"created_at"`
	}
	outFYI := make([]fyiItem, 0, len(fyiRows))
	for _, it := range fyiRows {
		outFYI = append(outFYI, fyiItem{ID: it.ID.String(), AccountID: it.AccountID.String(), MessageID: it.MessageID.String(), Text: it.Text, CreatedAt: it.CreatedAt.UTC().Format(time.RFC3339Nano)})
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
		Include   []string `json:"include_category_slugs"`
		Exclude   []string `json:"exclude_category_slugs"`
		ChunkSize int      `json:"chunk_size"`
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

func (h *Handlers) getSchedules(w http.ResponseWriter, r *http.Request) {
	if h.Schedules == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedules not configured"})
		return
	}
	rows, err := h.Schedules.ListSchedulesByUser(r.Context(), userIDOrEmpty(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var accountID *string
		if row.AccountID != nil {
			s := row.AccountID.String()
			accountID = &s
		}
		out = append(out, map[string]any{
			"id":               row.ID.String(),
			"name":             row.Name,
			"account_id":       accountID,
			"jobs":             row.Jobs,
			"interval_minutes": row.IntervalMinutes,
			"enabled":          row.Enabled,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"chains": out})
}

func (h *Handlers) updateSchedules(w http.ResponseWriter, r *http.Request) {
	if h.Schedules == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedules not configured"})
		return
	}
	var body struct {
		Chains []struct {
			ID              string   `json:"id"`
			Name            string   `json:"name"`
			AccountID       *string  `json:"account_id"`
			Jobs            []string `json:"jobs"`
			IntervalMinutes int      `json:"interval_minutes"`
			Enabled         bool     `json:"enabled"`
		} `json:"chains"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	now := time.Now().UTC()
	rows := make([]driven.ScheduleChainRow, 0, len(body.Chains))
	for _, in := range body.Chains {
		id := uuid.New()
		if strings.TrimSpace(in.ID) != "" {
			parsed, err := uuid.Parse(in.ID)
			if err == nil {
				id = parsed
			}
		}
		var accountID *uuid.UUID
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) != "" {
			aid, err := uuid.Parse(strings.TrimSpace(*in.AccountID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account_id"})
				return
			}
			accountID = &aid
		}
		if in.IntervalMinutes <= 0 {
			in.IntervalMinutes = 10
		}
		if in.IntervalMinutes > 1440 {
			in.IntervalMinutes = 1440
		}
		jobs := make([]string, 0, len(in.Jobs))
		for _, j := range in.Jobs {
			j = strings.TrimSpace(strings.ToLower(j))
			if j != "" {
				jobs = append(jobs, j)
			}
		}
		if len(jobs) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule chains require at least one job"})
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = "Scheduled chain"
		}
		rows = append(rows, driven.ScheduleChainRow{
			ID:              id,
			UserID:          userIDOrEmpty(r),
			Name:            name,
			AccountID:       accountID,
			Jobs:            jobs,
			IntervalMinutes: in.IntervalMinutes,
			Enabled:         in.Enabled,
			NextRunAt:       now.Add(time.Duration(in.IntervalMinutes) * time.Minute),
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if err := h.Schedules.ReplaceSchedulesByUser(r.Context(), userIDOrEmpty(r), rows); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) listRuns(w http.ResponseWriter, r *http.Request) {
	uid := userIDOrEmpty(r)
	if h.JobStore != nil {
		filter, err := parseJobStoreListFilter(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		filter.UserID = uid
		if filter.AccountID != nil {
			ok, err := h.authorizeAccount(r.Context(), uid, *filter.AccountID)
			if err != nil {
				h.Log.Error("authorize runs account", "err", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
		}
		page, err := h.JobStore.List(r.Context(), filter)
		if err != nil {
			h.Log.Error("list runs", "err", err)
			switch {
			case errors.Is(err, driven.ErrOffsetNotSupported):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cursor_required"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}
			return
		}
		if page.NextCursor != "" {
			w.Header().Set("X-Next-Cursor", page.NextCursor)
		}
		out, err := h.jobStoreListItems(r.Context(), uid, page.Jobs)
		if err != nil {
			h.Log.Error("render runs", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
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
	out := make([]jobRunItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, legacyJobRunItem(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) getRun(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if h.JobStore != nil {
		row, err := h.JobStore.Get(r.Context(), uid, id)
		if err != nil {
			if errors.Is(err, driven.ErrJobNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			h.Log.Error("get run", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		item, err := h.jobStoreItem(r.Context(), uid, *row)
		if err != nil {
			h.Log.Error("render run", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}
	if h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
	row, err := h.JobRuns.GetJobRun(r.Context(), uid, id)
	if err != nil {
		h.Log.Error("get run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, legacyJobRunItem(*row))
}

func (h *Handlers) cancelRun(w http.ResponseWriter, r *http.Request) {
	if h.JobStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cancel requires dynamodb job store"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	row, err := h.JobStore.RequestCancel(r.Context(), userIDOrEmpty(r), id, time.Now().UTC(), h.jobTerminalTTL())
	if err != nil {
		if errors.Is(err, driven.ErrJobNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.Log.Error("cancel run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	item, err := h.jobStoreItem(r.Context(), userIDOrEmpty(r), *row)
	if err != nil {
		h.Log.Error("render cancel run", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusAccepted, item)
}

type jobRunItem struct {
	ID              string          `json:"id"`
	AccountID       *string         `json:"account_id,omitempty"`
	AccountLabel    *string         `json:"account_label,omitempty"`
	JobType         string          `json:"job_type"`
	Trigger         string          `json:"trigger"`
	Status          string          `json:"status"`
	TimeWindowStart *string         `json:"time_window_start,omitempty"`
	TimeWindowEnd   *string         `json:"time_window_end,omitempty"`
	StartedAt       *string         `json:"started_at"`
	FinishedAt      *string         `json:"finished_at"`
	ErrorMessage    *string         `json:"error_message,omitempty"`
	Meta            json.RawMessage `json:"meta_json"`
}

func legacyJobRunItem(row driven.JobRunRow) jobRunItem {
	var accountID *string
	if row.AccountID != nil {
		s := row.AccountID.String()
		accountID = &s
	}
	meta := json.RawMessage("{}")
	if row.MetaJSON != "" {
		meta = json.RawMessage(row.MetaJSON)
	}
	return jobRunItem{
		ID:              row.ID.String(),
		AccountID:       accountID,
		AccountLabel:    row.AccountLabel,
		JobType:         row.JobType,
		Trigger:         row.TriggerKind,
		Status:          row.Status,
		TimeWindowStart: formatTimePtr(row.TimeWindowStart),
		TimeWindowEnd:   formatTimePtr(row.TimeWindowEnd),
		StartedAt:       timePtrString(row.StartedAt),
		FinishedAt:      formatTimePtr(row.FinishedAt),
		ErrorMessage:    row.ErrorMessage,
		Meta:            meta,
	}
}

func parseJobStoreListFilter(r *http.Request) (driven.JobListFilter, error) {
	filter := driven.JobListFilter{
		JobType: strings.TrimSpace(r.URL.Query().Get("job_type")),
		Limit:   50,
		Cursor:  strings.TrimSpace(r.URL.Query().Get("cursor")),
	}
	if aid := strings.TrimSpace(r.URL.Query().Get("account_id")); aid != "" {
		id, err := uuid.Parse(aid)
		if err != nil {
			return filter, fmt.Errorf("bad account_id")
		}
		filter.AccountID = &id
	}
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return filter, fmt.Errorf("bad limit")
		}
		filter.Limit = n
	}
	if filter.Limit < 1 {
		filter.Limit = 1
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return filter, fmt.Errorf("bad offset")
		}
		if n > 0 {
			return filter, fmt.Errorf("cursor_required")
		}
		filter.Offset = n
	}
	return filter, nil
}

func (h *Handlers) authorizeAccount(ctx context.Context, userID, accountID uuid.UUID) (bool, error) {
	if h.Accounts == nil {
		return true, nil
	}
	row, _, err := h.Accounts.GetAccount(ctx, userID, accountID)
	if err != nil {
		return false, err
	}
	return row != nil, nil
}

func (h *Handlers) enqueueAccountJob(ctx context.Context, userID, accountID uuid.UUID, jobType string, payload driven.JobPayload) (*driven.JobRecord, error) {
	if h.JobEnqueuer == nil {
		return nil, fmt.Errorf("job queue not configured")
	}
	return h.JobEnqueuer.Enqueue(ctx, driven.CreateJobInput{
		JobType:     jobType,
		UserID:      userID,
		AccountID:   &accountID,
		TriggerKind: driven.JobTriggerAPI,
		Payload:     payload,
		Now:         time.Now().UTC(),
	})
}

func (h *Handlers) jobStoreListItems(ctx context.Context, userID uuid.UUID, rows []driven.JobRecord) ([]jobRunItem, error) {
	out := make([]jobRunItem, 0, len(rows))
	for _, row := range rows {
		item, err := h.jobStoreItem(ctx, userID, row)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (h *Handlers) jobStoreItem(ctx context.Context, userID uuid.UUID, row driven.JobRecord) (jobRunItem, error) {
	var accountID *string
	var accountLabel *string
	if row.AccountID != nil {
		s := row.AccountID.String()
		accountID = &s
		if h.Accounts != nil {
			account, _, err := h.Accounts.GetAccount(ctx, userID, *row.AccountID)
			if err != nil {
				return jobRunItem{}, err
			}
			if account != nil {
				label := strings.TrimSpace(account.Label)
				if label == "" {
					label = strings.TrimSpace(account.PrimaryEmail)
				}
				if label != "" {
					accountLabel = &label
				}
			}
		}
	}
	return jobRunItem{
		ID:              row.ID.String(),
		AccountID:       accountID,
		AccountLabel:    accountLabel,
		JobType:         row.JobType,
		Trigger:         row.TriggerKind,
		Status:          row.Status,
		TimeWindowStart: formatTimePtr(row.Payload.TimeWindowStart),
		TimeWindowEnd:   formatTimePtr(row.Payload.TimeWindowEnd),
		StartedAt:       formatTimePtr(row.StartedAt),
		FinishedAt:      formatTimePtr(row.FinishedAt),
		ErrorMessage:    row.ErrorMessage,
		Meta:            sanitizeJobMeta(row.Progress.Detail),
	}, nil
}

func sanitizeJobMeta(detail map[string]interface{}) json.RawMessage {
	if len(detail) == 0 {
		return json.RawMessage("{}")
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(raw)
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func timePtrString(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

func (h *Handlers) jobTerminalTTL() time.Duration {
	if h.JobTerminalTTL > 0 {
		return h.JobTerminalTTL
	}
	return 30 * 24 * time.Hour
}

func writeAcceptedJob(w http.ResponseWriter, id uuid.UUID) {
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_run_id": id.String(),
		"status":     "queued",
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
