// Package overview serves the Home portfolio surface: aggregate counts, the
// caller's projects with real last-activity ordering, and a cross-project feed
// of changes to what those projects know.
package overview

import (
	"context"
	domainattention "github.com/Kapital-B/automata/svc/internal/domain/attention"

	domainauth "github.com/Kapital-B/automata/svc/internal/domain/auth"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// DefaultActivityLimit and MaxActivityLimit bound one page of the feed.
const (
	DefaultActivityLimit = 25
	MaxActivityLimit     = 100
	defaultProjectLimit  = 8
)

type Service struct {
	Users     domainauth.UserRepository
	Projects  domainprojects.ProjectRepository
	Attention domainattention.AttentionCounts
	Triage    domainprojects.AssignmentRepository
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

// Get assembles the Home overview.
//
// Each source is one query. The page this backs previously issued a request
// per project from the browser; that fan-out is what this replaces.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (*domainprojects.OverviewResult, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}

	counts, err := s.Projects.CountOverview(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	out := &domainprojects.OverviewResult{Counts: domainprojects.OverviewViewCounts{
		OpenContradictions: counts.OpenContradictions,
		ProvisionalFacts:   counts.ProvisionalFacts,
		ProposedDecisions:  counts.ProposedDecisions,
		ActiveProjects:     counts.ActiveProjects,
	}}

	if s.Triage != nil {
		sum, err := s.Triage.CountUnassignedSummary(ctx, userID)
		if err != nil {
			return nil, err
		}
		out.Counts.TriageUnassigned = sum.Unassigned
		out.Counts.TriageProvisional = sum.Provisional
	}

	var perProject map[uuid.UUID]int
	if s.Attention != nil {
		total, byProject, err := s.Attention.CountsForUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		out.Counts.NeedsYou = total
		perProject = byProject
	}

	projects, err := s.Projects.ListOverviewProjects(ctx, userID, orgID, defaultProjectLimit)
	if err != nil {
		return nil, err
	}
	out.Projects = make([]domainprojects.OverviewViewProject, 0, len(projects))
	for _, p := range projects {
		out.Projects = append(out.Projects, domainprojects.OverviewViewProject{
			ID: p.ID, Code: p.Code, Name: p.Name, Teaser: p.Teaser,
			LastActivityAt: p.LastActivityAt,
			AttentionCount: perProject[p.ID],
		})
	}
	return out, nil
}

// Activity returns one page of the change feed, newest first.
func (s *Service) Activity(ctx context.Context, userID uuid.UUID, in domainprojects.OverviewActivityInput) (*domainprojects.OverviewActivityPage, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultActivityLimit
	}
	if limit > MaxActivityLimit {
		limit = MaxActivityLimit
	}

	items, err := s.Projects.ListActivity(ctx, userID, orgID, domainprojects.ActivityFilter{
		Limit:     limit,
		Before:    in.Before,
		BeforeID:  in.BeforeID,
		Kinds:     in.Kinds,
		ProjectID: in.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	page := &domainprojects.OverviewActivityPage{Items: items}
	// Only offer a cursor when the page was full; a short page is the end.
	if len(items) == limit {
		last := items[len(items)-1]
		at := last.OccurredAt
		id := last.RefID
		page.NextBefore = &at
		page.NextID = &id
	}
	return page, nil
}
