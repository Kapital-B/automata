package attention

import (
	"context"
	"sort"
	"strings"
	"time"

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
	// IssueID and IssueTitle name the issue a mail to-do's message is on, so
	// the to-do reads as part of that piece of project work.
	IssueID    string     `json:"issue_id,omitempty"`
	IssueTitle string     `json:"issue_title,omitempty"`
	DueAt      *time.Time `json:"due_at,omitempty"`
	// OccurredAt dates the underlying row, so the list can be ordered by
	// recency within a why_me group.
	OccurredAt time.Time `json:"occurred_at"`
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
	// Assignments places mail to-dos on the project their message is filed
	// to. Optional: without it they stay unplaced.
	Assignments driven.AssignmentRepository
}

// lookupChunk bounds the IN lists sent to the database.
const lookupChunk = 500

func (s *Service) homeOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	return s.Users.GetHomeOrganisationID(ctx, userID)
}

func (s *Service) ForUser(ctx context.Context, userID uuid.UUID) (*Result, error) {
	orgID, err := s.homeOrg(ctx, userID)
	if err != nil {
		return nil, err
	}
	// One query across the caller's projects. This used to loop every project
	// and run a fact-version query per fact; Home makes it the hottest
	// endpoint in the product.
	rows, err := s.Projects.ListAttention(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	out := &Result{Items: make([]Item, 0, len(rows))}
	for _, row := range rows {
		item, ok := attentionItemFromRow(row)
		if !ok {
			continue
		}
		out.Items = append(out.Items, item)
	}
	mail, err := s.mailItems(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	out.Items = append(out.Items, mail...)
	sortItems(out.Items)
	out.Counts = countItems(out.Items)
	return out, nil
}

// attentionItemFromRow formats a repository row into the API shape, keeping
// the titles and ids identical to the per-project implementation.
func attentionItemFromRow(row driven.AttentionRow) (Item, bool) {
	item := Item{
		Title:       row.Title,
		ProjectID:   row.ProjectID.String(),
		ProjectName: row.ProjectName,
		RefType:     row.RefType,
		RefID:       row.RefID.String(),
		OccurredAt:  row.OccurredAt,
	}
	switch row.Kind {
	case "issue_assignee":
		item.ID = "issue:" + row.RefID.String()
		item.WhyMe = WhyIssueAssignee
	case "member_role":
		item.ID = "issue-role:" + row.RefID.String()
		item.WhyMe = WhyMemberRole
	case "provisional_fact":
		item.ID = "fact-version:" + row.RefID.String()
		item.WhyMe = WhyProvisionalFact
		item.Title = "Confirm fact: " + row.Title
	case "provisional_decision":
		item.ID = "decision:" + row.RefID.String()
		item.WhyMe = WhyProvisionalDecision
		item.Title = "Confirm decision: " + truncateTitle(row.Title, 80)
	case "open_contradiction":
		item.ID = "contradiction:" + row.RefID.String()
		item.WhyMe = WhyOpenContradiction
	default:
		return Item{}, false
	}
	return item, true
}

func truncateTitle(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "…"
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
	mail, err := s.mailItems(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	for _, it := range mail {
		if it.ProjectID == projectID.String() {
			out.Items = append(out.Items, it)
		}
	}
	sortItems(out.Items)
	out.Counts = countItems(out.Items)
	return out, nil
}

// mailItems returns the caller's open to-dos from mail. Each is placed on the
// project its message is filed to, and on the issue whose trail carries the
// message, when the caller can see that project. Nothing is stored: filing
// mail elsewhere moves its to-dos with it.
func (s *Service) mailItems(ctx context.Context, userID, orgID uuid.UUID) ([]Item, error) {
	if s == nil || s.Summaries == nil {
		return nil, nil
	}
	rows, err := s.Summaries.ListOpenActionItems(ctx, userID, nil)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	messageIDs := make([]uuid.UUID, 0, len(rows))
	seen := make(map[uuid.UUID]struct{}, len(rows))
	for _, it := range rows {
		if _, ok := seen[it.MessageID]; ok {
			continue
		}
		seen[it.MessageID] = struct{}{}
		messageIDs = append(messageIDs, it.MessageID)
	}
	projectOf, issueOf, err := s.placeMessages(ctx, userID, orgID, messageIDs)
	if err != nil {
		return nil, err
	}
	names := map[uuid.UUID]string{}
	visible := func(projectID uuid.UUID) (string, bool, error) {
		if name, ok := names[projectID]; ok {
			return name, name != "", nil
		}
		names[projectID] = ""
		p, err := s.Projects.GetProject(ctx, orgID, projectID)
		if err != nil || p == nil {
			return "", false, err
		}
		m, err := s.Projects.GetProjectMember(ctx, projectID, userID)
		if err != nil || m == nil {
			return "", false, err
		}
		names[projectID] = p.Name
		return p.Name, true, nil
	}

	out := make([]Item, 0, len(rows))
	for _, it := range rows {
		title := strings.TrimSpace(it.Text)
		if title == "" {
			title = "Open mail action"
		}
		item := Item{
			ID:         "mail:" + it.ID.String(),
			WhyMe:      WhyMailActionItem,
			Title:      title,
			RefType:    "action_item",
			RefID:      it.ID.String(),
			AccountID:  it.AccountID.String(),
			MessageID:  it.MessageID.String(),
			OccurredAt: it.CreatedAt,
			DueAt:      it.DueAt,
		}
		link, onIssue := issueOf[it.MessageID]
		projectID, filed := projectOf[it.MessageID]
		// Mail filed to one project but still on an issue in another has
		// moved on; the filing wins. Unfiled mail on an issue takes the
		// issue's project.
		if !filed && onIssue {
			projectID, filed = link.ProjectID, true
		}
		if onIssue && link.ProjectID != projectID {
			onIssue = false
		}
		if filed {
			name, ok, err := visible(projectID)
			if err != nil {
				return nil, err
			}
			if ok {
				item.ProjectID = projectID.String()
				item.ProjectName = name
				if onIssue {
					item.IssueID = link.IssueID.String()
					item.IssueTitle = link.Title
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

// placeMessages resolves the project and issue for many messages, in chunks.
func (s *Service) placeMessages(ctx context.Context, userID, orgID uuid.UUID, messageIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, map[uuid.UUID]driven.IssueLink, error) {
	projectOf := make(map[uuid.UUID]uuid.UUID, len(messageIDs))
	issueOf := make(map[uuid.UUID]driven.IssueLink)
	for start := 0; start < len(messageIDs); start += lookupChunk {
		chunk := messageIDs[start:min(start+lookupChunk, len(messageIDs))]
		if s.Assignments != nil {
			msgs := make([]driven.MessageRow, len(chunk))
			for i, id := range chunk {
				msgs[i] = driven.MessageRow{ID: id}
			}
			projects, err := s.Assignments.EffectiveProjectIDsForMessages(ctx, userID, msgs)
			if err != nil {
				return nil, nil, err
			}
			for mid, pid := range projects {
				if pid != nil {
					projectOf[mid] = *pid
				}
			}
		}
		if s.Issues != nil {
			links, err := s.Issues.ListIssueLinksForMessages(ctx, orgID, chunk)
			if err != nil {
				return nil, nil, err
			}
			for mid, link := range links {
				issueOf[mid] = link
			}
		}
	}
	return projectOf, issueOf, nil
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
		// Within a severity group, newest first so "recent outstanding" is
		// meaningful. Title breaks remaining ties to keep ordering stable.
		if !items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].OccurredAt.After(items[j].OccurredAt)
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

// CountsForUser returns the total attention count and a per-project breakdown,
// for the Home metric cards and project badges.
//
// Mail to-dos count toward the badge of the project their message is filed
// to; unfiled ones count toward the total only.
func (s *Service) CountsForUser(ctx context.Context, userID uuid.UUID) (int, map[uuid.UUID]int, error) {
	res, err := s.ForUser(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	byProject := map[uuid.UUID]int{}
	for _, it := range res.Items {
		if strings.TrimSpace(it.ProjectID) == "" {
			continue
		}
		id, err := uuid.Parse(it.ProjectID)
		if err != nil {
			continue
		}
		byProject[id]++
	}
	return res.Counts.Total, byProject, nil
}
