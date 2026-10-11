package http

import (
	"encoding/json"
	"errors"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"github.com/go-chi/chi/v5"
	"net/http"
	"sort"

	"time"

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
	return domainmessages.ForwardStart(start, now)
}

func writeRuleError(w http.ResponseWriter, err error) bool {
	var re *domainmessages.MessageForwardRuleError
	if errors.As(err, &re) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": re.Reason})
		return true
	}
	return false
}

func (h *Handlers) getForwardAllowlist(w http.ResponseWriter, r *http.Request) {
	if h.ForwardsQueries == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "persistence not configured"})
		return
	}
	rows, err := h.ForwardsQueries.ListForwardAllowlist(r.Context(), userIDOrEmpty(r))
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
	if err := h.ForwardsQueries.SaveForwardAllowlist(r.Context(), userIDOrEmpty(r), body.Emails); err != nil {
		writeOperationError(w, err)
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
	listing, err := h.ForwardsQueries.GetForwardRuleListing(r.Context(), uid, accountID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	rows, allowed, stats := listing.Rules, listing.Allowlist, listing.Stats
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
			"applies_from":   domainmessages.MessageRuleScopeStart(row).Format(time.RFC3339),
			"created_at":     row.CreatedAt.UTC().Format(time.RFC3339),
			"stats": map[string]any{
				"forwarded":         st.Forwarded,
				"failed":            st.Failed,
				"pending":           st.Pending,
				"last_forwarded_at": timeOrNil(st.LastForwardedAt),
				"last_activity_at":  timeOrNil(st.LastActivityAt),
			},
		}
		if block := domainmessages.MessageForwardRuleBlock(row, allowed); block != "" {
			item["blocked_reason"] = block
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) createForwardRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body forwardRuleBody
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	savedID, err := h.ForwardsQueries.SaveForwardRule(r.Context(), userIDOrEmpty(r), id, true, domainmessages.ForwardRuleInput{Name: body.Name, Mode: body.Mode, ConditionJSON: body.ConditionJSON, ForwardTo: body.ForwardTo, Enabled: body.Enabled, Start: body.Start})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": savedID.String()})
}

func (h *Handlers) updateForwardRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	var body forwardRuleBody
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	savedID, err := h.ForwardsQueries.SaveForwardRule(r.Context(), userIDOrEmpty(r), id, false, domainmessages.ForwardRuleInput{Name: body.Name, Mode: body.Mode, ConditionJSON: body.ConditionJSON, ForwardTo: body.ForwardTo, Enabled: body.Enabled, Start: body.Start})
	if err != nil {
		writeOperationError(w, err)
		return
	}
	_ = savedID
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handlers) deleteForwardRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := h.ForwardsQueries.DeleteForwardRule(r.Context(), userIDOrEmpty(r), ruleID); err != nil {
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
	rows, err := h.ForwardsQueries.ListForwardActivity(r.Context(), userIDOrEmpty(r), ruleID, 50)
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
