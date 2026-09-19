package projects

import (
	"strings"
	"testing"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func proj(code, name string, keywords ...string) driven.ProjectRow {
	return driven.ProjectRow{ID: uuid.New(), Code: code, Name: name, Keywords: keywords}
}

func msgWith(subject, body, fromJSON string) driven.MessageRow {
	return driven.MessageRow{ID: uuid.New(), Subject: subject, BodyText: &body, FromJSON: fromJSON}
}

func TestScoreMessageCodeTokenRanksHighest(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade", "chiller")
	ot02 := proj("OT02", "Other")
	msg := msgWith("Regarding DC01", "chiller work", `{"address":"a@b.com"}`)

	got := scoreMessage(msg, []driven.ProjectRow{dc01, ot02}, newSignals(), nil)
	if len(got) == 0 {
		t.Fatal("expected at least one candidate")
	}
	if got[0].Project.ID != dc01.ID {
		t.Fatalf("top candidate = %s, want DC01", got[0].Project.Code)
	}
	if got[0].Confidence < confidenceCodeToken {
		t.Errorf("confidence = %v, want >= %v", got[0].Confidence, confidenceCodeToken)
	}
}

func TestScoreMessageNoSignalsYieldsNothing(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade")
	msg := msgWith("lunch tomorrow?", "nothing relevant", `{"address":"a@b.com"}`)
	if got := scoreMessage(msg, []driven.ProjectRow{dc01}, newSignals(), nil); len(got) != 0 {
		t.Fatalf("expected no candidates, got %+v", got)
	}
}

func TestScoreMessageSenderDomainNeedsSupport(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade")
	projects := []driven.ProjectRow{dc01}
	msg := msgWith("no keywords here", "body", `{"address":"eng@contractor.com"}`)

	// Two prior assignments is a coincidence, not a pattern.
	sig := newSignals()
	for i := 0; i < minDomainSupport-1; i++ {
		sig.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: dc01.ID, FromJSON: `{"address":"eng@contractor.com"}`})
	}
	if got := scoreMessage(msg, projects, sig, nil); len(got) != 0 {
		t.Fatalf("below support threshold should not suggest, got %+v", got)
	}

	// At the threshold it becomes evidence.
	sig.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: dc01.ID, FromJSON: `{"address":"eng@contractor.com"}`})
	got := scoreMessage(msg, projects, sig, nil)
	if len(got) != 1 || got[0].Project.ID != dc01.ID {
		t.Fatalf("expected DC01 suggestion, got %+v", got)
	}
	if !strings.Contains(got[0].Reason, "sender_domain:contractor.com") {
		t.Errorf("reason = %q, want sender_domain", got[0].Reason)
	}
}

func TestScoreMessageSplitDomainIsWeakerThanExclusive(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	ot02 := proj("OT02", "Other")
	projects := []driven.ProjectRow{dc01, ot02}
	from := `{"address":"eng@shared.com"}`

	exclusive := newSignals()
	for i := 0; i < 6; i++ {
		exclusive.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: dc01.ID, FromJSON: from})
	}
	split := newSignals()
	for i := 0; i < 3; i++ {
		split.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: dc01.ID, FromJSON: from})
		split.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: ot02.ID, FromJSON: from})
	}

	msg := msgWith("subject", "body", from)
	exclusiveGot := scoreMessage(msg, projects, exclusive, nil)
	splitGot := scoreMessage(msg, projects, split, nil)
	if len(exclusiveGot) == 0 || len(splitGot) == 0 {
		t.Fatalf("expected candidates: exclusive=%d split=%d", len(exclusiveGot), len(splitGot))
	}
	if splitGot[0].Confidence >= exclusiveGot[0].Confidence {
		t.Errorf("a domain split across projects (%v) should score below an exclusive one (%v)",
			splitGot[0].Confidence, exclusiveGot[0].Confidence)
	}
}

func TestScoreMessageCorroborationBreaksTies(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	ot02 := proj("OT02", "Other")
	projects := []driven.ProjectRow{dc01, ot02}
	from := `{"address":"eng@contractor.com"}`

	// Both codes appear, so both tie on the code signal. Sender history points
	// at OT02, which must therefore win despite sorting second alphabetically.
	sig := newSignals()
	for i := 0; i < minDomainSupport; i++ {
		sig.addAssignmentSignal(driven.AssignmentSignalRow{ProjectID: ot02.ID, FromJSON: from})
	}
	msg := msgWith("DC01 and OT02", "both", from)
	got := scoreMessage(msg, projects, sig, nil)
	if len(got) < 2 {
		t.Fatalf("expected both candidates, got %+v", got)
	}
	if got[0].Project.ID != ot02.ID {
		t.Fatalf("top = %s, want OT02 (corroborated by sender history)", got[0].Project.Code)
	}
}

func TestScoreMessageParticipantOverlap(t *testing.T) {
	dc01 := proj("DC01", "Cooling")
	contactID := uuid.New()
	sig := newSignals()
	sig.addParticipant(driven.ProjectParticipantRow{ProjectID: dc01.ID, ContactID: contactID})

	msg := msgWith("unrelated subject", "body", `{"address":"x@y.com"}`)
	got := scoreMessage(msg, []driven.ProjectRow{dc01}, sig, []uuid.UUID{contactID})
	if len(got) != 1 || got[0].Project.ID != dc01.ID {
		t.Fatalf("expected participant-overlap suggestion, got %+v", got)
	}
	if !strings.Contains(got[0].Reason, "participant_overlap") {
		t.Errorf("reason = %q", got[0].Reason)
	}
}

func TestScoreMessageIgnoresArchivedProjects(t *testing.T) {
	archived := proj("DC01", "Cooling")
	archivedAt := time.Now().UTC()
	archived.ArchivedAt = &archivedAt
	msg := msgWith("Regarding DC01", "body", `{"address":"a@b.com"}`)
	if got := scoreMessage(msg, []driven.ProjectRow{archived}, newSignals(), nil); len(got) != 0 {
		t.Fatalf("archived projects must not be candidates, got %+v", got)
	}
}

func TestScoreMessageIsDeterministic(t *testing.T) {
	dc01 := proj("DC01", "Cooling Upgrade", "chiller")
	ot02 := proj("OT02", "Other", "chiller")
	projects := []driven.ProjectRow{dc01, ot02}
	msg := msgWith("chiller status", "chiller", `{"address":"a@b.com"}`)

	first := scoreMessage(msg, projects, newSignals(), nil)
	for i := 0; i < 20; i++ {
		got := scoreMessage(msg, projects, newSignals(), nil)
		if len(got) != len(first) {
			t.Fatalf("candidate count varied: %d vs %d", len(got), len(first))
		}
		for j := range got {
			if got[j].Project.ID != first[j].Project.ID || got[j].Confidence != first[j].Confidence {
				t.Fatalf("ordering varied at %d: %v vs %v", j, got[j], first[j])
			}
		}
	}
}

func TestSenderDomain(t *testing.T) {
	cases := map[string]string{
		`{"address":"Eng@Contractor.COM"}`:         "contractor.com",
		`{"emailAddress":{"address":"a@b.co.uk"}}`: "b.co.uk",
		`{"name":"no address"}`:                    "",
		``:                                         "",
		`not json`:                                 "",
		`{"address":"malformed"}`:                  "",
		`{"address":"trailing@"}`:                  "",
	}
	for in, want := range cases {
		if got := senderDomain(in); got != want {
			t.Errorf("senderDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
