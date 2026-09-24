package http

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A chain the scheduler cannot run used to be saved anyway (the page even
// suggested one, "auto-draft"), and then stalled the scheduler.
func TestScheduleSaveRefusesChainsThatCannotRunAndStoresCanonicalJobs(t *testing.T) {
	h := newForwardAPI(t)
	put := func(body string) (int, string) {
		return h.do(http.MethodPatch, "/api/settings/schedules", body, nil)
	}
	status, msg := put(`{"chains":[{"id":"` + uuid.NewString() + `","name":"Morning","jobs":["sync","categorise"],"interval_minutes":60,"enabled":true}]}`)
	if status != http.StatusUnprocessableEntity || !strings.Contains(msg, "Morning") || !strings.Contains(msg, "categorise") {
		t.Fatalf("typo: %d %q, want 422 naming the chain and the step", status, msg)
	}
	status, msg = put(`{"chains":[{"id":"` + uuid.NewString() + `","name":"Drafts","jobs":["sync","auto-draft"],"interval_minutes":60,"enabled":true}]}`)
	if status != http.StatusUnprocessableEntity || !strings.Contains(msg, "cannot be scheduled") {
		t.Fatalf("auto-draft: %d %q", status, msg)
	}
	if status, msg = put(`{"chains":[{"id":"` + uuid.NewString() + `","name":"Nightly","jobs":["Sync","forward"],"interval_minutes":60,"enabled":true}]}`); status != http.StatusOK {
		t.Fatalf("valid chain: %d %q", status, msg)
	}
	var got struct {
		Chains []struct {
			Jobs      []string `json:"jobs"`
			NextRunAt string   `json:"next_run_at"`
		} `json:"chains"`
		AvailableJobs []string `json:"available_jobs"`
	}
	h.do(http.MethodGet, "/api/settings/schedules", "", &got)
	if len(got.Chains) != 1 || strings.Join(got.Chains[0].Jobs, ",") != "sync,forward_rules" || got.Chains[0].NextRunAt == "" {
		t.Fatalf("stored chains = %+v", got.Chains)
	}
	if len(got.AvailableJobs) == 0 || got.AvailableJobs[0] != "sync" {
		t.Fatalf("available jobs = %v", got.AvailableJobs)
	}
}
