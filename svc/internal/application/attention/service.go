package attention

import (
	"context"
	"sort"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaindec "github.com/Kapital-B/automata/svc/internal/domain/decisions"
	domainfacts "github.com/Kapital-B/automata/svc/internal/domain/facts"
	domainissues "github.com/Kapital-B/automata/svc/internal/domain/issues"
	"github.com/google/uuid"
)

// WhyMe values match wave2 §5.4.
const (
	WhyIssueAssignee       = "issue_assignee"
	WhyMemberRole          = "member_role"
	WhyProvisionalFact     = "provisional_fact"
	WhyProvisionalDecision = "provisional_decision"
	WhyOpenContradiction   = "open_contradiction"
	WhyMailActionItem      = "mail_action_item"
)

type Item struct {
	ID          string `json:"id"`
	WhyMe       string `json:"why_me"`
	Title       string `json:"title"`
	ProjectID   string `json:"project_id,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
	RefType     string `json:"ref_type"`
	RefID       string `json:"ref_id"`
	AccountID   string `json:"account_id,omitempty"`
	MessageID   string `json:"message_id,omitempty"`
}

type Counts struct {
	Total               int `json:"total"`
	IssueAssignee       int `json:"issue_assignee"`
	MemberRole          int `json:"member_role"`
	ProvisionalFact     int `json:"provisional_fact"`
	ProvisionalDecision int `json:"provisional_decision"`
	OpenContradiction   int `json:"open_contradiction"`
	MailActionItem      int `json:"mail_action_item"`
}

type Result struct {
	Items  []Item `json:"items"`
	Counts Counts `json:"counts"`
}

type Service struct {
	Users          driven.UserRepository
	Projects       driven.ProjectRepository
	Issues         driven.IssueRepository
	Facts          driven.FactRepository
	Decisions      driven.DecisionRepository
	Contradictions driven.ContradictionRepository
	Summaries      driven.SummaryRepository
}

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

func (s *Service) ForUser(ctx context.Context, userID uuid.UUID) (*Result, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	projects, err := s.Projects.ListProjects(ctx, orgID, driven.ProjectListFilter{Limit: 200})
	if err != nil {
		return nil, err
	}
	out := &Result{Items: make([]Item, 0)}
	for _, p := range projects {
		m, err := s.Projects.GetProjectMember(ctx, p.ID, userID)
		if err != nil || m == nil {
			continue
		}
		part, err := s.forProject(ctx, userID, orgID, p, m)
		if err != nil {
			return nil, err
		}
		out.Items = append(out.Items, part.Items...)
	}
	if err := s.appendMailItems(ctx, userID, out); err != nil {
		return nil, err
	}
	sortItems(out.Items)
	out.Counts = countItems(out.Items)
	return out, nil
}

// ProjectIDsNeedingInput returns home-org projects that currently have project-scoped attention.
func (s *Service) ProjectIDsNeedingInput(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]struct{}, error) {
	res, err := s.ForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]struct{})
	if res == nil {
		return out, nil
	}
	for _, it := range res.Items {
		if it.WhyMe == WhyMailActionItem || strings.TrimSpace(it.ProjectID) == "" {
			continue
		}
		id, err := uuid.Parse(it.ProjectID)
		if err != nil {
			continue
		}
		out[id] = struct{}{}
	}
	return out, nil
}

func (s *Service) ForProject(ctx context.Context, userID, projectID uuid.UUID) (*Result, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := s.Projects.GetProject(ctx, orgID, projectID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return &Result{Items: []Item{}, Counts: Counts{}}, nil
	}
	m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
	if err != nil || m == nil {
		return &Result{Items: []Item{}, Counts: Counts{}}, nil
	}
	out, err := s.forProject(ctx, userID, orgID, *p, m)
	if err != nil {
		return nil, err
	}
	sortItems(out.Items)
	out.Counts = countItems(out.Items)
	return out, nil
}

func (s *Service) appendMailItems(ctx context.Context, userID uuid.UUID, out *Result) error {
	if s == nil || s.Summaries == nil || out == nil {
		return nil
	}
	items, err := s.Summaries.ListOpenActionItems(ctx, userID, nil)
	if err != nil {
		return err
	}
	for _, it := range items {
		title := strings.TrimSpace(it.Text)
		if title == "" {
			title = "Open mail action"
		}
		out.Items = append(out.Items, Item{
			ID:        "mail:" + it.ID.String(),
			WhyMe:     WhyMailActionItem,
			Title:     title,
			RefType:   "action_item",
			RefID:     it.ID.String(),
			AccountID: it.AccountID.String(),
			MessageID: it.MessageID.String(),
		})
	}
	return nil
}

func (s *Service) forProject(ctx context.Context, userID, orgID uuid.UUID, p driven.ProjectRow, m *driven.ProjectMemberRow) (*Result, error) {
	out := &Result{Items: make([]Item, 0)}
	issues, err := s.Issues.ListIssuesByProject(ctx, orgID, p.ID)
	if err != nil {
		return nil, err
	}
	for _, iss := range issues {
		if iss.Status == string(domainissues.StatusResolved) {
			continue
		}
		if iss.AssigneeUserID != nil && *iss.AssigneeUserID == userID {
			out.Items = append(out.Items, Item{
				ID: "issue:" + iss.ID.String(), WhyMe: WhyIssueAssignee, Title: iss.Title,
				ProjectID: p.ID.String(), ProjectName: p.Name, RefType: "issue", RefID: iss.ID.String(),
			})
		} else if iss.Status == string(domainissues.StatusAwaitingInput) && memberTouchesRole(m) {
			out.Items = append(out.Items, Item{
				ID: "issue-role:" + iss.ID.String(), WhyMe: WhyMemberRole, Title: iss.Title,
				ProjectID: p.ID.String(), ProjectName: p.Name, RefType: "issue", RefID: iss.ID.String(),
			})
		}
	}

	facts, err := s.Facts.ListFactsByProject(ctx, orgID, p.ID)
	if err != nil {
		return nil, err
	}
	for _, f := range facts {
		vers, err := s.Facts.ListFactVersions(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range vers {
			if v.Status != string(domainfacts.StatusProposed) {
				continue
			}
			out.Items = append(out.Items, Item{
				ID: "fact-version:" + v.ID.String(), WhyMe: WhyProvisionalFact,
				Title:     "Confirm fact: " + f.Label,
				ProjectID: p.ID.String(), ProjectName: p.Name, RefType: "fact_version", RefID: v.ID.String(),
			})
		}
	}

	if s.Decisions != nil {
		decs, err := s.Decisions.ListDecisionsByProject(ctx, orgID, p.ID, string(domaindec.StatusProposed))
		if err != nil {
			return nil, err
		}
		for _, d := range decs {
			title := d.Statement
			if len(title) > 80 {
				title = title[:77] + "…"
			}
			mine := d.AssigneeUserID != nil && *d.AssigneeUserID == userID
			why := WhyProvisionalDecision
			if mine {
				why = WhyProvisionalDecision
			}
			out.Items = append(out.Items, Item{
				ID: "decision:" + d.ID.String(), WhyMe: why, Title: "Confirm decision: " + title,
				ProjectID: p.ID.String(), ProjectName: p.Name, RefType: "decision", RefID: d.ID.String(),
			})
		}
	}

	if s.Contradictions != nil {
		contrs, err := s.Contradictions.ListContradictionsByProject(ctx, orgID, p.ID, "open")
		if err != nil {
			return nil, err
		}
		for _, c := range contrs {
			out.Items = append(out.Items, Item{
				ID: "contradiction:" + c.ID.String(), WhyMe: WhyOpenContradiction, Title: c.Summary,
				ProjectID: p.ID.String(), ProjectName: p.Name, RefType: "contradiction", RefID: c.ID.String(),
			})
		}
	}
	return out, nil
}

func memberTouchesRole(m *driven.ProjectMemberRow) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.Role) != "" {
		return true
	}
	if m.Discipline != nil && strings.TrimSpace(*m.Discipline) != "" {
		return true
	}
	if m.CurrentScope != nil && strings.TrimSpace(*m.CurrentScope) != "" {
		return true
	}
	return true // any member can see awaiting_input as role-aware attention
}

func sortItems(items []Item) {
	rank := map[string]int{
		WhyOpenContradiction:   0,
		WhyProvisionalDecision: 1,
		WhyProvisionalFact:     2,
		WhyIssueAssignee:       3,
		WhyMemberRole:          4,
		WhyMailActionItem:      5,
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, okI := rank[items[i].WhyMe]
		if !okI {
			ri = 50
		}
		rj, okJ := rank[items[j].WhyMe]
		if !okJ {
			rj = 50
		}
		if ri != rj {
			return ri < rj
		}
		return items[i].Title < items[j].Title
	})
}

func countItems(items []Item) Counts {
	c := Counts{Total: len(items)}
	for _, it := range items {
		switch it.WhyMe {
		case WhyIssueAssignee:
			c.IssueAssignee++
		case WhyMemberRole:
			c.MemberRole++
		case WhyProvisionalFact:
			c.ProvisionalFact++
		case WhyProvisionalDecision:
			c.ProvisionalDecision++
		case WhyOpenContradiction:
			c.OpenContradiction++
		case WhyMailActionItem:
			c.MailActionItem++
		}
	}
	return c
}
