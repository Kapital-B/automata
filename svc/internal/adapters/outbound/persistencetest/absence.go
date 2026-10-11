package persistencetest

import (
	"context"
	"database/sql"
	"errors"
	domaincontradictions "github.com/Kapital-B/automata/svc/internal/domain/contradictions"
	domaindecisions "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	domainshared "github.com/Kapital-B/automata/svc/internal/domain/shared"
	"github.com/google/uuid"
	"testing"
	"time"
)

// RunAbsence validates the R0 matrix against each SQL adapter's domain ports.
func RunAbsence(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("error_based_absence", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		id, org, other := uuid.New(), uuid.New(), uuid.New()
		now := time.Now().UTC()
		cases := []struct {
			name string
			run  func() error
		}{
			{"user_by_email", func() error { _, _, err := h.Repo.GetUserByEmail(ctx, "missing@example.com"); return err }},
			{"user_by_id_raw_scan", func() error { _, err := h.Repo.GetUserByID(ctx, id); return err }},
			{"home_organisation", func() error { _, err := h.Repo.GetHomeOrganisationID(ctx, id); return err }},
			{"provider_message", func() error { _, err := h.Repo.GetMessageIDByProvider(ctx, id, "missing"); return err }},
			{"merge_contacts", func() error { return h.Repo.MergeContacts(ctx, org, id, other, now) }},
			{"manual_assignment", func() error {
				return h.Repo.UpdateManualItemAssignment(ctx, org, id, nil, "committed", "reason", "user", nil)
			}},
			{"update_issue", func() error { return h.Repo.UpdateIssue(ctx, domainissues.IssueRow{ID: id, OrganisationID: org}) }},
			{"update_fact", func() error { return h.Repo.UpdateFact(ctx, domainfacts.FactRow{ID: id, OrganisationID: org}) }},
			{"update_fact_version", func() error { return h.Repo.UpdateFactVersion(ctx, domainfacts.FactVersionRow{ID: id, FactID: other}) }},
			{"update_contradiction", func() error {
				return h.Repo.UpdateContradiction(ctx, domaincontradictions.ContradictionRow{ID: id, OrganisationID: org})
			}},
			{"update_decision", func() error {
				return h.Repo.UpdateDecision(ctx, domaindecisions.DecisionRow{ID: id, OrganisationID: org})
			}},
			{"remove_fact_evidence", func() error { return h.Repo.RemoveFactEvidence(ctx, org, id, other) }},
			{"remove_issue_item", func() error { return h.Repo.RemoveIssueItem(ctx, org, id, other) }},
			{"remove_decision_evidence", func() error { return h.Repo.RemoveDecisionEvidence(ctx, org, id, other) }},
			{"interpretation_status", func() error { return h.Repo.UpdateInterpretationStatus(ctx, org, id, "dismissed", now) }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.run()
				if !errors.Is(err, domainshared.ErrNotFound) {
					t.Fatalf("absence = %v, want domain ErrNotFound", err)
				}
				if errors.Is(err, sql.ErrNoRows) {
					t.Fatal("SQL absence escaped the port")
				}
			})
		}
	})
	t.Run("nil_boolean_and_idempotent_absence", func(t *testing.T) {
		h := factory(t)
		ctx := context.Background()
		id, org := uuid.New(), uuid.New()
		a, credential, err := h.Repo.GetAccount(ctx, id, org)
		if err != nil || a != nil || credential != nil {
			t.Fatalf("account absence: %v %v %v", a, credential, err)
		}
		m, err := h.Repo.GetMessage(ctx, id, org)
		if err != nil || m != nil {
			t.Fatalf("message absence: %v %v", m, err)
		}
		f, err := h.Repo.GetFact(ctx, org, id)
		if err != nil || f != nil {
			t.Fatalf("fact absence: %v %v", f, err)
		}
		in, err := h.Repo.GetInterpretation(ctx, org, id)
		if err != nil || in != nil {
			t.Fatalf("interpretation absence: %v %v", in, err)
		}
		uid, ok, err := h.Repo.FindIdentity(ctx, "missing", "missing")
		if err != nil || ok || uid != uuid.Nil {
			t.Fatalf("identity absence: %v %v %v", uid, ok, err)
		}
		_, ok, err = h.Repo.ConsumeAuthSession(ctx, "missing")
		if err != nil || ok {
			t.Fatalf("session absence: %v %v", ok, err)
		}
		_, _, _, ok, err = h.Repo.TakeOAuthState(ctx, "missing")
		if err != nil || ok {
			t.Fatalf("OAuth state absence: %v %v", ok, err)
		}
		if err = h.Repo.DeleteAccount(ctx, id, org); err != nil {
			t.Fatalf("idempotent account delete: %v", err)
		}
		if err = h.Repo.DeleteCategoryDefinition(ctx, id, org); err != nil {
			t.Fatalf("idempotent category delete: %v", err)
		}
		now := time.Now().UTC()
		ok, err = h.Repo.MarkScheduleExecutedIfDue(ctx, id, now, now, now)
		if err != nil || ok {
			t.Fatalf("CAS miss: %v %v", ok, err)
		}
	})
	t.Run("storage_failure_is_not_absence", func(t *testing.T) {
		h := factory(t)
		_ = h.DB.Close()
		_, err := h.Repo.GetUserByID(context.Background(), uuid.New())
		if err == nil || errors.Is(err, domainshared.ErrNotFound) {
			t.Fatalf("closed database error = %v", err)
		}
	})
}
