package projects

import (
	"encoding/json"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// Confidence bands for the deterministic scorer. Wave 1 §7 only commits an
// assignment on a project code token at >= 0.9; everything else is provisional
// and stays operator-correctable.
const (
	confidenceCodeToken    = 0.95
	confidenceNameKeyword  = 0.60
	confidenceSenderDomain = 0.75
	confidenceParticipant  = 0.55

	// provisionalFloor is the weakest signal still worth showing. Below this a
	// suggestion costs the operator more attention than it saves.
	provisionalFloor = 0.35

	// minDomainSupport is how many prior committed assignments a sender domain
	// needs before it counts as a pattern. One prior assignment is a
	// coincidence, not evidence, and a young organisation must not be flooded
	// with confident-looking guesses.
	minDomainSupport = 3

	// corroborationBonus rewards independent signals that agree, breaking ties
	// on evidence instead of alphabetical order.
	corroborationBonus = 0.02
	// maxConfidence keeps a corroborated score below certainty.
	maxConfidence = 0.99
)

// candidate is one scored project for a message.
type candidate struct {
	Project    driven.ProjectRow
	Confidence float64
	Reason     string
}

// signals is the per-run evidence the scorer reuses across every message in a
// chunk. Building it once keeps scoring off the per-message query path.
type signals struct {
	// domainProjects maps a sender domain to how many committed assignments
	// each project has from that domain.
	domainProjects map[string]map[uuid.UUID]int
	// contactProjects maps a contact to the projects they participate in.
	contactProjects map[uuid.UUID][]uuid.UUID
}

func newSignals() *signals {
	return &signals{
		domainProjects:  map[string]map[uuid.UUID]int{},
		contactProjects: map[uuid.UUID][]uuid.UUID{},
	}
}

func (s *signals) addAssignmentSignal(row driven.AssignmentSignalRow) {
	domain := senderDomain(row.FromJSON)
	if domain == "" {
		return
	}
	if s.domainProjects[domain] == nil {
		s.domainProjects[domain] = map[uuid.UUID]int{}
	}
	s.domainProjects[domain][row.ProjectID]++
}

func (s *signals) addParticipant(row driven.ProjectParticipantRow) {
	s.contactProjects[row.ContactID] = append(s.contactProjects[row.ContactID], row.ProjectID)
}

// scoreMessage ranks projects for a message. It never returns a candidate
// below provisionalFloor, and returns candidates sorted strongest first.
//
// Unlike the original rule chain this does not go silent when more than one
// project matches: an ambiguous top candidate is still worth showing as a
// provisional suggestion the operator can reject in one click.
func scoreMessage(msg driven.MessageRow, projects []driven.ProjectRow, sig *signals, contactIDs []uuid.UUID) []candidate {
	body := ""
	if msg.BodyText != nil {
		body = *msg.BodyText
	}
	haystack := msg.Subject + "\n" + body

	type acc struct {
		project    driven.ProjectRow
		confidence float64
		reasons    []string
	}
	scores := map[uuid.UUID]*acc{}
	bump := func(p driven.ProjectRow, conf float64, reason string) {
		if conf <= 0 {
			return
		}
		cur, ok := scores[p.ID]
		if !ok {
			cur = &acc{project: p}
			scores[p.ID] = cur
		}
		// The strongest signal sets the score. Independent signals that agree add
		// a small corroboration bonus, so when two projects tie on the same
		// signal the one with supporting evidence wins rather than whichever
		// code sorts first.
		if conf > cur.confidence {
			cur.confidence = conf
		}
		cur.reasons = append(cur.reasons, reason)
	}

	for _, p := range matchProjectCodes(haystack, projects) {
		bump(p, confidenceCodeToken, "code:"+p.Code)
	}

	nameHits := matchProjectNamesKeywords(msg.Subject, projects)
	if len(nameHits) == 0 {
		nameHits = matchProjectNamesKeywords(haystack, projects)
	}
	for _, p := range nameHits {
		bump(p, confidenceNameKeyword, "name_or_keyword:"+p.Code)
	}

	if sig != nil {
		byID := map[uuid.UUID]driven.ProjectRow{}
		for _, p := range projects {
			if p.ArchivedAt == nil {
				byID[p.ID] = p
			}
		}

		if domain := senderDomain(msg.FromJSON); domain != "" {
			if counts := sig.domainProjects[domain]; len(counts) > 0 {
				total := 0
				for _, n := range counts {
					total += n
				}
				for projectID, n := range counts {
					p, ok := byID[projectID]
					if !ok || n < minDomainSupport {
						continue
					}
					// Scale by how exclusively this domain maps to the project.
					share := float64(n) / float64(total)
					bump(p, confidenceSenderDomain*share, "sender_domain:"+domain)
				}
			}
		}

		seenProject := map[uuid.UUID]bool{}
		for _, cid := range contactIDs {
			for _, projectID := range sig.contactProjects[cid] {
				if seenProject[projectID] {
					continue
				}
				seenProject[projectID] = true
				if p, ok := byID[projectID]; ok {
					bump(p, confidenceParticipant, "participant_overlap")
				}
			}
		}
	}

	out := make([]candidate, 0, len(scores))
	for _, a := range scores {
		reasons := dedupeStrings(a.reasons)
		conf := a.confidence + corroborationBonus*float64(len(reasons)-1)
		if conf > maxConfidence {
			conf = maxConfidence
		}
		if conf < provisionalFloor {
			continue
		}
		out = append(out, candidate{
			Project:    a.project,
			Confidence: conf,
			Reason:     strings.Join(reasons, "+"),
		})
	}
	// Strongest first; ties break on code so ordering is stable across runs.
	sortCandidates(out)
	return out
}

func sortCandidates(in []candidate) {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0; j-- {
			a, b := in[j-1], in[j]
			if a.Confidence > b.Confidence || (a.Confidence == b.Confidence && a.Project.Code <= b.Project.Code) {
				break
			}
			in[j-1], in[j] = in[j], in[j-1]
		}
	}
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// senderDomain pulls the domain from a Graph-shaped from_json payload.
func senderDomain(fromJSON string) string {
	addr := senderAddress(fromJSON)
	if addr == "" {
		return ""
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 || at == len(addr)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(addr[at+1:]))
}

func senderAddress(fromJSON string) string {
	trimmed := strings.TrimSpace(fromJSON)
	if trimmed == "" {
		return ""
	}
	var payload struct {
		Address      string `json:"address"`
		EmailAddress struct {
			Address string `json:"address"`
		} `json:"emailAddress"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return ""
	}
	if payload.Address != "" {
		return strings.ToLower(strings.TrimSpace(payload.Address))
	}
	return strings.ToLower(strings.TrimSpace(payload.EmailAddress.Address))
}

// codeTokenHit reports whether exactly one project code token appears, which is
// the only signal Wave 1 §7 allows to commit without operator confirmation.
func codeTokenHit(msg driven.MessageRow, projects []driven.ProjectRow) (driven.ProjectRow, bool) {
	body := ""
	if msg.BodyText != nil {
		body = *msg.BodyText
	}
	hits := matchProjectCodes(msg.Subject+"\n"+body, projects)
	if len(hits) == 1 {
		return hits[0], true
	}
	return driven.ProjectRow{}, false
}
