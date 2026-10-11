package asynqtask

import (
	"github.com/google/uuid"
	"time"
)

const (
	QueueSync       = "sync"
	QueueCategorize = "categorize"
	QueueSummarize  = "summarize"
	QueueDraft      = "draft_suggest"
	QueueForward    = "forward_rules"

	TypeSyncV1       = "sync:v1"
	TypeCategorizeV1 = "categorize:v1"
	TypeSummarizeV1  = "summarize:v1"
	TypeDraftV1      = "draft_suggest:v1"
	TypeForwardV1    = "forward_rules:v1"
	TypeSyncSlackV1  = "sync_slack:v1"
)

type Payload struct {
	SchemaVersion      int        `json:"schema_version"`
	RunID              uuid.UUID  `json:"run_id"`
	UserID             uuid.UUID  `json:"user_id"`
	AccountID          uuid.UUID  `json:"account_id"`
	ConnectorAccountID uuid.UUID  `json:"connector_account_id,omitempty"`
	TriggerKind        string     `json:"trigger_kind"`
	MessageID          *uuid.UUID `json:"message_id,omitempty"`
	Force              bool       `json:"force,omitempty"`
	Recategorize       bool       `json:"recategorize,omitempty"`
	RemainingJobs      []string   `json:"remaining_jobs,omitempty"`
	ScheduleID         *uuid.UUID `json:"schedule_id,omitempty"`
	ChainStartedAt     *time.Time `json:"chain_started_at,omitempty"`
}
