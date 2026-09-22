package projects

import (
	"encoding/json"
	"strings"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// confidenceCodeToken is the confidence recorded for a project code match,
// the only signal Wave 1 §7 allows to commit without operator confirmation.
const confidenceCodeToken = 0.95

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
