# RFC: A Grounded Assistant

**Status:** Slice 1 items 1–3 built (§11); eval set not started
**Related:** [AI-first assistant PRD](../prds/addendum-ai-first-assistant.md) (§8.1 conversation persistence still applies), [Aurora DSQL](addendum-aurora-dsql.md) (no extensions), [Triage efficiency](addendum-triage-efficiency.md) §17 (batched hydration)
**Last updated:** 2026-10-04 (decisions recorded in §6; slice 1 as built in §11)

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

Aurora DSQL supports no extensions ([addendum-aurora-dsql.md](addendum-aurora-dsql.md), "No extensions"), so `pgvector` and `pg_trgm` are out. Whether `tsvector` and GIN indexes work isn't documented there and needs testing. The options:

| Option | Quality | Cost to build and run | When |
| ------ | ------- | --------------------- | ---- |
| a. `ILIKE` over subject and preview, scoped by project, contact or date | Keyword only, no ranking | Nothing new | First |
| b. Rank in Go over the rows from (a) | Basic relevance ranking | Small | With (a) |
| c. A separate index (OpenSearch Serverless or S3 Vectors) fed by the sync job | Keyword plus semantic | A new service, an ingestion path and a deletion path | Only once evals show search quality is the bottleneck |

**Decided: stay on DSQL.** Build (a)+(b). Scoping by project and contact keeps the scanned set small, and one organisation's mail is modest. A separate service (c) is acceptable later, if evals show search quality is the bottleneck. It would sit behind the same `search_messages` tool, so nothing above it changes.

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

### 5.0 The target: $10 per user per month

The cheapest subscription should cost at most **$10 of LLM spend per user per month**. The usage data we collect will also feed into setting subscription prices. That has three consequences.

**The assistant is not the main cost.** The app calls the LLM from nine places, and seven of them run on incoming mail, not on questions:

| Call site | Runs when | Cost driven by |
| --------- | --------- | -------------- |
| `messages/categorize.go` | Mail syncs | Mail volume |
| `projects/llm_assign.go` | Mail syncs, rules can't place it | Mail volume |
| `messages/summarize.go` | Scheduled summaries | Mail volume |
| `messages/auto_draft.go` | Draft generation | Mail volume |
| `messages/forward_rules.go` | LLM-mode forward rules | Mail volume × LLM rules |
| `issues/suggest.go` | Issue suggestions | Project activity |
| `interpret/service.go` | Interpretation runs | Project activity |
| `messages/executors.go` | Merging partial summaries into one | Mail volume |
| `projectai/service.go` | Questions | **Questions asked** |

A user's mail volume fixes most of their cost before they ask a single question.

**Illustrative numbers.** These use first-party list prices for Claude Sonnet 5 ($2 / $10 per million input / output tokens) and assumed token counts. Bedrock prices differ, and slice 1 replaces all of this with measurements.

| Item | Assumption | Cost |
| ---- | ---------- | ---- |
| One message ingested (categorise + assign) | ~1.5k tokens in, ~100 out | ~$0.004 |
| A busy user's month of ingestion | 50 messages a day × 22 working days | ~$4.40 |
| One assistant answer, uncached | 3 model calls × (6.5k in + 400 out) | ~$0.05 |
| One assistant answer, cached | Same, with 5k of each call read from cache | ~$0.02 |

So for a busy user, ingestion could take about half the budget, leaving about $5.60: roughly 110–230 answers a month, or 5–10 per working day. For ingestion, the model choice and batching (below) matter more than anything done on the assistant.

**Two kinds of cost need two kinds of control.** Ingestion is fixed per user and grows with mail volume. It's controlled by model choice, skipping work rules can already do, and running non-urgent jobs (summaries, extraction) through Bedrock batch inference at a discount. The assistant is variable and grows with use. It's controlled by the per-user cap in §5.6. Both come out of the same $10.

**Pricing needs data per user and per feature.** That's more than run metadata gives us (`job_runs` is keyed by account, not user, and `meta_json` can't be aggregated). See §5.1.

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

Converse returns these counts (`TokenUsage`, including cache reads and writes), so the Bedrock adapter only needs to copy them across.

**Meter every call, not just the assistant.** Wrap the `LLMClient` in a metering decorator so all nine call sites (§5.0) are covered without editing each one. Callers put the user, account and feature on the `context.Context`. The decorator writes one row per call:

```
llm_usage(id, user_id, account_id NULL, feature, model,
          input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
          created_at)
```

`feature` uses the call site's name (`categorize`, `assign`, `summarize`, `ask`, …). Cost is **not** stored: it's computed at read time from a price table per model, so price changes and Bedrock-versus-list comparisons don't need a backfill. A call with no user on its context is recorded with a null user and counted, so gaps show up rather than disappearing.

This answers the pricing questions directly: cost per user per month, split by feature, and how that varies with mail volume. Nothing else in this section can be judged without these numbers.

### 5.2 Answer fixed questions without the LLM

"What needs me?", "my to-dos" and "what's open on DC07" are queries, not reasoning. Detect them cheaply and render the cards directly. At most, add a one-line summary. These are likely the most common questions on the home page.

### 5.3 Prompt caching

Order each request so the parts that don't change come first: tool definitions, then the system prompt, then the user's project list (and briefs), then the conversation. Mark a cache breakpoint at the end of that stable prefix.

- Cache reads cost about 0.1× base input. Writes cost 1.25× with a 5-minute TTL, or 2× with a 1-hour TTL. Two requests sharing a prefix within 5 minutes already pays for itself. A conversation with follow-ups is exactly that.
- **Any byte change in the prefix stops it being reused.** No timestamps, unsorted maps or per-request IDs before the breakpoint. Dates like "today is" go after it.
- The minimum cacheable prefix depends on the model: 512 tokens on the newest Opus, 1024 on Sonnet 5, **4096 on Haiku 4.5**. A short routing prompt on Haiku won't be cached at all.
- Confirm caching works by checking that cache-read tokens (§5.1) are non-zero on follow-ups.

Bedrock supports prompt caching, and the Converse API exposes it as cache-point blocks (`CachePointBlock` in the SDK version we use). Whether to stay on Converse or move to the Bedrock Messages endpoint is deferred (§6.3).

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
- **An assistant allowance per user**, read from `llm_usage`. It's whatever is left of the plan's monthly figure after projected ingestion, spread over the month so one heavy day can't use it all. Once it's spent, the assistant falls back to deterministic cards ("here's what matched; detailed answers resume tomorrow") instead of erroring. The allowance is a per-plan setting, so higher tiers can raise it without code changes.
- Ingestion isn't cut off when the budget runs out. Mail must still be filed. Users whose ingestion alone exceeds the plan are a pricing signal, not something the app should silently degrade for.

### 5.7 Pay at ingestion, not per question

Facts, decisions, issues and briefs are extracted once and read many times. Each improvement to extraction makes questions cheaper too. When an eval question fails because a fact was never extracted, fix the extraction, not the assistant's prompt.

## 6. Decisions

### 6.1 Search service: stay on DSQL (decided 2026-10-04)

A separate search service is acceptable in principle, but not now. Build §4.3 (a)+(b) on DSQL.

### 6.2 Budget: $10 per user per month on the cheapest plan (decided 2026-10-04)

See §5.0. The $10 covers **all** LLM spend: mail processing as well as the assistant (confirmed 2026-10-04). The metering in §5.1 is what subscription pricing will be based on.

### 6.3 Converse or the Bedrock Messages endpoint (deferred)

Decide with slice 1's numbers, before slice 3.

**For the Messages endpoint** (Anthropic's request format on Bedrock, through the official Anthropic Go SDK's Bedrock client):
- **The full Claude API.** It's the same request shape as Anthropic's own API, so caching TTLs and automatic caching, structured outputs, effort and thinking settings, and tool search are all available as documented. Converse is AWS's model-neutral layer, and Anthropic-specific features reach it later or not at all (tool search, for example, isn't available through Converse).
- **SDK support for the tool loop.** Typed tool definitions, a tool runner, and usage types, instead of hand-mapping Converse content blocks.
- **Portable.** The same code can call Anthropic's first-party API or Claude Platform on AWS by swapping the client. That lets us compare real costs per provider, which matters for pricing.

**For staying on Converse:**
- **Already wired and working.** No migration.
- **Model-neutral.** Non-Claude Bedrock models can be tried with the same code. Under a $10 budget, that's a real option for high-volume ingestion jobs such as categorisation, where a cheaper model may be good enough.

A split is possible too: Converse for ingestion, where model choice is about price, and the Messages endpoint for the assistant, where Claude-specific features matter most.

### 6.4 Where suggestions come from (proposed: the server)

Today `buildAssistantSuggestions` (`web/src/hooks/useAssistantHomeData.ts`) builds them in the browser from data the page has already fetched. That's fine for simple rules. It stops working once suggestions use the briefs and the LLM:

- **Budget.** An LLM-written briefing has to count against the user's allowance (§5.6). Only the server can enforce that. A browser can't be trusted to hold a budget.
- **Computed once, not on every page load.** The server can build the briefing when extraction runs (§4.4) and store it, so opening the home page costs no tokens. Built in the browser, every visit on every device would pay again.
- **One place for rules and permissions.** Attention, briefs and membership checks already live in Go. Duplicating them in TypeScript means two copies to keep consistent, and a permissions bug in the browser copy would be a leak.
- **Usable beyond the web app.** The same suggestions could go into an email digest or a notification later.
- **Measurable.** Recording which suggestions are shown and acted on tells us which features users value, which feeds pricing as well.

The cost is one new endpoint. The existing rules move to Go and stay deterministic.

## 7. Slices

| # | Slice | Changes behaviour? | Depends on |
| - | ----- | ------------------ | ---------- |
| 1 | **Foundations.** Usage in `LLMResponse`; metering decorator and `llm_usage` covering all nine call sites (§5.1); batch `buildContext`; eval set (§8) | No | none |
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
- **Metering.** A test with a fake LLM client checks that each call writes one `llm_usage` row with the user and feature from the context, and that a call without a user is recorded with a null user, not dropped.
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

## 11. As built: slice 1 (usage, metering, batching)

Built on 2026-10-04. The eval set (§8) is still to do. It needs real questions.

**Usage on the port.** `driven.LLMUsage` carries input, output, cache-read and cache-write tokens and the serving model. `InputTokens` means input billed at the full rate, so cached tokens are never counted twice:
- **OpenAI-compatible adapter.** It subtracts `prompt_tokens_details.cached_tokens` from `prompt_tokens`.
- **Bedrock adapter.** The SDK doesn't document whether `InputTokens` includes cached tokens, so `TotalTokens` decides. If it equals input plus output alone, the cached tokens are inside `InputTokens` and are subtracted. **To confirm:** check one real Converse response with caching on, to see which case Bedrock actually returns.

**Metering.** `llm.MeteredClient` wraps the shared client once per service in `composition/app.go`, so no call site changed. Feature names:

| Feature | Service |
| ------- | ------- |
| `categorize` | `CategorizeService` |
| `assign` | `AssignService` (LLM scoring tier) |
| `summarize` | `SummarizeService`, including the executor's merge of partial summaries |
| `auto_draft` | `AutoDraftService` |
| `forward_rules` | `ForwardRulesService` (LLM-mode rules) |
| `issue_suggest` | `issues.Service` |
| `interpret` | `interpret.Service` |
| `ask` | `projectai.Service` (`Ask` and `AskAcross`) |

User and account come from `driven.WithUsageScope`, set in two places:
- **The auth middleware** sets the user on every authenticated request.
- **The job runner (`runChunk`)** sets the job's user and account. `RunSynchronous` also goes through `runChunk`, so synchronous jobs are covered too.

Calls outside both scopes are still recorded, with a null user. A usage write that fails is logged and doesn't fail the call. The write ignores the caller's cancellation, because a call that completed has already been paid for.

**Storage.** `llm_usage` is created in `common/009`. Its `(user_id, created_at)` index is in `postgres/005` and `dsql/005` (`ASYNC`). The sqlite copy is `025_llm_usage.sql`. In sqlite, `created_at` is written in a fixed-width format, because RFC3339Nano text misorders times near a second boundary. `SumLLMUsageByUser` totals a user's usage per feature and model over a time range. There's no API or UI over it yet; for pricing analysis, query the table directly.

**Batched context build.**
- `AskAcross` filters membership in the query: `ProjectListFilter.MemberUserID`.
- `buildContext` loads active fact versions and evidence with `ListActiveFactVersionsForFacts` and `ListFactEvidenceForVersions`, in chunks of 200.
- Statements per question no longer grow with the number of facts: `Ask` 11 and `AskAcross` 10, with 2 facts or 12. Before, it was 13→33 and 14→34. `TestAskContextStatementsDoNotGrowWithFacts` holds this in place.
- Two behaviour changes. `AskAcross` now considers the user's first 200 member projects, not the organisation's first 200 filtered afterwards. Evidence lookup errors are now returned instead of silently dropped.

**Found along the way, not changed.** `ExecutionService.RunSynchronous` takes an executor argument but `runChunk` looks the executor up in the registry, so the argument only sets the job type. An unregistered executor passed directly fails as "unregistered job type".
