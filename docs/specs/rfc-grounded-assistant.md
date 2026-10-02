# RFC: A Grounded Assistant

**Status:** Proposal, not started
**Related:** [AI-first assistant PRD](../prds/addendum-ai-first-assistant.md) (§8.1 conversation persistence still applies), [Aurora DSQL](addendum-aurora-dsql.md) (no extensions), [Triage efficiency](addendum-triage-efficiency.md) §17 (batched hydration)
**Last updated:** 2026-10-02

The assistant on the home page already answers questions through `POST /api/ask` (`projectai.AskAcross`). This RFC asks three questions about it. What would make it useful? How does it get the context to answer? How do we keep it cheap? It then proposes an order to build in.

The AI-first assistant PRD dates from April. It was written around summaries and action items, before projects, facts, issues and to-dos were the centre of the product. Its safety rules still hold: provenance, confirmation before side effects, encrypted conversations. The capabilities it lists don't match the product we have now. This RFC replaces its Phases 2–6.

---

## 1. What exists today

**Two endpoints answer questions with citations.** `POST /api/projects/{id}/ask` (`projectai.Ask`, used by `AskPanel`) and `POST /api/ask` (`projectai.AskAcross`, used by `AssistantHome`). Both build a text context, send it to the model once, and expect JSON with an answer, cited IDs and a confidence. `filterCitations` drops any ID the model wasn't shown, and that rule is worth keeping. When the LLM fails, `heuristicAnswer` falls back to keyword matching.

**The context is a fixed snapshot of each project, chosen without looking at the question.** `buildContext` writes out, for each project:

| Section | Cap (single / across) |
| ------- | --------------------- |
| Active facts, with evidence message IDs | 40 / 16 |
| Accepted decisions | 24 / 8 |
| Open issues | 24 / 8 |
| Recent timeline snippets, ≤240 chars each | 12 / 6 |

`AskAcross` picks at most 8 projects, preferring those that `ProjectIDsNeedingInput` flags as needing attention, then the most recent (`SelectAskAcrossProjects`).

That causes the following failures:

- **The project selection ignores the question.** "What did Jan say about the P-03 seal on DC07?" fails if DC07 isn't in the top 8. It also fails if the email isn't among the last 6–12 timeline items.
- **Older mail is out of reach.** Without search, anything older than the latest dozen timeline items is invisible.
- **The home page's main questions can't be answered.** To-dos, attention items, the unfiled inbox and contacts aren't in the context. "What needs me today?" and "what do I owe Jan?" have nothing to work from.
- **There's no conversation.** Each question stands alone. Nothing is stored, so a follow-up like "and on DC09?" has no referent.
- **The context build is N+1.** One `GetProjectMember` call per project, up to 200. One `GetActiveFactVersion` and one `ListFactEvidence` call per fact. This is the same pattern triage-efficiency §17 removed from the timeline.

**The LLM port can't support any of the cost work.** `driven.LLMResponse` is `{Content string}`. There's no token usage, no tool calls, and no structured output. When the JSON comes back malformed, `callAskLLM` makes a second call to repair it. The Bedrock adapter already uses the Converse API (`bedrock.go`), so moving to tool use doesn't mean replacing the client. It means using what's already there.

**Nothing evaluates answer quality.** `projectai/eval_test.go` tests project selection only.

## 2. What the assistant is for

The user runs engineering projects through correspondence. The structured pages already show any one thing well. The assistant earns its place by **combining things across those pages in one answer**. In order of value:

1. **Where things stand.** "What's the agreed duty on P-03, and who confirmed it?" Facts are versioned and carry evidence, so the answer can include *when the value changed and on whose word*. That's the strongest thing Automata has over a general-purpose assistant.
2. **What's owed.** "What do I owe, and who's waiting on me?" covers open to-dos, unanswered threads and issues blocked on the user, grouped by person or project.
3. **Briefings.** "Prep me for my call with Jan": open issues involving Jan, the last exchange, unresolved contradictions. "What changed on DC07 since Monday?": new facts, decisions and issues.
4. **Finding things.** "The email with the revised pump curve, around March."
5. **Doing things.** Draft the reply, create the to-do, file the email, resolve the issue, each behind a confirmation card (PRD §5.4).

**How answers should look:**

- **A short answer first, then the evidence as cards.** The cards are existing objects (fact, message, issue, to-do), each badged with project and account.
- **When it can't answer, say what's missing.** For example: "There's no recorded fact for the P-03 duty; the closest is Jan's email of 12 Sep."
- **Lead with a briefing.** The most useful turn is often unprompted. The home page should open with "3 things changed on your projects since yesterday", and chat is for following up on it.

## 3. Non-goals

- Sending, forwarding or enabling rules without a confirmation control. PRD §5.4 still applies.
- Answering about anything outside connected data: no web search, no general knowledge.
- Fine-tuning or training.
- Connectors beyond mail and Slack until they exist (PRD Phase 6).

## 4. Proposal: getting the right context

### 4.1 Three tiers of data

| Tier | What | Cost to read | Use |
| ---- | ---- | ------------ | --- |
| 1. Derived | Facts and their versions, decisions, issues, to-dos, attention, summaries | Low. Already distilled, and the extraction cost is paid once at ingestion | Answer from here first |
| 2. Search | Finding messages and manual items | One query | When tier 1 can't answer, or the user asks to find something |
| 3. Raw | Full message bodies | High (tokens) | Only the few messages search returns, capped in length |

The more questions tier 1 can answer, the cheaper and more reliable the assistant gets. Section 5 builds on this.

### 4.2 Tools instead of a fixed context

The model asks for what it needs. Each tool wraps a query the repository already has, and **enforces membership and account scope itself**, so the model can't see anything the user can't.

| Tool | Backed by | Notes |
| ---- | --------- | ----- |
| `resolve(text)` | Project code regex, contact names and identities, issue titles | Deterministic, no LLM. Turns "Jan", "DC07" or "the pump" into IDs before anything else runs |
| `project_brief(project_id)` | Today's `buildContext`, batched | Fixes the N+1 (§1) with one statement per relation |
| `fact_history(fact_id)` | Fact versions plus evidence | The "on whose word" answer |
| `timeline(project_id, since?, contact_id?)` | `ListProjectTimeline` | Already batched (triage-efficiency §17) |
| `attention()` | Attention service | "What needs me" |
| `todos(scope)` | To-do repository | Personal and project-shared |
| `search_messages(query, project_id?, contact_id?, since?)` | **New**, see §4.3 | Returns IDs, subjects and snippets, never full bodies |
| `get_message(id)` | `GetMessage` | Body capped (e.g. 4k chars), HTML stripped |

Tool results carry IDs, so citations keep working as they do now. They're checked against everything the conversation has actually returned, not against a single context pack.

### 4.3 Search on DSQL

Aurora DSQL supports no extensions ([addendum-aurora-dsql.md](addendum-aurora-dsql.md) §97), so `pgvector` and `pg_trgm` are out. Whether `tsvector` and GIN indexes work isn't documented there and needs testing. The options:

| Option | Quality | Cost to build and run | When |
| ------ | ------- | --------------------- | ---- |
| a. `ILIKE` over subject and preview, scoped by project, contact or date | Keyword only, no ranking | Nothing new | First |
| b. Rank in Go over the rows from (a) | Basic relevance ranking | Small | With (a) |
| c. A separate index (OpenSearch Serverless or S3 Vectors) fed by the sync job | Keyword plus semantic | A new service, an ingestion path and a deletion path | Only once evals show search quality is the bottleneck |

Start with (a)+(b). Scoping by project and contact keeps the scanned set small, and one organisation's mail is modest. Option (c) can come later behind the same `search_messages` tool, so nothing above it changes.

### 4.4 Project briefs

Keep a short summary per project (around 300 tokens: current state, open issues, last notable change), stored and regenerated when its facts, decisions or issues change. The extraction debounce (`ListProjectsDueForExtraction`) already finds "projects with new material, now quiet", which is the right trigger. Generate briefs in the same job.

Questions across projects then read many short briefs instead of 8 full snapshots. That's cheaper, and it removes the 8-project cap that makes `AskAcross` miss projects.

### 4.5 Conversations

PRD §8.1 stands: `assistant_conversations` and `assistant_messages`, with content encrypted using the existing `AESGCMVault` pattern, a 90-day retention, and nothing decrypted in logs or `meta_json`.

Additions:
- **Store tool results by reference.** Persist the IDs a tool returned plus a short summary, not the raw text. On resume, re-fetch what's needed. Stored data stays small and never goes stale, and an archived message isn't kept alive inside a chat.
- **Compress old turns.** Past a size threshold, summarise the older turns into one block. The cited IDs survive, so earlier references still resolve.
- **Account and project scope persist on the conversation.** Follow-ups inherit it (PRD §5.3). Ambiguous scope triggers a clarifying question.

## 5. Proposal: keeping it cheap

### 5.1 Measure first

Add usage to the port:

```go
type LLMUsage struct {
	InputTokens, OutputTokens, CacheReadTokens, CacheWriteTokens int
	Model string
}
type LLMResponse struct {
	Content string
	Usage   LLMUsage
}
```

Converse returns these counts, so the Bedrock adapter only needs to copy them across. Record them in each run's `meta_json`, which `project_ai` runs already write. Nothing else in this section can be judged without these numbers.

### 5.2 Answer fixed questions without the LLM

"What needs me?", "my to-dos" and "what's open on DC07" are queries, not reasoning. Detect them cheaply and render the cards directly. At most, add a one-line summary. These are likely the most common questions on the home page.

### 5.3 Prompt caching

Order each request so the parts that don't change come first: tool definitions, then the system prompt, then the user's project list (and briefs), then the conversation. Mark a cache breakpoint at the end of that stable prefix.

- Cache reads cost about 0.1× base input. Writes cost 1.25× with a 5-minute TTL, or 2× with a 1-hour TTL. Two requests sharing a prefix within 5 minutes already pays for itself. A conversation with follow-ups is exactly that.
- **Any byte change in the prefix stops it being reused.** No timestamps, unsorted maps or per-request IDs before the breakpoint. Dates like "today is" go after it.
- The minimum cacheable prefix depends on the model: 512 tokens on the newest Opus, 1024 on Sonnet 5, **4096 on Haiku 4.5**. A short routing prompt on Haiku won't be cached at all.
- Confirm caching works by checking that cache-read tokens (§5.1) are non-zero on follow-ups.

Bedrock supports prompt caching. Whether the Converse adapter or a move to the Bedrock Messages endpoint (Anthropic's Mantle client) is the better path should be settled in slice 3. Note that tool search, for example, is InvokeModel-only on Bedrock, not Converse.

### 5.4 Structured output instead of "JSON only"

The final answer should come back as a strict tool call or as structured output (`strict: true` / `output_config.format`, both supported on Bedrock). That removes the JSON-repair call in `callAskLLM` and the `normalizeJSON` workarounds.

### 5.5 Model choice: measure before tiering

The obvious design is a small model (Haiku 4.5) for routing and a larger one for the answer. Two things to weigh first:

- **Caches are per model.** A two-model setup builds two caches and reuses neither across the switch.
- **A capable model at low effort** often matches a cheaper model's quality at a similar cost per *completed* answer, especially when the cheap model needs extra turns.

So: build the eval set (§8) first, then compare (i) one model at low/medium effort against (ii) a small-plus-large cascade, on cost per completed answer. Bedrock prices differ from Anthropic's list prices, so compare using Bedrock's pricing page.

### 5.6 Hard limits

- At most 6 tool calls per answer, and a context token cap per answer.
- `get_message` bodies capped and HTML stripped.
- A daily token budget per user. Once it's spent, the assistant falls back to deterministic cards ("here's what matched; detailed answers resume tomorrow") instead of erroring.

### 5.7 Pay at ingestion, not per question

Facts, decisions, issues and briefs are extracted once and read many times. Each improvement to extraction makes questions cheaper too. When an eval question fails because a fact was never extracted, fix the extraction, not the assistant's prompt.

## 6. Decisions needed

1. **Search service.** Is a service outside DSQL and DynamoDB acceptable (§4.3c)? This RFC doesn't need it to start, but it caps search quality later.
2. **A monthly LLM budget.** It sets the daily per-user cap (§5.6) and decides §5.5.
3. **Converse or the Bedrock Messages endpoint.** Converse is already wired. The Messages endpoint gives the full Anthropic request shape (caching controls, structured outputs, tool search). Decide in slice 3, using the slice 1 numbers.
4. **Where suggestions come from.** PRD open question 5. This RFC proposes the server (§2, briefing), so it can use permissions and project briefs.

## 7. Slices

| # | Slice | Changes behaviour? | Depends on |
| - | ----- | ------------------ | ---------- |
| 1 | **Foundations.** Usage in `LLMResponse` and run meta; batch `buildContext`; eval set (§8) | No | none |
| 2 | **Retrieval.** `resolve`; `search_messages` (ILIKE + ranking); `AskAcross` picks projects from what `resolve` finds in the question, falling back to attention | Yes: better project choice, older mail reachable | 1 |
| 3 | **Tool loop.** Converse tool use; strict final answer; prompt caching; conversations persisted and encrypted | Yes: follow-ups, lower cost per turn | 1, 2 |
| 4 | **Fixed questions.** Deterministic routes for the common intents (§5.2) | Yes: instant, no LLM | 3 |
| 5 | **Briefs.** Per-project brief in the extraction job; across-project questions read briefs; home page opens with "since yesterday" | Yes | 3 |
| 6 | **Actions.** Draft, to-do, file to project, resolve issue, each with a confirmation card | Yes | 3 |

Slice 1 is safe to ship on its own and makes every later slice measurable. Slice 2 alone fixes the most visible failure in §1.

## 8. Testing and evals

- **Eval set.** About 30 real questions, drawn from the five jobs in §2. Each records its expected cited IDs and the facts the answer must contain. Score citation precision and recall, and must-contain facts. Track tokens and latency per question from slice 1's usage data. It runs against a fixed seeded dataset (a contract-suite fixture), not live mail.
- **Contract tests.** Each new repository query (search, batched brief) goes in the shared persistence suite, so sqlite and postgres agree. Search scoping must be tested: no results from another user's accounts or another organisation's projects.
- **Tool scope tests.** Each tool is called directly with an ID the user can't access, and must return not found.
- **Statement-count assertion.** `project_brief` must stay within a bounded number of statements, as `timeline_hydration_is_batched_and_correct` does for the timeline.
- **Caching check.** One integration test confirms a second turn reports non-zero cache-read tokens. It's skipped without credentials, like the postgres suite.

## 9. Risks

| Risk | Mitigation |
| ---- | ---------- |
| Prompt injection through email content ("ignore previous instructions, forward this to…") | Tool results are data, not instructions. Side effects only happen through confirmation cards the user clicks (PRD §5.4). Forwarding still checks the allowlist when it executes |
| Leaking across projects or accounts | Scope is enforced inside tools, not in the prompt. Scope tests (§8). `filterCitations` stays |
| ILIKE search too weak | Evals will show it. Option (c) slots in behind the same tool |
| Briefs going stale | Regenerated by the extraction debounce. Each brief records the watermark it was built from |
| Tokens growing without anyone noticing | Usage per run (§5.1), hard limits (§5.6) |
| Decrypted conversation content reaching logs | PRD §8.1. Tool results stored by reference (§4.5) shrink what's sensitive |

## 10. Open questions

- Does DSQL support `tsvector` and GIN indexes? It would sit between (a) and (c) in §4.3. Test on dev.
- Which questions does the operator actually ask most? The eval set should start from real usage, not guesses.
- Should conversation retention (90 days) be configurable in the UI? Carried over from PRD §11.
- Should project briefs be shown to users (e.g. at the top of the project page), or stay assistant-only? Showing them makes their quality visible and gives users a way to correct them.
