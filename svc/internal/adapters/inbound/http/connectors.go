package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	asynqadapter "github.com/Kapital-B/automata/svc/internal/adapters/inbound/asynq"
	appconnectors "github.com/Kapital-B/automata/svc/internal/application/connectors"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handlers) listConnectors(w http.ResponseWriter, r *http.Request) {
	if h.ConnectorSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connectors not configured"})
		return
	}
	rows, err := h.ConnectorSvc.List(r.Context(), userIDOrEmpty(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorAccountJSON(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) startConnectorConnect(w http.ResponseWriter, r *http.Request) {
	if h.ConnectorSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connectors not configured"})
		return
	}
	var body struct {
		Provider string  `json:"provider"`
		Label    *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	result, err := h.ConnectorSvc.StartConnect(r.Context(), userIDOrEmpty(r), appconnectors.StartConnectInput{
		Provider: body.Provider, LabelHint: body.Label,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_url": result.AuthorizationURL,
		"state":             result.State,
	})
}

func (h *Handlers) connectorOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.ConnectorSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connectors not configured"})
		return
	}
	if r.URL.Query().Get("error") != "" {
		h.redirectError(w, r, "access_denied")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		h.redirectError(w, r, "invalid_state")
		return
	}
	result, err := h.ConnectorSvc.CompleteOAuth(r.Context(), code, state)
	if err != nil {
		if errors.Is(err, appconnectors.ErrInvalidOAuthState) {
			h.redirectError(w, r, "invalid_state")
			return
		}
		if h.Log != nil {
			h.Log.Error("complete connector oauth", "err", err)
		}
		h.redirectError(w, r, "token_exchange_failed")
		return
	}
	path := h.ConnectorSuccessPath
	if strings.TrimSpace(path) == "" {
		path = h.SuccessPath
	}
	target := appendURLQuery(h.Dashboard+path, "connected_connector_id", result.ConnectorAccountID.String())
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handlers) deleteConnector(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.ConnectorSvc.Disconnect(r.Context(), userIDOrEmpty(r), id); err != nil {
		if errors.Is(err, appconnectors.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) listConnectorBindings(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.ConnectorSvc.ListBindings(r.Context(), userIDOrEmpty(r), id)
	if err != nil {
		if errors.Is(err, appconnectors.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorBindingJSON(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) createConnectorBinding(w http.ResponseWriter, r *http.Request) {
	connectorAccountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body struct {
		ExternalChannelID string  `json:"external_channel_id"`
		OrganisationID    *string `json:"organisation_id"`
		ProjectID         *string `json:"project_id"`
		Label             string  `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	input := appconnectors.CreateBindingInput{
		ExternalChannelID: body.ExternalChannelID,
		Label:             body.Label,
	}
	if body.OrganisationID != nil && strings.TrimSpace(*body.OrganisationID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.OrganisationID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad organisation_id"})
			return
		}
		input.OrganisationID = &id
	}
	if body.ProjectID != nil && strings.TrimSpace(*body.ProjectID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.ProjectID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad project_id"})
			return
		}
		input.ProjectID = &id
	}
	row, err := h.ConnectorSvc.CreateBinding(r.Context(), userIDOrEmpty(r), connectorAccountID, input)
	if err != nil {
		if errors.Is(err, appconnectors.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, connectorBindingJSON(*row))
}

func (h *Handlers) syncConnector(w http.ResponseWriter, r *http.Request) {
	if h.ConnectorSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connector sync not configured"})
		return
	}
	connectorAccountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	userID := userIDOrEmpty(r)
	if h.JobsInline {
		result, err := h.ConnectorSvc.Sync(r.Context(), userID, connectorAccountID)
		if err != nil {
			if errors.Is(err, appconnectors.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"job_run_id": result.JobRunID.String(), "messages_upserted": result.MessagesUpserted,
			"status": "success",
		})
		return
	}
	account, err := h.ConnectorSvc.List(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	found := false
	for _, row := range account {
		if row.ID == connectorAccountID {
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.JobEnqueuer != nil {
		job, err := h.JobEnqueuer.Enqueue(r.Context(), driven.CreateJobInput{
			JobType:     "sync_slack",
			UserID:      userID,
			TriggerKind: driven.JobTriggerAPI,
			Payload: driven.JobPayload{
				ConnectorAccountID: &connectorAccountID,
			},
			Now: time.Now().UTC(),
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeAcceptedJob(w, job.ID)
		return
	}
	if h.JobQueue == nil || h.JobRuns == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connector sync not configured"})
		return
	}
	runID := uuid.New()
	startedAt := time.Now().UTC()
	meta, _ := json.Marshal(map[string]any{
		"queued": true, "connector_account_id": connectorAccountID.String(),
	})
	if err := h.JobRuns.InsertJobRun(
		r.Context(), runID, uuid.Nil, "sync_slack", "api", "pending",
		startedAt, time.Time{}, nil, string(meta),
	); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	err = h.JobQueue.EnqueueSyncSlack(r.Context(), asynqadapter.TaskPayload{
		SchemaVersion: 1, RunID: runID, UserID: userID,
		ConnectorAccountID: connectorAccountID, TriggerKind: "api",
	})
	if err != nil {
		message := err.Error()
		finishedAt := time.Now().UTC()
		_ = h.JobRuns.UpdateJobRunStatus(r.Context(), runID, "failed", &finishedAt, &message, string(meta))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_run_id": runID.String(), "status": "queued"})
}

func connectorAccountJSON(row driven.ConnectorAccountRow) map[string]any {
	out := map[string]any{
		"id": row.ID.String(), "provider": row.Provider, "label": row.Label,
		"connection_status": row.ConnectionStatus, "scopes": row.Scopes,
		"created_at": row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if row.ExternalTenantID != nil {
		out["external_tenant_id"] = *row.ExternalTenantID
	}
	if row.LastError != nil {
		out["last_error"] = *row.LastError
	}
	if row.LastSyncedAt != nil {
		out["last_synced_at"] = row.LastSyncedAt.UTC().Format(time.RFC3339Nano)
	}
	return out
}

func connectorBindingJSON(row driven.ConnectorBindingRow) map[string]any {
	out := map[string]any{
		"id": row.ID.String(), "connector_account_id": row.ConnectorAccountID.String(),
		"organisation_id": row.OrganisationID.String(), "external_channel_id": row.ExternalChannelID,
		"label": row.Label, "created_at": row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if row.ProjectID != nil {
		out["project_id"] = row.ProjectID.String()
	}
	if row.SyncCursor != nil {
		out["sync_cursor"] = *row.SyncCursor
	}
	return out
}

func appendURLQuery(rawURL, key, value string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
