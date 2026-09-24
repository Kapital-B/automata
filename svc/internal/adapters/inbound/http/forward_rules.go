package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	appmessages "github.com/Kapital-B/automata/svc/internal/application/messages"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// forwardRuleBody is a rule as the client sends it. Start says where a rule
// being switched on begins: "new" (mail from now) or "existing" (all synced
// mail too). It is required when a rule is switched on, so doing that is
// always a deliberate choice about existing mail.
type forwardRuleBody struct {
	Name          string          `json:"name"`
	Mode          string          `json:"mode"`
	ConditionJSON json.RawMessage `json:"condition_json"`
	ForwardTo     string          `json:"forward_to"`
	Enabled       *bool           `json:"enabled"`
	Start         string          `json:"start"`
}

const (
	forwardStartNew      = "new"
	forwardStartExisting = "existing"
)

// forwardStart turns a start choice into the rule's ApplyFrom.
func forwardStart(start string, now time.Time) (*time.Time, bool) {
	switch strings.ToLower(strings.TrimSpace(start)) {
	case forwardStartNew:
		return &now, true
	case forwardStartExisting, "all":
		epoch := time.Unix(0, 0).UTC()
		return &epoch, true
	default:
		return nil, false
	}
}

func writeRuleError(w http.ResponseWriter, err error) bool {
	var re *appmessages.ForwardRuleError
	if errors.As(err, &re) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": re.Reason})
		return true
	}
	return false
}

// validRule checks everything a rule needs, returning its canonical mode and
// condition, or writing the reason it cannot be saved.
func (h *Handlers) validRule(w http.ResponseWriter, r *http.Request, uid uuid.UUID, body forwardRuleBody) (mode, cond, forwardTo string, ok bool) {
	if strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "give the rule a name"})
		return "", "", "", false
	}
	mode, cond, err := appmessages.NormalizeForwardRule(body.Mode, body.ConditionJSON)
	if err != nil {
		if !writeRuleError(w, err) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return "", "", "", false
	}
	forwardTo = strings.ToLower(strings.TrimSpace(body.ForwardTo))
	allowRows, err := h.Forwards.ListForwardAllowlist(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return "", "", "", false
	}
	if _, ok := appmessages.AllowlistSet(allowRows)[forwardTo]; !ok {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the destination must be on your forwarding allowlist"})
		return "", "", "", false
	}
	return mode, cond, forwardTo, true
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
	seen := map[string]bool{}
	emails := make([]string, 0, len(body.Emails))
	for _, e := range body.Emails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" || seen[e] {
			continue
		}
		if !appmessages.ValidForwardAddress(e) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": strconv.Quote(e) + " is not an email address"})
			return
		}
		seen[e] = true
		emails = append(emails, e)
	}
	if err := h.Forwards.ReplaceForwardAllowlist(r.Context(), userIDOrEmpty(r), emails); err != nil {
		h.Log.Error("replace forward allowlist", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func timeOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func (h *Handlers) listForwardRules(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	rows, err := h.Forwards.ListForwardRules(r.Context(), uid, accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	allowRows, err := h.Forwards.ListForwardAllowlist(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	allowed := appmessages.AllowlistSet(allowRows)
	stats := map[uuid.UUID]driven.ForwardRuleStats{}
	if st, err := h.Forwards.ForwardRuleStats(r.Context(), uid, accountID); err == nil {
		for _, s := range st {
			stats[s.RuleID] = s
		}
	} else {
		h.Log.Warn("forward rule stats", "err", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		st := stats[row.ID]
		item := map[string]any{
			"id":             row.ID.String(),
			"account_id":     row.AccountID.String(),
			"name":           row.Name,
			"mode":           row.Mode,
			"condition_json": json.RawMessage(row.ConditionJSON),
			"forward_to":     row.ForwardTo,
			"enabled":        row.Enabled,
			"applies_from":   appmessages.RuleScopeStart(row).Format(time.RFC3339),
			"created_at":     row.CreatedAt.UTC().Format(time.RFC3339),
			"stats": map[string]any{
				"forwarded":         st.Forwarded,
				"failed":            st.Failed,
				"pending":           st.Pending,
				"last_forwarded_at": timeOrNil(st.LastForwardedAt),
				"last_activity_at":  timeOrNil(st.LastActivityAt),
			},
		}
		if block := appmessages.ForwardRuleBlock(row, allowed); block != "" {
			item["blocked_reason"] = block
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) createForwardRule(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, accountID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	var body forwardRuleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	mode, cond, forwardTo, ok := h.validRule(w, r, uid, body)
	if !ok {
		return
	}
	now := time.Now().UTC()
	enabled := body.Enabled != nil && *body.Enabled
	var applyFrom *time.Time
	if enabled {
		if applyFrom, ok = forwardStart(body.Start, now); !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": `say whether the rule covers existing mail: start must be "new" or "existing"`})
			return
		}
	}
	row := driven.ForwardRuleRow{
		ID: uuid.New(), UserID: uid, AccountID: accountID, Name: strings.TrimSpace(body.Name),
		Mode: mode, ConditionJSON: cond, ForwardTo: forwardTo, Enabled: enabled, ApplyFrom: applyFrom,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := h.Forwards.CreateForwardRule(r.Context(), row); err != nil {
		h.Log.Error("create forward rule", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
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
	var body forwardRuleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := userIDOrEmpty(r)
	mode, cond, forwardTo, ok := h.validRule(w, r, uid, body)
	if !ok {
		return
	}
	enabled := body.Enabled != nil && *body.Enabled
	// A start re-scopes the rule; without one it keeps its current scope.
	var applyFrom *time.Time
	if strings.TrimSpace(body.Start) != "" {
		if applyFrom, ok = forwardStart(body.Start, time.Now().UTC()); !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": `start must be "new" or "existing"`})
			return
		}
	}
	row := driven.ForwardRuleRow{
		ID: ruleID, UserID: uid, Name: strings.TrimSpace(body.Name), Mode: mode, ConditionJSON: cond,
		ForwardTo: forwardTo, Enabled: enabled, ApplyFrom: applyFrom, UpdatedAt: time.Now().UTC(),
	}
	if err := h.Forwards.UpdateForwardRule(r.Context(), row); err != nil {
		h.Log.Error("update forward rule", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
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
		h.Log.Error("delete forward rule", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func senderFields(fromJSON string) (string, string) {
	var from struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	_ = json.Unmarshal([]byte(fromJSON), &from)
	return from.Name, from.Address
}

// forwardRuleActivity lists what a rule has sent, failed on or is waiting on.
func (h *Handlers) forwardRuleActivity(w http.ResponseWriter, r *http.Request) {
	ruleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	rows, err := h.Forwards.ListForwardActivity(r.Context(), userIDOrEmpty(r), ruleID, 50)
	if err != nil {
		h.Log.Error("forward rule activity", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		name, addr := senderFields(row.FromJSON)
		reason := ""
		if row.Reason != nil {
			reason = *row.Reason
		}
		out = append(out, map[string]any{
			"message_id":   row.MessageID.String(),
			"account_id":   row.AccountID.String(),
			"subject":      row.Subject,
			"from_name":    name,
			"from_address": addr,
			"received_at":  row.ReceivedAt.UTC().Format(time.RFC3339),
			"status":       row.Status,
			"pending":      row.Pending,
			"attempts":     row.Attempts,
			"reason":       reason,
			"at":           row.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// previewForwardRule says what switching a rule on for existing mail would
// reach, before it is switched on.
func (h *Handlers) previewForwardRule(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	uid := userIDOrEmpty(r)
	if ok, err := h.authorizeAccount(r.Context(), uid, accountID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	} else if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if h.ForwardRulesSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "forwarding not configured"})
		return
	}
	var body forwardRuleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	p, err := h.ForwardRulesSvc.Preview(r.Context(), uid, accountID, body.Mode, body.ConditionJSON, time.Unix(0, 0).UTC())
	if err != nil {
		if writeRuleError(w, err) {
			return
		}
		h.Log.Error("preview forward rule", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	samples := make([]map[string]any, 0, len(p.Samples))
	sort.SliceStable(p.Samples, func(i, j int) bool { return p.Samples[i].ReceivedAt.After(p.Samples[j].ReceivedAt) })
	for _, m := range p.Samples {
		name, addr := senderFields(m.FromJSON)
		samples = append(samples, map[string]any{
			"subject": m.Subject, "from_name": name, "from_address": addr,
			"received_at": m.ReceivedAt.UTC().Format(time.RFC3339),
		})
	}
	out := map[string]any{"in_scope": p.InScope, "capped": p.Capped, "samples": samples}
	if p.Matched != nil {
		out["matched"] = *p.Matched
	}
	writeJSON(w, http.StatusOK, out)
}
