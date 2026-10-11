package messages

import (
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"time"
)

type MessagePage struct {
	Messages   []MessageRow
	ProjectIDs map[uuid.UUID]*uuid.UUID
}
type SummaryOverview struct {
	Snapshot    *SummarySnapshotRow
	ActionItems []ActionItemRow
	FYI         []FYIRow
}
type ForwardRuleInput struct {
	Name          string
	Mode          string
	ConditionJSON json.RawMessage
	ForwardTo     string
	Enabled       *bool
	Start         string
}
type ForwardRuleListing struct {
	Rules     []ForwardRuleRow
	Allowlist map[string]struct{}
	Stats     map[uuid.UUID]ForwardRuleStats
}

func ForwardStart(start string, now time.Time) (*time.Time, bool) {
	switch strings.ToLower(strings.TrimSpace(start)) {
	case "new":
		return &now, true
	case "existing", "all":
		epoch := time.Unix(0, 0).UTC()
		return &epoch, true
	default:
		return nil, false
	}
}
