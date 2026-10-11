package http

import (
	domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"
	"net/http"
)

func writeSubmission(w http.ResponseWriter, out *domainjobs.Submission, kind string, payload domainjobs.JobPayload) {
	if out.Mode == "durable" {
		writeAcceptedJob(w, out.RunID)
		return
	}
	body := map[string]any{"job_run_id": out.RunID.String(), "status": "queued"}
	status := http.StatusAccepted
	if out.Mode == "inline" {
		status = http.StatusOK
		body["status"] = "success"
		switch kind {
		case "sync", "sync_slack":
			body["messages_upserted"] = out.MessagesUpserted
		case "categorize":
			body["messages_categorized"] = out.MessagesCategorized
		case "draft_suggest":
			body["drafts_generated"] = out.DraftsGenerated
			body["action_items_seen"] = out.ActionItemsSeen
		}
	}
	if kind == "categorize" {
		body["recategorize"] = payload.Recategorize
	}
	writeJSON(w, status, body)
}
