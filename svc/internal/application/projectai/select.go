package projectai

import (
	"sort"

	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// SelectAskAcrossProjects picks a context budget: projects with open attention first,
// then most recently updated, capped at max. max <= 0 means no cap.
func SelectAskAcrossProjects(projects []domainprojects.ProjectRow, prefer map[uuid.UUID]struct{}, max int) []domainprojects.ProjectRow {
	if len(projects) == 0 {
		return nil
	}
	ranked := append([]domainprojects.ProjectRow(nil), projects...)
	sort.SliceStable(ranked, func(i, j int) bool {
		_, pi := prefer[ranked[i].ID]
		_, pj := prefer[ranked[j].ID]
		if pi != pj {
			return pi
		}
		return ranked[i].UpdatedAt.After(ranked[j].UpdatedAt)
	})
	if max > 0 && len(ranked) > max {
		ranked = ranked[:max]
	}
	return ranked
}

// PutNamedFirst puts the projects a question names ahead of the ranked
// selection, then fills the remaining places from it, capped at max. Named
// projects are not exclusive: a name match can be a false positive, so the
// usual candidates still get the places that are left.
func PutNamedFirst(named, ranked []domainprojects.ProjectRow, max int) []domainprojects.ProjectRow {
	out := make([]domainprojects.ProjectRow, 0, len(named)+len(ranked))
	seen := map[uuid.UUID]bool{}
	for _, group := range [][]domainprojects.ProjectRow{named, ranked} {
		for _, p := range group {
			if seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}
