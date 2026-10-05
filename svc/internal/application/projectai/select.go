package projectai

import (
	"sort"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// SelectAskAcrossProjects picks a context budget: projects with open attention first,
// then most recently updated, capped at max. max <= 0 means no cap.
func SelectAskAcrossProjects(projects []driven.ProjectRow, prefer map[uuid.UUID]struct{}, max int) []driven.ProjectRow {
	if len(projects) == 0 {
		return nil
	}
	ranked := append([]driven.ProjectRow(nil), projects...)
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
func PutNamedFirst(named, ranked []driven.ProjectRow, max int) []driven.ProjectRow {
	out := make([]driven.ProjectRow, 0, len(named)+len(ranked))
	seen := map[uuid.UUID]bool{}
	for _, group := range [][]driven.ProjectRow{named, ranked} {
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
