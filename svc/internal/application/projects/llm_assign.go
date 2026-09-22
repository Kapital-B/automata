package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

// llmBatchSize is how many thread digests go into one completion. Scoring a
// mailbox one message at a time is neither fast nor affordable; the registry
// caps an assign chunk at 25 messages, so one call covers a whole chunk.
const llmBatchSize = 25

const (
	maxLLMSubjectChars = 220
	maxLLMSnippetChars = 600
)

// llmThread is one decision unit offered to the model.
type llmThread struct {
	// ref is an opaque per-call label. The model never sees a uuid, which keeps
	// the prompt small and removes uuid hallucination as a failure mode.
	ref string
	msg driven.MessageRow
}

type llmAssignPayload struct {
	SchemaVersion int `json:"schema_version"`
	Assignments   []struct {
		Ref         string  `json:"ref"`
		ProjectCode string  `json:"project_code"`
		Confidence  float64 `json:"confidence"`
		Reason      string  `json:"reason"`
	} `json:"assignments"`
}

// scoreWithLLM suggests projects for messages the deterministic tier could not
// place. Every suggestion is written provisional with source=llm: Wave 1 §7
// reserves committed for a project code token, so the model never auto-commits.
// Bounds on a model-supplied confidence. Below the floor a suggestion costs
// the operator more attention than it saves; the ceiling keeps even a
// confident answer short of certainty, since only a project code commits.
const (
	provisionalFloor = 0.35
	maxConfidence    = 0.99
)

func (s *AssignService) scoreWithLLM(ctx context.Context, orgID, accountID uuid.UUID, msgs []driven.MessageRow, projects []driven.ProjectRow, runID *uuid.UUID, now time.Time) (int, error) {
	if s.LLM == nil || len(msgs) == 0 {
		return 0, nil
	}
	active := make([]driven.ProjectRow, 0, len(projects))
	for _, p := range projects {
		if p.ArchivedAt == nil {
			active = append(active, p)
		}
	}
	if len(active) == 0 {
		return 0, nil
	}

	// Score at thread level: one representative message per conversation.
	threads := dedupeThreads(msgs)

	assigned := 0
	for start := 0; start < len(threads); start += llmBatchSize {
		end := start + llmBatchSize
		if end > len(threads) {
			end = len(threads)
		}
		batch := threads[start:end]
		parsed, err := s.callAssignLLM(ctx, batch, active)
		if err != nil {
			// A failed batch must not strand the deterministic results already
			// written; leave these threads unscored for the next run.
			return assigned, err
		}
		byRef := map[string]driven.MessageRow{}
		for _, t := range batch {
			byRef[t.ref] = t.msg
		}
		byCode := map[string]driven.ProjectRow{}
		for _, p := range active {
			byCode[strings.ToUpper(strings.TrimSpace(p.Code))] = p
		}

		for _, a := range parsed.Assignments {
			msg, ok := byRef[strings.TrimSpace(a.Ref)]
			if !ok {
				// Unknown ref: the model invented a thread. Drop it.
				continue
			}
			project, ok := byCode[strings.ToUpper(strings.TrimSpace(a.ProjectCode))]
			if !ok {
				// Unknown or archived project code. Drop it.
				continue
			}
			conf := a.Confidence
			if conf <= 0 {
				continue
			}
			if conf > maxConfidence {
				conf = maxConfidence
			}
			if conf < provisionalFloor {
				continue
			}
			reason := strings.TrimSpace(a.Reason)
			if reason == "" {
				reason = "llm:" + project.Code
			}
			if err := s.writeAssignment(ctx, orgID, accountID, msg, project.ID,
				domainprojects.StatusProvisional, reason, domainprojects.SourceLLM,
				&conf, runID, now); err != nil {
				return assigned, err
			}
			assigned++
		}
	}
	return assigned, nil
}

// dedupeThreads keeps one representative message per conversation and labels
// each with a stable per-call ref.
func dedupeThreads(msgs []driven.MessageRow) []llmThread {
	seen := map[string]struct{}{}
	out := make([]llmThread, 0, len(msgs))
	for _, m := range msgs {
		key := m.ID.String()
		if m.ConversationID != nil && strings.TrimSpace(*m.ConversationID) != "" {
			key = "conv:" + strings.TrimSpace(*m.ConversationID)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, llmThread{ref: fmt.Sprintf("t%d", len(out)+1), msg: m})
	}
	return out
}

func (s *AssignService) callAssignLLM(ctx context.Context, batch []llmThread, projects []driven.ProjectRow) (*llmAssignPayload, error) {
	var b strings.Builder
	b.WriteString("PROJECTS (choose only from these codes):\n")
	for _, p := range projects {
		fmt.Fprintf(&b, "- %s: %s", p.Code, p.Name)
		if p.Client != nil && strings.TrimSpace(*p.Client) != "" {
			fmt.Fprintf(&b, " (client: %s)", strings.TrimSpace(*p.Client))
		}
		if p.Description != nil && strings.TrimSpace(*p.Description) != "" {
			fmt.Fprintf(&b, " — %s", clampLLMText(*p.Description, 160))
		}
		if len(p.Keywords) > 0 {
			fmt.Fprintf(&b, " [keywords: %s]", strings.Join(p.Keywords, ", "))
		}
		b.WriteString("\n")
	}

	b.WriteString("\nTHREADS:\n")
	for _, t := range batch {
		body := ""
		if t.msg.BodyText != nil {
			body = *t.msg.BodyText
		}
		if looksLikeHTMLBody(body) {
			body = stripHTMLBody(body)
		}
		fmt.Fprintf(&b, "- ref=%s\n  subject: %s\n  from: %s\n  snippet: %s\n",
			t.ref,
			clampLLMText(t.msg.Subject, maxLLMSubjectChars),
			clampLLMText(senderAddress(t.msg.FromJSON), 120),
			clampLLMText(body, maxLLMSnippetChars))
	}

	system := "You assign work correspondence to engineering projects. " +
		"Choose only from the supplied project codes. Never invent a project. " +
		"Omit any thread you are not confident about rather than guessing. JSON only."
	user := b.String() +
		"\nFor each thread you are confident about, return the project code, a confidence 0..1, " +
		"and a short human-readable reason (one phrase, no reasoning chain).\n" +
		"Omit threads with no clear project.\n" +
		`Return JSON: {"schema_version":1,"assignments":[{"ref":"t1","project_code":"DC01","confidence":0.0,"reason":""}]}`

	resp, err := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
	if err != nil {
		return nil, err
	}
	parsed, err := decodeAssignPayload(resp.Content)
	if err == nil {
		return parsed, nil
	}
	// One repair attempt, mirroring categorize/interpret.
	resp2, err2 := s.LLM.ChatCompletion(ctx, []driven.LLMMessage{
		{Role: "system", Content: "Fix the following into valid JSON only. Keep semantic meaning."},
		{Role: "user", Content: resp.Content},
	})
	if err2 != nil {
		return nil, err
	}
	return decodeAssignPayload(resp2.Content)
}

func decodeAssignPayload(content string) (*llmAssignPayload, error) {
	var out llmAssignPayload
	if err := json.Unmarshal([]byte(normalizeAssignJSON(content)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func normalizeAssignJSON(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```JSON")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(trimmed[start : end+1])
	}
	return trimmed
}

func clampLLMText(s string, maxChars int) string {
	trimmed := strings.TrimSpace(s)
	if maxChars <= 0 || len(trimmed) <= maxChars {
		return trimmed
	}
	return trimmed[:maxChars] + "...[truncated]"
}

func looksLikeHTMLBody(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "<html") || strings.Contains(lower, "<div") || strings.Contains(lower, "<p>")
}

func stripHTMLBody(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
