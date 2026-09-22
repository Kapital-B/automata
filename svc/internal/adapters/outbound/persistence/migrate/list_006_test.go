package migrate

import (
	"strings"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
)

// A migration that is not listed for an engine never runs there, and the
// column it adds is missing at runtime rather than at deploy.
func TestContactResolutionMigrationIsListedForEveryEngine(t *testing.T) {
	for _, engine := range []factory.Engine{factory.EngineDSQL, factory.EnginePostgres} {
		migrations, err := List(engine)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range migrations {
			if strings.Contains(m.Path, "006_contact_resolution") {
				found = true
				if !strings.Contains(m.SQL, "contacts_resolved_at") {
					t.Errorf("%s: 006 does not add the column", engine)
				}
			}
		}
		if !found {
			t.Errorf("%s: 006_contact_resolution is not listed", engine)
		}
	}
}
