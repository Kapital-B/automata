package retrieval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

func project(code, name string) driven.ProjectRow {
	return driven.ProjectRow{ID: uuid.New(), Code: code, Name: name}
}

func codes(ps []driven.ProjectRow) string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Code)
	}
	return strings.Join(out, ",")
}

func TestMatchProjects(t *testing.T) {
	projects := []driven.ProjectRow{
		project("DC07", "Data Centre Seven"),
		project("DC09", "Cooling"),
		project("P-100", "Pumps"),
		project("X1", "Ice"), // name too short to match on its own
	}
	for q, want := range map[string]string{
		"What did Jan say about the P-03 seal on DC07?": "DC07",
		"compare dc07 and dc-09":                        "DC07,DC09",
		"status of p100":                                "P-100",
		"what's the cooling load on Data Centre Seven":  "DC07,DC09",
		"is there ice on the roof":                      "",
		"anything on x1":                                "X1",
	} {
		if got := codes(matchProjects(projects, tokenize(q), map[int]bool{})); got != want {
			t.Errorf("%q: got %q, want %q", q, got, want)
		}
	}
}

func TestSearchTermsDropScopesAndStopwords(t *testing.T) {
	toks := tokenize("What did Jan say about the P-03 seal on DC07, and the 2 kW figure?")
	used := map[int]bool{}
	matchProjects([]driven.ProjectRow{project("DC07", "x")}, toks, used)
	for i, tk := range toks {
		if tk.lower == "jan" {
			used[i] = true // as if resolved to a contact
		}
	}
	if got, want := fmt.Sprint(searchTerms(toks, used)), "[p-03 seal 2 figure]"; got != want {
		t.Fatalf("terms = %s, want %s", got, want)
	}
}

func TestContactCandidatesPreferCapitalisedWords(t *testing.T) {
	pick := func(q string) string {
		toks := tokenize(q)
		out := []string{}
		for _, i := range contactCandidates(toks, map[int]bool{}) {
			out = append(out, toks[i].lower)
		}
		return strings.Join(out, ",")
	}
	if got := pick("What did Jan say about the seal update"); got != "jan" {
		t.Errorf("capitalised: got %q, want jan", got)
	}
	// All lowercase: any plain word may be a name, capped at four lookups.
	if got := pick("what did jan say about the seal update today"); got != "jan,seal,update,today" {
		t.Errorf("lowercase: got %q", got)
	}
	// The first word of a sentence is capitalised by grammar, not because it
	// is a name.
	if got := pick("Pumps are noisy"); got != "pumps,noisy" {
		t.Errorf("sentence start: got %q", got)
	}
}

type fakeProjects struct {
	driven.ProjectRepository
	rows      []driven.ProjectRow
	gotFilter driven.ProjectListFilter
}

func (f *fakeProjects) ListProjects(_ context.Context, _ uuid.UUID, filter driven.ProjectListFilter) ([]driven.ProjectRow, error) {
	f.gotFilter = filter
	return f.rows, nil
}

type fakeContacts struct {
	driven.ContactRepository
	rows    []driven.ContactRow
	queries []string
}

func (f *fakeContacts) ListContacts(_ context.Context, _ uuid.UUID, filter driven.ContactListFilter) ([]driven.ContactRow, error) {
	f.queries = append(f.queries, filter.Query)
	var out []driven.ContactRow
	for _, c := range f.rows {
		if strings.Contains(strings.ToLower(c.DisplayName), filter.Query) {
			out = append(out, c)
		}
	}
	return out, nil
}

func TestResolve(t *testing.T) {
	userID := uuid.New()
	projects := &fakeProjects{rows: []driven.ProjectRow{project("DC07", "Seven"), project("DC08", "Eight")}}
	jan := driven.ContactRow{ID: uuid.New(), DisplayName: "Jan de Vries"}
	contacts := &fakeContacts{rows: []driven.ContactRow{
		jan,
		{ID: uuid.New(), DisplayName: "Janet Smith"}, // contains "jan" but is not a Jan
	}}
	r := &Resolver{Projects: projects, Contacts: contacts}
	res, err := r.Resolve(context.Background(), userID, uuid.New(), "What did Jan say about the P-03 seal on DC07?")
	if err != nil {
		t.Fatal(err)
	}
	if projects.gotFilter.MemberUserID == nil || *projects.gotFilter.MemberUserID != userID {
		t.Fatal("projects must be limited to the user's memberships")
	}
	if codes(res.Projects) != "DC07" {
		t.Errorf("projects = %s, want DC07", codes(res.Projects))
	}
	if len(res.Contacts) != 1 || res.Contacts[0].ID != jan.ID {
		t.Errorf("contacts = %+v, want only Jan de Vries", res.Contacts)
	}
	if got := fmt.Sprint(res.Terms); got != "[p-03 seal]" {
		t.Errorf("terms = %s", got)
	}
	if fmt.Sprint(contacts.queries) != "[jan]" {
		t.Errorf("contact lookups = %v, want only the capitalised name", contacts.queries)
	}
	if !res.HasSearch() {
		t.Error("expected something to search for")
	}
}

func TestSnippet(t *testing.T) {
	body := strings.Repeat("filler ", 60) + "The P-03 seal is weeping again. " + strings.Repeat("tail ", 60)
	got := Snippet(body, []string{"seal"}, 80)
	if !strings.Contains(got, "P-03 seal is weeping") || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "…") {
		t.Fatalf("snippet = %q", got)
	}
	if got := Snippet("<p>Short &amp; sweet</p>", nil, 80); got != "Short & sweet" {
		t.Fatalf("html snippet = %q", got)
	}
	if got := Snippet(strings.Repeat("é", 100), []string{"zzz"}, 10); got != strings.Repeat("é", 10)+"…" {
		t.Fatalf("multi-byte snippet = %q", got)
	}
}
