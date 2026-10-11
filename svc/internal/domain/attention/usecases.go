package attention

import (
	context "context"
	uuid "github.com/google/uuid"
	time "time"
)

type AttentionItem struct {
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

type AttentionResult struct {
	Items  []AttentionItem     `json:"items"`
	Counts AttentionViewCounts `json:"counts"`
}

type AttentionViewCounts struct {
	Total               int `json:"total"`
	IssueAssignee       int `json:"issue_assignee"`
	MemberRole          int `json:"member_role"`
	ProvisionalFact     int `json:"provisional_fact"`
	ProvisionalDecision int `json:"provisional_decision"`
	OpenContradiction   int `json:"open_contradiction"`
	MailActionItem      int `json:"mail_action_item"`
}

type AttentionUseCases interface {
	ForUser(ctx context.Context, userID uuid.UUID) (*AttentionResult, error)
	ProjectIDsNeedingInput(ctx context.Context, userID uuid.UUID) (map[uuid.UUID]struct{}, error)
	ForProject(ctx context.Context, userID, projectID uuid.UUID) (*AttentionResult, error)
	CountsForUser(ctx context.Context, userID uuid.UUID) (int, map[uuid.UUID]int, error)
}
