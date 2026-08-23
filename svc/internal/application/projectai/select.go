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
