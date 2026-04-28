# PRD Addendum: AI-First Assistant Experience

**Status:** Draft  
**Owner:** Product (single-user / self-hosted context)  
**Last updated:** 2026-04-28  
**Parent PRD:** [docs/prds/initial.md](initial.md)  

## 1. Summary

This addendum extends the initial product direction with an **AI-first assistant home**. The assistant becomes the primary way users ask what needs attention, inspect mail-derived intelligence, draft replies, and initiate safe automation. The existing dashboard pages remain important as structured drill-down surfaces for summaries, inbox, drafts, rules, runs, and settings.

The product should feel less like a generic mailbox dashboard with a chat widget and more like an **operator console where chat is the main control surface**. The assistant answers: **"What should I know, decide, or do next across my accounts?"**

All requirements from the parent PRD still apply. In particular, **multi-account provenance**, **explicit send approval**, **allowlisted forwarding**, and **auditable job runs** remain non-negotiable.

## 2. Problem

The parent PRD centers the dashboard and Today view as the main surfaces for daily intelligence. That is useful, but it still asks users to inspect several structured pages to decide what matters. An AI-first approach can reduce this overhead by making the default experience conversational, state-aware, and action-oriented.

However, if users land on an assistant instead of the Today page, the product must still make outstanding action items impossible to miss. The assistant home must proactively surface open work before waiting for the user to ask.

## 3. Goals

1. Make the assistant the default landing experience for day-to-day use.
2. Preserve the parent PRD's daily intelligence promise: users always know what needs attention.
3. Make every assistant answer involving mail visibly attributable to the source account and source message.
4. Let users complete common workflows from chat: review action items, dismiss FYIs, draft replies, refresh summaries, sync accounts, and create safe rules.
5. Keep high-risk actions explicit and reversible: sending and forwarding require clear confirmation.
6. Keep structured dashboard pages available for detail, audit, and bulk management.

## 4. Non-Goals

- A fully autonomous agent that sends mail or forwards messages without explicit user confirmation.
- Replacing all dashboard views with chat-only UX.
- A general-purpose assistant unrelated to connected account data and configured integrations.
- Slack, Linear, MCP server, or Google Workspace execution in the first assistant phase; these can appear as future connector concepts but must not be presented as available until implemented.
- Training or fine-tuning a model. The assistant uses the same local/OpenAI-compatible LLM assumptions as the parent PRD.

## 5. Design Principles

### 5.1 Assistant as Control Surface

The assistant should be the user's primary control surface for daily work. It can answer questions, propose actions, and launch workflows, but it should not hide the underlying product state. Every meaningful answer should link back to structured objects: messages, action items, drafts, rules, summaries, or runs.

### 5.2 Ambient Attention

If the user lands on chat, outstanding work must still be visible without requiring a prompt. The assistant home should include a compact state summary such as:

- Open action items.
- Drafts ready.
- FYIs.
- Last sync status.
- Connected account count.
- Running or failed jobs when relevant.

### 5.3 Provenance Everywhere

Assistant responses must obey the same provenance requirements as dashboard views:

- Every cited message, action item, draft, and rule includes `account_id` and a user-facing account label or badge.
- Cross-account answers group or badge content by account.
- The assistant must not merge data from multiple accounts without making source identity clear.
- Follow-up actions inherit the same account scope unless the user explicitly changes it.

### 5.4 Confirmation for Side Effects

The assistant can prepare, preview, and explain actions, but must require explicit confirmation for:

- Sending a draft.
- Forwarding a message.
- Creating or enabling a forwarding rule.
- Disconnecting an account.
- Bulk dismissing or marking items done.

For lower-risk actions such as refreshing a summary or syncing an account, the assistant may use a lightweight confirmation or immediate execution depending on UX preference, but it must still show the resulting `job_run` status.

## 6. User-Facing Capabilities

### 6.1 Read and Ask

The assistant should answer natural-language questions over the user's connected mail intelligence:

- "What needs my attention today?"
- "What changed since yesterday?"
- "What invoices came in this week?"
- "Show unanswered emails from Work."
- "What did I miss from newsletters?"
- "Which account is this from?"
- "Why is this marked important?"

Answers should cite source objects inline and include account badges.

### 6.2 Triage and Organize

The assistant should help organize and correct state:

- Categorize recent mail.
- Explain category assignments.
- Let users correct category mistakes.
- Convert message findings into action items.
- Mark action items done.
- Dismiss FYIs.
- Refresh summary for one account or all connected accounts.
- Sync one account or all connected accounts.

### 6.3 Draft and Prepare

The assistant should support reply preparation:

- Draft a reply for a specific message.
- Draft replies for open action items.
- Rewrite drafts in a different tone.
- Save app-local drafts.
- Show draft provenance: source message, account, run, and model when available.
- Send only after explicit user confirmation.

### 6.4 Automate and Configure

The assistant should help configure automation safely:

- Propose forwarding rules from natural language.
- Preview matching messages before creating or enabling a rule.
- Validate forwarding destinations against the allowlist.
- Explain why a rule did or did not run.
- Show forward audit history for a rule or message.
- Create or update schedules for sync/categorize/summarize chains.

### 6.5 Connector Awareness

The assistant home may show connector status chips:

- Email: connected when at least one mailbox is connected.
- Slack, Linear, MCP servers, Google Workspace: future or unavailable until implemented.

Connector chips are discovery aids, not promises of working functionality. Disabled connectors should use clear "coming soon" or "not connected" copy.

## 7. UX Requirements

### 7.1 Home Layout

The default home route should be an AI-first assistant page with:

- A compact attention strip above or near the composer.
- A conversational empty state.
- State-aware suggestion cards.
- A persistent account scope selector inherited from the app shell.
- A visible path to the structured Today/dashboard view.

Suggested attention strip:

- `3 action items`
- `2 drafts ready`
- `5 FYIs`
- `Last sync 4 min ago`
- `2 accounts connected`

### 7.2 State-Aware Suggestions

Suggestions should be generated from real state when available. Examples:

- "Review 3 open action items."
- "Draft replies for 2 messages."
- "Refresh today's summary."
- "Show FYIs from Personal."
- "Create a finance forwarding rule."

When there are no accounts, suggestions must be replaced by a connect-account CTA.

### 7.3 Rich Assistant Cards

Assistant answers should use rich cards for actionable objects:

- **Action item card:** text, due date, account badge, source message link, `Done`, `Draft reply`.
- **FYI card:** text, account badge, source message link, `Dismiss`.
- **Draft card:** subject, body preview, source message, account badge, `Edit`, `Send`, `Discard`.
- **Rule proposal card:** condition, destination, allowlist status, account scope, `Preview matches`, `Create rule`.
- **Run card:** job type, account, status, progress, `View run`.

Plain prose is acceptable for explanation, but action cards should carry the state-changing controls.

### 7.4 Outstanding Action Visibility

When users do not land on Today, action items remain visible through multiple redundant surfaces:

- A persistent attention banner when open action items exist.
- A Today or Action Items navigation badge.
- A composer context chip, for example `3 open actions`.
- Empty-state priority cards before generic prompts.
- Assistant greeting that reflects current state.
- Optional per-account open counts in the account switcher.

The assistant should proactively summarize outstanding work, for example:

> Good morning. You have 3 action items across 2 accounts, 2 FYIs, and 1 draft ready. Want the short version?

### 7.5 Structured Views Remain Available

The Today page remains the structured review surface for users who prefer lists. Inbox, Drafts, Rules, Runs, Accounts, and Settings remain the authoritative places for deeper inspection and bulk management.

## 8. Safety, Trust, and Audit

- The assistant must show account provenance for every mail-derived fact.
- The assistant must not expose access tokens, refresh tokens, raw provider IDs unnecessarily, or full private content in logs.
- Assistant conversations must be persisted in their own conversation/message tables, separate from `llm_artifacts`.
- Assistant conversation content must be encrypted at rest in the database. The service may store non-sensitive metadata in plaintext for listing, filtering, and audit, but user prompts, assistant replies, and model/tool context payloads must be encrypted.
- Conversation content is decrypted only inside the API service when returning authorized conversation responses or when constructing an authorized follow-up assistant request.
- Logs, job metadata, and analytics must not include decrypted conversation bodies.
- Side effects must write or reference durable records where applicable: `job_runs`, drafts, send attempts, forward audits, and rule rows.
- Assistant-initiated jobs should link to `GET /api/runs/{id}` state.
- Sending and forwarding must use explicit confirmation buttons.
- Rule creation must validate the allowlist both at proposal confirmation and execution time.
- Ambiguous account scope must trigger a clarifying question.

### 8.1 Conversation Persistence

Assistant conversations are first-class product state and should use a dedicated persistence model rather than being stored as generic `llm_artifacts`.

Minimum logical model:

- `assistant_conversations`: `id`, `user_id`, optional active `account_id` scope, title or summary metadata, created/updated timestamps, and archived/deleted status.
- `assistant_messages`: `id`, `conversation_id`, role (`user`, `assistant`, `system`, `tool` where needed), encrypted content payload, non-sensitive metadata JSON, created timestamp, and optional links to `run_id`, `message_id`, `draft_id`, `rule_id`, or other durable objects referenced by the message.

Encryption requirements:

- Store prompt text, assistant text, tool-call context, retrieved snippets, and any email-derived conversational payloads only as encrypted ciphertext.
- Use the same general token-vault/security adapter pattern as other encrypted data where practical, but keep conversation encryption independent from provider token storage semantics.
- Decrypt only after authorization checks confirm the requester can access the conversation.
- Return decrypted content through the API only to the authorized client session.
- Do not duplicate decrypted content into `job_runs.meta_json`, logs, traces, or analytics events.

Assistant conversation retention is **90 days by default**. Retention should remain product-configurable, but deletion or archival must remove or render inaccessible the encrypted message payloads for that conversation.

## 9. Success Metrics

- Users can answer "What needs my attention?" from the assistant home without opening Today.
- Users can identify the source account for every assistant-cited item.
- Open action items are visible on initial load when they exist.
- Common tasks can be completed from the assistant in fewer steps than navigating structured pages.
- No draft send or forward occurs without explicit confirmation.
- Failed or running assistant-initiated jobs are visible and recoverable.

## 10. Phased Implementation

### Phase 1 — MVP: State-Aware Assistant Home

**Goal:** Replace the mock assistant landing experience with a real assistant-flavored home that surfaces existing summary/action-item state and keeps Today available as a structured detail page.

**Capabilities:**

- Show a real attention strip on the assistant home:
  - open action item count;
  - FYI count;
  - connected account count;
  - latest summary/run timestamp when available;
  - draft count if already available from the API, otherwise omit rather than show placeholder data.
- Replace static suggestions with state-aware suggestions from current API data.
- Show a connect-account CTA if no accounts are connected.
- Add an action-item banner or priority card when open action items exist.
- Keep provenance visible through account labels/badges on surfaced items.
- Route users to Today, Inbox, Drafts, Runs, or Accounts for deeper detail.

**Out of scope for Phase 1:**

- Free-form LLM chat execution.
- Creating rules from chat.
- Sending drafts from chat.
- Multi-connector assistant behavior beyond email.

**Exit criteria:**

- `/` is a useful AI-first home backed by real account/summary data, not mock replies.
- A user with open action items sees them or a clear count immediately after landing.
- A user with no connected accounts sees a clear connect-account CTA.
- Account provenance is visible for surfaced action items and FYIs.

### Phase 2 — Assistant Answers over Daily Intelligence

**Goal:** Let users ask natural-language questions over existing summaries, action items, FYIs, categories, and messages.

**Capabilities:**

- Persist conversations and messages in dedicated encrypted assistant tables.
- Return conversation history through authorized API responses by decrypting message content server-side.
- Answer a constrained set of read-only questions using existing backend data.
- Cite source messages and account badges.
- Support account scope: all accounts or one selected account.
- Use deterministic tools/data retrieval first; use the LLM for synthesis only after relevant records are selected.
- Show assistant responses as a combination of prose and source cards.

**Exit criteria:**

- New assistant conversations survive page reloads and can be resumed.
- Persisted conversation bodies are encrypted in the database and are not exposed in logs or job metadata.
- "What needs my attention today?" returns current action items with account provenance.
- "What did I miss?" returns FYIs and summary highlights.
- "Show finance items from Work" returns categorized message references.

### Phase 3 — Assistant Triage Actions

**Goal:** Let users complete low-risk triage from the assistant.

**Capabilities:**

- Mark action items done.
- Dismiss FYIs.
- Trigger sync and summary refresh.
- Re-categorize messages or propose category corrections.
- Show resulting run status cards for queued jobs.

**Exit criteria:**

- Assistant actions update durable backend state.
- The UI reconciles immediately with Today and Runs.
- Ambiguous actions ask for account or message clarification.

### Phase 4 — Assistant Draft Workflow

**Goal:** Make reply drafting conversational while preserving explicit send approval.

**Capabilities:**

- Generate drafts for selected messages or open action items.
- Rewrite drafts by tone or length.
- Save app-local drafts.
- Show draft cards with source message and account.
- Send only from an explicit confirmation control.

**Exit criteria:**

- Drafts created through the assistant appear in the Drafts page.
- Sending writes `send_attempts` and shows outcome.
- No send can be triggered by natural language alone without a confirmation UI.

### Phase 5 — Assistant Rule Workflow

**Goal:** Let users create and manage forwarding rules safely from natural language.

**Capabilities:**

- Propose rule conditions from a user request.
- Validate destination against the allowlist.
- Preview matched messages before enabling a rule.
- Create disabled-by-default rules unless the user explicitly enables them.
- Explain forward audit history.

**Exit criteria:**

- No rule can forward to a non-allowlisted address.
- Rule proposals show account scope and provenance.
- Rule execution creates auditable records.

### Phase 6 — Multi-Connector Assistant Foundation

**Goal:** Prepare the assistant for future connectors without diluting v1 email safety.

**Capabilities:**

- Show connector status accurately.
- Add connector-specific source badges.
- Keep account/provider provenance in all assistant answers.
- Support future Google Workspace once provider integration is implemented.
- Reserve space for Slack, Linear, files, and MCP-backed context as later product decisions.

**Exit criteria:**

- The assistant can distinguish available, disconnected, and coming-soon connectors.
- Future connectors can add tools without bypassing the provenance and confirmation model.

## 11. Open Questions

- Should the 90-day assistant conversation retention period be configurable in the UI, environment, or both?
- Should "mark all reviewed" exist as an assistant action, a Today action, both, or neither?
- Should draft generation from chat operate per message only, or also across all open action items?
- What is the minimum structured tool protocol between `web/` and `svc/` for assistant actions?
- Should assistant suggestions be generated server-side so they can reuse product logic and permissions?

---

*This addendum changes the product's primary UX posture from dashboard-first to assistant-first. It does not relax the parent PRD's requirements for provenance, safe sending, allowlisted forwarding, or auditable automation.*
