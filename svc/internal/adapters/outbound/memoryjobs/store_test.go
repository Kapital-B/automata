package memoryjobs_test

import (
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/jobstoretest"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/memoryjobs"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

func TestJobStoreContract(t *testing.T) {
	jobstoretest.RunContractTests(t, func(t *testing.T) (driven.JobStore, func()) {
		t.Helper()
		return memoryjobs.NewStore(), func() {}
	})
}

func TestJobStoreCrashWindows(t *testing.T) {
	jobstoretest.RunCrashWindowTests(t, func(t *testing.T) (driven.JobStore, func()) {
		t.Helper()
		return memoryjobs.NewStore(), func() {}
	})
}
