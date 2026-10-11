package projects

import (
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"testing"

	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

func proj(code, name string, keywords ...string) domainprojects.ProjectRow {
	return domainprojects.ProjectRow{ID: uuid.New(), Code: code, Name: name, Keywords: keywords}
}

func msgWith(subject, body, fromJSON string) domainmessages.MessageRow {
	return domainmessages.MessageRow{ID: uuid.New(), Subject: subject, BodyText: &body, FromJSON: fromJSON}
}

// A project code is the one signal allowed to commit without the operator, so
// it has to be unambiguous: exactly one project, or nothing.
func TestCodeTokenHitNeedsExactlyOneProject(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade")
	ot02 := proj("OT02", "Other")
	all := []domainprojects.ProjectRow{dc01, ot02}

	got, ok := codeTokenHit(msgWith("Regarding DC01", "chiller work", ""), all)
	if !ok || got.ID != dc01.ID {
		t.Fatalf("subject match = %v/%s, want DC01", ok, got.Code)
	}

	if got, ok := codeTokenHit(msgWith("update", "see DC01 and OT02", ""), all); ok {
		t.Errorf("two codes matched %s, want no commit", got.Code)
	}
	if _, ok := codeTokenHit(msgWith("lunch tomorrow?", "nothing relevant", ""), all); ok {
		t.Error("expected no match without a code")
	}
}

func TestCodeTokenHitReadsTheBody(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade")
	got, ok := codeTokenHit(msgWith("no code here", "raised under DC01 last week", ""), []domainprojects.ProjectRow{dc01})
	if !ok || got.ID != dc01.ID {
		t.Fatalf("body match = %v/%s, want DC01", ok, got.Code)
	}
}

func TestSenderAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"flat", `{"address":"A@Example.com"}`, "a@example.com"},
		{"graph shaped", `{"emailAddress":{"address":"b@Example.COM"}}`, "b@example.com"},
		{"empty", "", ""},
		{"not json", "nonsense", ""},
		{"no address", `{"name":"Someone"}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := senderAddress(tc.in); got != tc.want {
				t.Errorf("senderAddress(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
