package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"html"
	"log/slog"
	"net/http"
	"net/url"

	"strconv"
	"strings"
	"time"

	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainattention "github.com/Kapital-B/automata/svc/internal/domain/attention"
	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainconnectors "github.com/Kapital-B/automata/svc/internal/domain/connectors"
	domaincontacts "github.com/Kapital-B/automata/svc/internal/domain/contacts"
	domaindecisions "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domaininterpretations "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"

	domaincontradictions "github.com/Kapital-B/automata/svc/internal/domain/contradictions"

	domainacc "github.com/Kapital-B/automata/svc/internal/domain/accounts"

	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

// Handlers holds wired application services for HTTP.
type Handlers struct {
	Log             *slog.Logger
	AccountSvc      domainaccounts.AccountUseCases
	ConnectorSvc    domainconnectors.ConnectorUseCases
	SyncSvc         domainmessages.MessageSyncUseCases
	CategorizeSvc   domainmessages.MessageCategorizeUseCases
	SummarizeSvc    domainmessages.MessageSummarizeUseCases
	AutoDraftSvc    domainmessages.MessageAutoDraftUseCases
	DraftsSvc       domainmessages.MessageDraftLifecycleUseCases
	ForwardRulesSvc domainmessages.MessageForwardRulesUseCases
	AuthSvc         domainauth.AuthUseCases
	ContactSvc      domaincontacts.ContactUseCases
	ProjectSvc      domainprojects.ProjectUseCases
	IssueSvc        domainissues.IssueUseCases
	TodoSvc         domainissues.TodoUseCases
	FactSvc         domainfacts.FactUseCases
	InterpretSvc    domaininterpretations.InterpretationUseCases
	ReconcileSvc    domaincontradictions.ReconciliationUseCases
	DecisionSvc     domaindecisions.DecisionUseCases
	AttentionSvc    domainattention.AttentionUseCases
	OverviewSvc     domainprojects.OverviewUseCases
	ProjectAISvc    domainprojects.ProjectAIUseCases
	AccountsQueries domainaccounts.AccountsQueries
	MessagesQueries domainmessages.MessagesQueries
	JobRunsQueries  domainjobs.JobRunsQueries
	JobStoreQueries domainjobs.JobStoreQueries

	JobRequests      domainjobs.RequestSubmission
	AccountJobs      domainmessages.AccountJobSubmission
	ConnectorJobs    domainconnectors.SyncSubmission
	Extraction       domainprojects.ExtractionSubmission
	SummariesQueries domainmessages.SummariesQueries
	ForwardsQueries  domainmessages.ForwardsQueries
	SchedulesQueries domainjobs.SchedulesQueries

	UsersQueries domainauth.UsersQueries

	Dashboard            string
	SuccessPath          string
	ConnectorSuccessPath string
	ErrorPath            string
	AuthSuccessPath      string
	AuthErrorPath        string
	StateTTL             time.Duration
	TokenVerifier        domainauth.AccessTokenVerifier
	CORSOrigins          []string
}

func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	// CORS must be registered before routes; chi ignores Use() after route registration.
	if len(h.CORSOrigins) > 0 {
		r.Use(cors.New(cors.Options{
			AllowedOrigins:   h.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Content-Type", "X-Request-ID", "Authorization"},
			ExposedHeaders:   []string{"X-Next-Cursor"},
			AllowCredentials: false,
			MaxAge:           300,
		}).Handler)
	}
	r.Use(requestIDMiddleware)
	if h.Log != nil {
		r.Use(requestLogMiddleware(h.Log))
	}
	r.Use(authMiddleware(h.TokenVerifier))
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
	r.Post("/api/projects/{id}/extract", h.extractProject)
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
	r.Get("/api/overview", h.overview)
	r.Get("/api/activity", h.activity)
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
	r.Get("/api/projects/{id}/todos", h.listProjectTodos)
	r.Post("/api/projects/{id}/todos/{todoID}/done", h.completeProjectTodo)
	r.Get("/api/issues/{id}", h.getIssue)
	r.Patch("/api/issues/{id}", h.updateIssue)
	r.Post("/api/issues/{id}/discard", h.discardIssue)
	r.Post("/api/issues/{id}/items", h.addIssueItem)
	r.Delete("/api/issues/{id}/items/{itemID}", h.removeIssueItem)
	r.Get("/api/unassigned/summary", h.unassignedSummary)
	r.Get("/api/unassigned", h.listUnassigned)
	r.Post("/api/project-assignments/batch", h.assignProjectsBatch)
	r.Post("/api/unassigned/rescan", h.rescanUnassigned)
	r.Post("/api/messages/{id}/project-assignment", h.assignMessageProject)
	r.Delete("/api/messages/{id}/project-assignment/override", h.clearMessageOverride)
	r.Post("/api/manual-items", h.createManualItem)
	r.Post("/api/manual-items/{id}/project-assignment", h.assignManualItemProject)

	r.Get("/api/accounts", h.listAccounts)
	r.Post("/api/accounts", h.startConnect)
	r.Post("/api/accounts/imap", h.connectIMAP)
	r.Get("/api/accounts/providers", h.listMailProviders)
	r.Get("/api/accounts/callback", h.oauthCallback)
	// Google needs its own registered redirect URI; the state decides which
	// provider completes, so both land in the same handler.
	r.Get("/api/accounts/google/callback", h.oauthCallback)
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
	r.Get("/api/forward-rules/{id}/activity", h.forwardRuleActivity)
	r.Post("/api/accounts/{id}/forward-rules/preview", h.previewForwardRule)
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
	rows, err := h.AccountsQueries.ListAccounts(r.Context(), uid)
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
		// Capabilities is absent when the provider is not configured here.
		Capabilities *capabilitiesBody `json:"capabilities,omitempty"`
	}
	out := make([]item, 0, len(rows))
	for _, a := range rows {
		it := item{
			ID:               a.ID.String(),
			Label:            a.Label,
			Provider:         domainaccounts.AccountProviderKey(a.Provider),
			MsAccountKind:    string(a.MsAccountKind),
			PrimaryEmail:     a.PrimaryEmail,
			ConnectionStatus: a.ConnectionStatus,
			LastError:        a.LastError,
		}
		if a.LastSyncedAt != nil {
			s := a.LastSyncedAt.UTC().Format(time.RFC3339Nano)
			it.LastSyncedAt = &s
		}
		if h.AccountSvc != nil {
			if caps, ok := h.AccountSvc.Capabilities(a.Provider); ok {
				body := capabilitiesJSON(caps)
				it.Capabilities = &body
			}
		}
		out = append(out, it)
	}
	writeJSON(w, http.StatusOK, out)
}

type capabilitiesBody struct {
	IncrementalSync   bool `json:"incremental_sync"`
	ServerSideForward bool `json:"server_side_forward"`
	ServerSideReply   bool `json:"server_side_reply"`
	ReportsRemovals   bool `json:"reports_removals"`
}

func capabilitiesJSON(c domainaccounts.MailboxCapabilities) capabilitiesBody {
	return capabilitiesBody{
		IncrementalSync:   c.IncrementalSync,
		ServerSideForward: c.ServerSideForward,
		ServerSideReply:   c.ServerSideReply,
		ReportsRemovals:   c.ReportsRemovals,
	}
}

// listMailProviders tells the UI which mailbox providers this deployment can
// connect, and how, so it never offers one that is not configured.
func (h *Handlers) listMailProviders(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Provider     string           `json:"provider"`
		Connect      string           `json:"connect"`
		Capabilities capabilitiesBody `json:"capabilities"`
	}
	out := []item{}
	if h.AccountSvc != nil {
		for _, p := range h.AccountSvc.Providers() {
			out = append(out, item{Provider: p.Provider, Connect: p.Connect, Capabilities: capabilitiesJSON(p.Capabilities)})
		}
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
	res, err := h.AccountSvc.StartConnect(r.Context(), userIDOrEmpty(r), domainaccounts.AccountStartConnectInput{
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

type mailServerBody struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"`
}

type connectIMAPBody struct {
	Email    string         `json:"email"`
	Username string         `json:"username"`
	Password string         `json:"password"`
	IMAP     mailServerBody `json:"imap"`
	SMTP     mailServerBody `json:"smtp"`
	Label    *string        `json:"label"`
}

// connectIMAP connects a mailbox from typed-in server settings. The servers
// are contacted before anything is stored, so a wrong password or host comes
// back here, as a 422 naming the cause, rather than on the first sync.
func (h *Handlers) connectIMAP(w http.ResponseWriter, r *http.Request) {
	if h.AccountSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account service not configured"})
		return
	}
	var body connectIMAPBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	server := func(b mailServerBody) domainaccounts.MailServer {
		return domainaccounts.MailServer{Host: b.Host, Port: b.Port, Security: b.Security}
	}
	id, err := h.AccountSvc.ConnectWithPassword(r.Context(), userIDOrEmpty(r), domainaccounts.AccountConnectWithPasswordInput{
		Provider: domainaccounts.AccountProviderIMAP,
		Request: domainaccounts.PasswordConnectRequest{
			Email:    body.Email,
			Username: body.Username,
			Password: body.Password,
			IMAP:     server(body.IMAP),
			SMTP:     server(body.SMTP),
		},
		LabelHint: body.Label,
	})
	if err != nil {
		var rejected *domainaccounts.ConnectRejectedError
		if errors.As(err, &rejected) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": rejected.Reason})
			return
		}
		if errors.Is(err, domainaccounts.ErrUnsupportedProvider) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "IMAP accounts are not enabled"})
			return
		}
		h.Log.Error("connect imap", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"account_id": id.String()})
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
		if errors.Is(err, domainaccounts.AccountErrInvalidOAuthState) {
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
	row, err := h.AccountsQueries.GetScopedAccount(r.Context(), userIDOrEmpty(r), id)
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
	force := r.URL.Query().Get("force") == "true"
	result, err := h.AccountJobs.SubmitAccount(r.Context(), uid, id, "sync", domainjobs.JobPayload{Force: force})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeSubmission(w, result, "sync", domainjobs.JobPayload{Force: force})
}

func (h *Handlers) listMessages(w http.ResponseWriter, r *http.Request) {
	filter := domainmessages.MessageListFilter{Category: r.URL.Query().Get("category")}
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
		if n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad offset"})
			return
		}
		filter.Offset = n
	}
	// before_received_at/before_id name the last row of the previous page.
	// Offset paging shifts when mail arrives or is filed between pages, which
	// repeats or skips messages; the keyset does not.
	beforeAt, beforeID := r.URL.Query().Get("before_received_at"), r.URL.Query().Get("before_id")
	if beforeAt != "" || beforeID != "" {
		ts, err := time.Parse(time.RFC3339Nano, beforeAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad before_received_at"})
			return
		}
		id, err := uuid.Parse(beforeID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad before_id"})
			return
		}
		if filter.Offset != 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset cannot be combined with before_received_at/before_id"})
			return
		}
		ts = ts.UTC()
		filter.BeforeReceivedAt = &ts
		filter.BeforeID = &id
	}
	filter.OmitBody = true
	uid := userIDOrEmpty(r)
	page, err := h.MessagesQueries.ListMessagePage(r.Context(), uid, filter)
	if err != nil {
		h.Log.Error("list messages", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	rows, projectIDs := page.Messages, page.ProjectIDs
	type item struct {
		ID                 string          `json:"id"`
		AccountID          string          `json:"account_id"`
		ProviderMessageID  string          `json:"provider_message_id"`
		Subject            string          `json:"subject"`
		ReceivedAt         string          `json:"received_at"`
		HasAttachments     bool            `json:"has_attachments"`
		FromJSON           json.RawMessage `json:"from_json"`
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
			Preview:            preview,
			CategorySlug:       m.CategorySlug,
			CategoryConfidence: m.CategoryConfidence,
			ConversationID:     m.ConversationID,
		}
		if projectID := projectIDs[m.ID]; projectID != nil {
			s := projectID.String()
			it.ProjectID = &s
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
	m, err := h.MessagesQueries.GetMessage(r.Context(), userIDOrEmpty(r), id)
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
	if h.SummariesQueries == nil {
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
	rows, err := h.SummariesQueries.ListDraftSuggestions(r.Context(), uid, accountID, 200)
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

func (h *Handlers) runForwardRules(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	result, err := h.AccountJobs.SubmitAccount(r.Context(), uid, accountID, "forward_rules", domainjobs.JobPayload{})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeSubmission(w, result, "forward_rules", domainjobs.JobPayload{})
}

func (h *Handlers) listDraftAttempts(w http.ResponseWriter, r *http.Request) {
	if h.SummariesQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	draftID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.SummariesQueries.ListSendAttemptsByDraft(r.Context(), userIDOrEmpty(r), draftID, 50)
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
	rows, err := h.MessagesQueries.ListCategoryDefinitions(r.Context(), userIDOrEmpty(r))
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

func toCategoryItem(row domainmessages.CategoryDefinitionRow) categoryItem {
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

// slugPattern allows 1 to 64 characters. It used to demand either exactly one
// or at least three, so a category called "HR" or "IT" could not be created.

func normalizeCategoryInput(body categoryUpsertBody) (categoryUpsertBody, error) {
	in, err := domainmessages.NormalizeCategoryInput(domainmessages.CategoryInput{Slug: body.Slug, DisplayName: body.DisplayName, Definition: body.Definition, SortOrder: body.SortOrder})
	return categoryUpsertBody{Slug: in.Slug, DisplayName: in.DisplayName, Definition: in.Definition, SortOrder: in.SortOrder}, err
}

func (h *Handlers) createCategory(w http.ResponseWriter, r *http.Request) {
	var body categoryUpsertBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	in := domainmessages.CategoryInput{Slug: body.Slug, DisplayName: body.DisplayName, Definition: body.Definition, SortOrder: body.SortOrder}
	row, err := h.MessagesQueries.CreateCategory(r.Context(), userIDOrEmpty(r), in)
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCategoryItem(*row))
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
	in := domainmessages.CategoryInput{Slug: body.Slug, DisplayName: body.DisplayName, Definition: body.Definition, SortOrder: body.SortOrder}
	row, err := h.MessagesQueries.UpdateCategory(r.Context(), userIDOrEmpty(r), id, in)
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCategoryItem(*row))
}

func (h *Handlers) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	err = h.MessagesQueries.DeleteCategory(r.Context(), userIDOrEmpty(r), id, r.URL.Query().Get("replacement_id"))
	if err != nil {
		writeOperationError(w, err)
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
	result, err := h.AccountJobs.SubmitAccount(r.Context(), uid, id, "categorize", domainjobs.JobPayload{Recategorize: body.Recategorize})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeSubmission(w, result, "categorize", domainjobs.JobPayload{Recategorize: body.Recategorize})
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
	result, err := h.AccountJobs.SubmitAccount(r.Context(), uid, id, "summarize", domainjobs.JobPayload{})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeSubmission(w, result, "summarize", domainjobs.JobPayload{})
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
	result, err := h.AccountJobs.SubmitAccount(r.Context(), uid, id, "draft_suggest", domainjobs.JobPayload{MessageID: messageID, Force: messageID != nil})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeSubmission(w, result, "draft_suggest", domainjobs.JobPayload{MessageID: messageID, Force: messageID != nil})
}

func (h *Handlers) listSummaries(w http.ResponseWriter, r *http.Request) {
	if h.SummariesQueries == nil {
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
	overview, err := h.SummariesQueries.GetSummaryOverview(r.Context(), uid, accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if overview.Snapshot == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"snapshot":     nil,
			"action_items": []any{},
			"fyi":          []any{},
		})
		return
	}
	snap, items, fyiRows := *overview.Snapshot, overview.ActionItems, overview.FYI
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
	if h.SummariesQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.SummariesQueries.MarkActionItemDone(r.Context(), userIDOrEmpty(r), id, time.Now().UTC()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) dismissFYI(w http.ResponseWriter, r *http.Request) {
	if h.SummariesQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.SummariesQueries.DeleteFYI(r.Context(), userIDOrEmpty(r), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) getSummarySettings(w http.ResponseWriter, r *http.Request) {
	if h.SummariesQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summaries not configured"})
		return
	}
	row, err := h.SummariesQueries.GetSummarySettings(r.Context(), userIDOrEmpty(r))
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
	if h.SummariesQueries == nil {
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
	if err := h.SummariesQueries.SaveSummarySettings(r.Context(), userIDOrEmpty(r), body.Include, body.Exclude, body.ChunkSize); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) getSchedules(w http.ResponseWriter, r *http.Request) {
	if h.SchedulesQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedules not configured"})
		return
	}
	rows, err := h.SchedulesQueries.ListSchedulesByUser(r.Context(), userIDOrEmpty(r))
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
		item := map[string]any{
			"id":               row.ID.String(),
			"name":             row.Name,
			"account_id":       accountID,
			"jobs":             row.Jobs,
			"interval_minutes": row.IntervalMinutes,
			"enabled":          row.Enabled,
			"next_run_at":      row.NextRunAt.UTC().Format(time.RFC3339),
		}
		if row.LastRunAt != nil {
			item["last_run_at"] = row.LastRunAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	// The page offers exactly the jobs a schedule can run, in pipeline order,
	// rather than a free-text field where a typo schedules nothing.
	writeJSON(w, http.StatusOK, map[string]any{"chains": out, "available_jobs": domainjobs.SchedulableJobs})
}

func (h *Handlers) updateSchedules(w http.ResponseWriter, r *http.Request) {
	if h.SchedulesQueries == nil {
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
	inputs := make([]domainjobs.ScheduleInput, 0, len(body.Chains))
	for _, in := range body.Chains {
		id, _ := uuid.Parse(in.ID)
		var accountID *uuid.UUID
		if in.AccountID != nil && strings.TrimSpace(*in.AccountID) != "" {
			aid, err := uuid.Parse(strings.TrimSpace(*in.AccountID))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account_id"})
				return
			}
			accountID = &aid
		}
		inputs = append(inputs, domainjobs.ScheduleInput{ID: id, Name: in.Name, AccountID: accountID, Jobs: in.Jobs, IntervalMinutes: in.IntervalMinutes, Enabled: in.Enabled})
	}
	if err := h.SchedulesQueries.SaveSchedules(r.Context(), userIDOrEmpty(r), inputs); err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success"})
}

func (h *Handlers) listRuns(w http.ResponseWriter, r *http.Request) {
	uid := userIDOrEmpty(r)
	if h.JobStoreQueries != nil {
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
		page, err := h.JobStoreQueries.List(r.Context(), filter)
		if err != nil {
			h.Log.Error("list runs", "err", err)
			switch {
			case errors.Is(err, domainjobs.ErrOffsetNotSupported):
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
	if h.JobRunsQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
	filter := domainjobs.JobRunListFilter{
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
	rows, err := h.JobRunsQueries.ListJobRuns(r.Context(), uid, filter)
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
	if h.JobStoreQueries != nil {
		row, err := h.JobStoreQueries.Get(r.Context(), uid, id)
		if err != nil {
			if errors.Is(err, domainjobs.ErrJobNotFound) {
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
	if h.JobRunsQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "job runs not configured"})
		return
	}
	row, err := h.JobRunsQueries.GetJobRun(r.Context(), uid, id)
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
	if h.JobStoreQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cancel requires dynamodb job store"})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	row, err := h.JobStoreQueries.CancelRun(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		if errors.Is(err, domainjobs.ErrJobNotFound) {
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

func legacyJobRunItem(row domainjobs.JobRunRow) jobRunItem {
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

func parseJobStoreListFilter(r *http.Request) (domainjobs.JobListFilter, error) {
	filter := domainjobs.JobListFilter{
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

func (h *Handlers) authorizeAccount(ctx context.Context, user, id uuid.UUID) (bool, error) {
	if h.AccountsQueries == nil {
		return true, nil
	}
	return h.AccountsQueries.OwnsAccount(ctx, user, id)
}

func (h *Handlers) jobStoreListItems(ctx context.Context, userID uuid.UUID, rows []domainjobs.JobRecord) ([]jobRunItem, error) {
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

func (h *Handlers) jobStoreItem(ctx context.Context, userID uuid.UUID, row domainjobs.JobRecord) (jobRunItem, error) {
	var accountID *string
	accountLabel := row.AccountLabel
	if row.AccountID != nil {
		value := row.AccountID.String()
		accountID = &value
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
