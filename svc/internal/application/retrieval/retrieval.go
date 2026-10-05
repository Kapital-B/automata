// Package retrieval finds what a question is about without a model call:
// the projects and contacts it names, and the keywords to search mail for.
// It is deterministic so that it costs nothing against the user's AI budget
// and behaves the same way every time.
package retrieval

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/mailtext"
	"github.com/google/uuid"
)

const (
	maxProjects        = 8
	maxContacts        = 5
	maxContactLookups  = 4
	maxTerms           = 8
	minProjectNameLen  = 4
	contactLookupLimit = 20
	memberProjectLimit = 200
)

// Resolution is what a question names, scoped to what the user can see.
type Resolution struct {
	// Projects the user is a member of, named in the question by code or by
	// name. Code matches come first.
	Projects []driven.ProjectRow
	// Contacts in the user's organisation named in the question.
	Contacts []driven.ContactRow
	// Terms are the remaining keywords, lowercased, for SearchMessages.
	Terms []string
}

// ProjectIDs returns the IDs of the resolved projects.
func (r *Resolution) ProjectIDs() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(r.Projects))
	for _, p := range r.Projects {
		out = append(out, p.ID)
	}
	return out
}

// ContactIDs returns the IDs of the resolved contacts.
func (r *Resolution) ContactIDs() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(r.Contacts))
	for _, c := range r.Contacts {
		out = append(out, c.ID)
	}
	return out
}

// HasSearch reports whether there is anything to search mail for.
func (r *Resolution) HasSearch() bool {
	return r != nil && (len(r.Terms) > 0 || len(r.Contacts) > 0)
}

type Resolver struct {
	Projects driven.ProjectRepository
	Contacts driven.ContactRepository
}

// Resolve finds the projects, contacts and keywords in question. Projects
// are limited to those userID is a member of, so a question can never widen
// what the user sees. It issues one project query and at most
// maxContactLookups small contact queries.
func (r *Resolver) Resolve(ctx context.Context, userID, organisationID uuid.UUID, question string) (*Resolution, error) {
	var projects []driven.ProjectRow
	if r.Projects != nil {
		var err error
		projects, err = r.Projects.ListProjects(ctx, organisationID, driven.ProjectListFilter{MemberUserID: &userID, Limit: memberProjectLimit})
		if err != nil {
			return nil, err
		}
	}
	return r.ResolveAmong(ctx, organisationID, projects, question)
}

// ResolveAmong is Resolve for a caller that has already loaded the projects
// the user may see. Only those projects can be matched.
func (r *Resolver) ResolveAmong(ctx context.Context, organisationID uuid.UUID, projects []driven.ProjectRow, question string) (*Resolution, error) {
	toks := tokenize(question)
	res := &Resolution{}
	used := map[int]bool{}
	res.Projects = matchProjects(projects, toks, used)

	if r.Contacts != nil {
		seen := map[uuid.UUID]bool{}
		for _, i := range contactCandidates(toks, used) {
			rows, err := r.Contacts.ListContacts(ctx, organisationID, driven.ContactListFilter{Query: toks[i].lower, Limit: contactLookupLimit})
			if err != nil {
				return nil, err
			}
			for _, c := range rows {
				if seen[c.ID] || !nameHasWord(c.DisplayName, toks[i].lower) {
					continue
				}
				seen[c.ID] = true
				used[i] = true
				res.Contacts = append(res.Contacts, c)
				if len(res.Contacts) == maxContacts {
					break
				}
			}
			if len(res.Contacts) == maxContacts {
				break
			}
		}
	}

	res.Terms = searchTerms(toks, used)
	return res, nil
}

type token struct {
	raw   string // as written, surrounding punctuation trimmed
	lower string
	norm  string // lowercase letters and digits only, for code matching
	start int    // position among words, for sentence-start detection
}

func tokenize(s string) []token {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(",;:!?()[]{}\"'“”‘’", r)
	})
	out := make([]token, 0, len(fields))
	for i, f := range fields {
		f = strings.Trim(f, ".-_/")
		if f == "" {
			continue
		}
		out = append(out, token{raw: f, lower: strings.ToLower(f), norm: normalize(f), start: i})
	}
	return out
}

func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchProjects finds projects by code ("DC07", "dc-07") or by name as a
// whole phrase. Names shorter than minProjectNameLen are ignored, since
// short names collide with ordinary words.
func matchProjects(projects []driven.ProjectRow, toks []token, used map[int]bool) []driven.ProjectRow {
	var byCode, byName []driven.ProjectRow
	matched := map[uuid.UUID]bool{}
	for _, p := range projects {
		code := normalize(p.Code)
		if len(code) < 2 {
			continue
		}
		for i, t := range toks {
			if t.norm == code {
				byCode = append(byCode, p)
				matched[p.ID] = true
				used[i] = true
				break
			}
		}
	}
	for _, p := range projects {
		if matched[p.ID] {
			continue
		}
		name := strings.Fields(strings.ToLower(p.Name))
		if len(strings.Join(name, " ")) < minProjectNameLen {
			continue
		}
		if at := indexPhrase(toks, name); at >= 0 {
			byName = append(byName, p)
			matched[p.ID] = true
			for k := range name {
				used[at+k] = true
			}
		}
	}
	out := append(byCode, byName...)
	if len(out) > maxProjects {
		out = out[:maxProjects]
	}
	return out
}

func indexPhrase(toks []token, phrase []string) int {
	if len(phrase) == 0 {
		return -1
	}
	for i := 0; i+len(phrase) <= len(toks); i++ {
		ok := true
		for k, w := range phrase {
			if toks[i+k].lower != strings.Trim(w, ".-_/") {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// contactCandidates picks the words worth looking up as names. Capitalised
// words other than the first are preferred; if the question has none (typed
// all in lowercase), any plain word is a candidate. Lookups are capped.
func contactCandidates(toks []token, used map[int]bool) []int {
	var capitalised, plain []int
	for i, t := range toks {
		if used[i] || stopwords[t.lower] || utf8.RuneCountInString(t.lower) < 3 || !isAlpha(t.lower) {
			continue
		}
		first, _ := utf8.DecodeRuneInString(t.raw)
		if unicode.IsUpper(first) && t.start > 0 {
			capitalised = append(capitalised, i)
		} else {
			plain = append(plain, i)
		}
	}
	out := capitalised
	if len(out) == 0 {
		out = plain
	}
	if len(out) > maxContactLookups {
		out = out[:maxContactLookups]
	}
	return out
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

func nameHasWord(name, word string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if w == word {
			return true
		}
	}
	return false
}

// searchTerms keeps the words that say what the question is about. Words
// that named a project or contact are scopes, not keywords, and are dropped.
func searchTerms(toks []token, used map[int]bool) []string {
	seen := map[string]bool{}
	var out []string
	for i, t := range toks {
		if used[i] || stopwords[t.lower] || seen[t.lower] {
			continue
		}
		hasDigit := strings.IndexFunc(t.lower, unicode.IsDigit) >= 0
		if utf8.RuneCountInString(t.lower) < 3 && !hasDigit {
			continue
		}
		seen[t.lower] = true
		out = append(out, t.lower)
		if len(out) == maxTerms {
			break
		}
	}
	return out
}

// Snippet returns up to maxRunes of body around the first term it contains,
// as plain text on one line. With no term found it returns the start.
func Snippet(body string, terms []string, maxRunes int) string {
	if mailtext.LooksLikeHTML(body) {
		body = mailtext.StripHTML(body)
	}
	text := []rune(strings.Join(strings.Fields(body), " "))
	if maxRunes <= 0 || len(text) <= maxRunes {
		return string(text)
	}
	lower := []rune(strings.ToLower(string(text)))
	at := -1
	// Lowercasing can change the rune count for a few scripts; positions
	// would then not line up, so fall back to the start of the body.
	for _, t := range terms {
		if len(lower) != len(text) {
			break
		}
		if i := runeIndex(lower, []rune(t)); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	start := 0
	if at > maxRunes/3 {
		start = at - maxRunes/3
	}
	if start+maxRunes > len(text) {
		start = len(text) - maxRunes
	}
	out := string(text[start : start+maxRunes])
	if start > 0 {
		out = "…" + out
	}
	if start+maxRunes < len(text) {
		out += "…"
	}
	return out
}

func runeIndex(s, sub []rune) int {
	if len(sub) == 0 || len(sub) > len(s) {
		return -1
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for k := range sub {
			if s[i+k] != sub[k] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`
		a an the and or but if then than so of to in on at by for from with about into over under
		as is are was were be been being am do does did done doing have has had having can could
		will would shall should may might must not no nor yes it its it's this that these those
		there here what which who whom whose when where why how all any some each every both few
		more most other such own same too very just also only again further once i me my mine we
		us our ours you your yours he him his she her hers they them their theirs
		say said says tell told ask asked know knew need needs want wants get got give gave
		please thanks thank hi hello re fw fwd let lets let's any anything something everything
		since until while during before after above below between up down out off
		email emails mail mails message messages thread threads latest last recent recently
		project projects
	`) {
		m[w] = true
	}
	return m
}()
