# Addendum: Redis, Asynq, and the job runner

**Status:** Draft (implementation guidance)  
**Parent document:** [Technical specification (initial.md)](initial.md)  
**Companion PRD:** [docs/prds/initial.md](../prds/initial.md)  
**Last updated:** 2026-04-25

This addendum details how to run **asynchronous background jobs** (sync, categorization, summarization, forward rules, draft suggestion, and future pipeline steps) using **Redis** and **[Asynq](https://github.com/hibiken/asynq)** in the **`svc/`** Go backend. It aligns with the **hexagonal layout**, **`job_runs`** contract, **HTTP APIs** for runs, **scheduler** expectations, and **logging / tracing** rules in [initial.md](initial.md). It does **not** change product intent; the PRD remains authoritative for product behavior.

---

## 1. Goals and non-goals

**Goals**

- **Durable job state** in the existing relational store via **`job_runs`** ([§4.7](initial.md#47-job_runs)): the dashboard and operators use the same rows for polling (`GET /api/runs`, `GET /api/runs/{id}`) and audit.
- **Asynq** on **Redis** for dispatch, retries (optional), and **per-queue concurrency**.
- A **global** cap on concurrent jobs across all queues, in addition to per-queue limits—important when **Graph**, **LLM**, and **SQLite** share one machine locally.
- **Structured logs** and **OpenTelemetry** traces with stable correlation: `run_id`, `account_id` (when applicable), `job_type`, and trace identifiers.
- **Real-time updates** for **`web/`** (optional for v1): complement polling with push after durable writes—see [§6](#6-real-time-updates-durable-inbox--push).
- **On-demand** and **scheduled** jobs invoke the **same application use cases** as HTTP handlers ([§1](initial.md#1-system-context), [§2.4](initial.md#24-backend-project-structure-ddd-hexagonal-ports-and-adapters)).

**Non-goals**

- **No dead-letter queue (DLQ)** as a separate product surface: terminal failure is represented by **`job_runs.status = failed`** and **`error_message`** (and optional detail in `meta_json`). Asynq may still retain failed task records for ops; that is an implementation detail, not a user-facing DLQ contract.
- This addendum does **not** mandate a separate “notifications” table: **`job_runs`** is the **durable inbox** for run history unless product later adds user-facing notification rows.

---

## 2. Repository and process layout

| Component | Role |
| --------- | ---- |
| **`svc/cmd/server`** | HTTP API; holds **Asynq client** only to **enqueue** tasks when an endpoint should return immediately with a `job_run_id`. |
| **`svc/cmd/worker`** | **Asynq server** (consumer): registers **mux** handlers per task type; runs scheduled enqueues or consumes from queues; **no** requirement to serve HTTP unless you colocate health on the same process for local dev. |
| **`svc/internal/adapters/inbound/asynq/`** (suggested name) | Thin **driving adapters**: decode payload, attach logging/tracing context, acquire global concurrency (see [§5](#5-concurrency)), call **`application/...`** services, update **`job_runs`**, publish real-time events (see [§6](#6-real-time-updates-durable-inbox--push)). |
| **Scheduler entrypoint** | Either **inside `cmd/worker`** (cron + client enqueueing to self) or a dedicated **`svc/cmd/scheduler`** that only enqueues—choose based on operational preference for local runs. |

**Dependency rule:** Asynq handlers depend on **`internal/application`** and **`internal/application/ports/driven`** only through types constructed in the **composition root** (same pattern as [`cmd/server/main.go`](../../svc/cmd/server/main.go)); **domain** must not import Asynq or Redis client packages.

---

## 3. Mapping spec concepts to Asynq

### 3.1 Queues and task types

Map **one Asynq queue per `job_runs.job_type`** so metrics, concurrency, and logs stay aligned with [§4.7](initial.md#47-job_runs):

| `job_type` (enum) | Asynq queue name | Notes |
| ----------------- | ----------------- | ----- |
| `sync` | `sync` | Graph-heavy; honor 429 / `Retry-After` in the use case ([§3.4](initial.md#34-rate-limits-and-backoff)). |
| `categorize` | `categorize` | LLM-bound; strict JSON contract per PRD/spec. |
| `summarize` | `summarize` | LLM-bound; may include `time_window_*` on the run row. |
| `forward_rules` | `forward_rules` | Send path; allowlist enforced in application code ([§8](initial.md#8-security)). |
| `draft_suggest` | `draft_suggest` | LLM-bound; drafts remain app-local per PRD. |

**Task payload (JSON, versioned):** Every task should carry at minimum:

- `schema_version` (integer) for forward-compatible decoding.
- `run_id` (UUID) — **must** match the **`job_runs.id`** row created before or when enqueueing.
- `user_id` (UUID) — for authorization and repository scoping (single-operator v1 still benefits from consistent keys).
- `account_id` (UUID, nullable only when the spec explicitly allows a global job; default is **per-account** runs for traceability).
- `trigger_kind`: `schedule` \| `api` — maps to **`job_runs.trigger`**.
- Optional: `time_window_start`, `time_window_end` for summarize; **`traceparent`** (W3C) for on-demand jobs started from HTTP so worker spans can **link** to the API trace.

**Task name / kind:** Use a small constant per handler (e.g. `TypeSyncV1`) distinct from the queue name if you prefer; the queue name remains the spec’s `job_type` for ops clarity.

### 3.2 `job_runs` lifecycle

Statuses are defined in [§4.7](initial.md#47-job_runs): `pending` \| `running` \| `success` \| `failed` \| `cancelled`.

Recommended transitions:

1. **API or scheduler** creates **`job_runs`** row: `pending` (or `running` if you only insert at dequeue time—prefer **`pending` at enqueue** so `GET /api/runs/{id}` works immediately after `202`/`200` with id).
2. **Worker** starts handler: update to **`running`**, set **`started_at`** if not already set.
3. **Success:** **`success`**, **`finished_at`**, populate **`meta_json`** (counts, model name, etc.).
4. **Failure:** **`failed`**, **`finished_at`**, **`error_message`** (sanitized, no secrets; no raw bodies per [§8](initial.md#8-security)).

**Provenance:** Downstream rows (`message_categories.run_id`, summary snapshots, etc.) should reference this same **`run_id`** where the spec requires automation traceability ([§2.1](initial.md#21-provenance-non-negotiable)).

**Idempotency:** Enqueue and handler design should tolerate **at-least-once** delivery: use cases should be safe to retry or should detect duplicate execution via `run_id` + state in DB where needed.

---

## 4. HTTP API behavior (alignment)

Existing and planned routes from [§6](initial.md#6-http-api-go-service-in-svc):

- **`POST /api/accounts/{id}/sync`** — Should return **`job_run_id`** once sync is **asynchronous**; implementation creates `job_runs` then enqueues Asynq task on queue `sync` with payload above.
- **`POST /api/accounts/{id}/summaries/refresh`** — Same pattern for queue `summarize`.
- **`GET /api/runs`** / **`GET /api/runs/{id}`** — Remain the **durable** status contract for **`web/`** ([§10](initial.md#10-react-dashboard-contract)).

Optional spec item: **`WS /api/runs/{id}/stream`** — If implemented, see [§6](#6-real-time-updates-durable-inbox--push).

---

## 5. Concurrency

### 5.1 Per-queue limits

Configure the Asynq **Server** with a **per-queue concurrency** map (environment-driven), for example:

- `sync` — moderate; bounded by Graph and local DB.
- `categorize`, `summarize`, `draft_suggest` — **lower**, to protect **LM Studio** / local LLM from OOM and overload ([§7](initial.md#7-scheduler-and-job-runner)).

Exact numbers are **deployment tuning**, not normative in this addendum.

### 5.2 Global limit across queues

Asynq’s per-queue settings **do not** enforce a **cluster-wide** or **process-wide** ceiling: two queues could each run at full capacity simultaneously.

**Required pattern (single `cmd/worker` process, local):** A **weighted semaphore** (or equivalent) with capacity **`GLOBAL_MAX_CONCURRENT_JOBS`**, acquired at the start of every handler’s “real work” section and released in **`defer`**. Per-queue concurrency then caps **admission** to each handler type; the semaphore caps **total** concurrent executions.

**Future multi-worker:** Move the global limiter to **Redis** (atomic counter + lease/TTL, or Lua script) so all worker processes share the same cap—same interface in application code behind a small port if needed.

### 5.3 At most one sync per `account_id`

[§7](initial.md#7-scheduler-and-job-runner) requires **at most one sync per `account_id` at a time**.

Implementation options (choose one):

- **Asynq unique tasks:** Unique key derived from `account_id` for queue `sync`, with **TTL** covering the longest expected sync; second enqueue fails or is deduplicated—surface as **409 Conflict** or return existing **`run_id`** from API policy decision.
- **Redis SETNX / lock** at enqueue time in the API adapter, with short TTL and clear error if lock held.

Combine with **`job_runs`** query if you need to distinguish “already running” vs “failed last time” for UX copy.

### 5.4 LLM batching

Spec: batch LLM work with concurrency limits for LM Studio. Implement **inside** categorize/summarize/draft use cases (batch size + semaphore), **in addition to** queue-level and global limits—queue limits alone do not cap in-handler parallelism if one task fans out to many goroutines.

---

## 6. Real-time updates (durable inbox + push)

**Durable inbox:** **`job_runs`** is the source of truth. The UI should **`GET /api/runs/{id}`** on load and after reconnect ([§6](initial.md#6-http-api-go-service-in-svc), [§10](initial.md#10-react-dashboard-contract)).

**Push (optional v1):** After a **successful commit** of `job_runs` status (or in the same transaction as the update when using a store that supports it), **publish** a small event:

- **Channel key:** e.g. `runs:{run_id}` or `user:{user_id}:runs` for fan-out.
- **Transport:** **Redis Pub/Sub** is natural because Redis is already required for Asynq; **`cmd/server`** subscribes and forwards over **`WS /api/runs/{id}/stream`** or **SSE** as specified in [§6](initial.md#6-http-api-go-service-in-svc).
- **Payload:** `run_id`, `status`, optional short `meta` snippet—**no secrets**, no full mail bodies (PII policy [§8](initial.md#8-security)).

**Ordering:** Push is **best-effort**; clients must **reconcile** with `GET /api/runs/{id}` if they missed an event.

---

## 7. Failure handling (no DLQ)

- **Normal errors:** Handler returns `error` → Asynq applies **retry policy** if configured; on **final** failure, set **`job_runs.status = failed`** and **`error_message`** once.
- **Panic:** Shared middleware **`defer` + `recover`**: log structured fields + stack (truncated), set **`failed`**, release global semaphore, then either return error to Asynq or re-panic per team policy—**never** leave `running` indefinitely without a watchdog.
- **Retries:** If product wants **“crash once = failed row”** with no Asynq retries, set **`MaxRetry = 0`** and map every returned error to terminal **`failed`** on the run row.

There is **no** separate user-facing DLQ table; operators use **`GET /api/runs`**, logs, and Asynq inspection tools as needed.

---

## 8. Logging and tracing

### 8.1 Structured logging

Every **job-related** log line must include at minimum ([§7](initial.md#7-scheduler-and-job-runner)):

- `run_id`
- `account_id` (when the job is account-scoped; omit or null only for explicitly global jobs)
- `job_type`

Additionally:

- `trigger_kind` (`schedule` \| `api`)
- `trace_id` / `span_id` when OpenTelemetry is enabled

Use **`log/slog`** with JSON handler in local dev for grep-friendly output; align attribute names with OTel where practical.

### 8.2 OpenTelemetry

- **One consumer span per task execution** (name: e.g. `job.sync`), attributes: `run_id`, `job_type`, `account_id`, `trigger_kind`.
- **On-demand jobs:** If the HTTP handler creates a span, propagate **`traceparent`** in the task payload and **link** or continue the trace in the worker so cross-service diagrams show API → queue → worker.
- **Scheduled jobs:** Root span per execution is sufficient.

Spec [§13](initial.md#13-future-extensions-non-blocking) already calls out OTel with `account_id` on spans; this addendum makes **`run_id`** and **`job_type`** mandatory for job spans.

### 8.3 PII

Follow [§8](initial.md#8-security): no raw email bodies in logs; prefer internal ids and short non-sensitive metadata.

---

## 9. Scheduler integration

Nightly pipeline per [§7](initial.md#7-scheduler-and-job-runner): **`sync` → `categorize` → `summarize` → `forward_rules`** (when enabled), **per `account_id`**.

**Enqueue strategy:**

- **Separate tasks per step:** Each step gets its own **`job_runs`** row (clear audit, simple `GET /api/runs` rows), or
- **Single orchestrator task** that enqueues the next step on success and records linkage in `meta_json` (fewer rows, harder to read from runs list alone).

Either is valid; document the chosen UX in implementation notes. **Trigger** for all scheduled rows: **`trigger_kind = schedule`**.

**Cron:** `robfig/cron` or ticker inside **`cmd/worker`** (or scheduler binary), reading **`SCHEDULER_UTC_CRON`** or per-job crons from [§9](initial.md#9-configuration-environment).

---

## 10. Configuration (environment)

Add to or extend the deployment surface from [§9](initial.md#9-configuration-environment):

| Variable | Purpose |
| -------- | ------- |
| `REDIS_ADDR` / `REDIS_URL` | Redis for Asynq (and optional Pub/Sub for run events). |
| `ASYNQ_PREFIX` | Optional key prefix when sharing Redis with other local apps. |
| `JOB_QUEUE_SYNC_CONCURRENCY` | Asynq concurrency for queue `sync`. |
| `JOB_QUEUE_CATEGORIZE_CONCURRENCY` | For queue `categorize`. |
| `JOB_QUEUE_SUMMARIZE_CONCURRENCY` | For queue `summarize`. |
| `JOB_QUEUE_FORWARD_RULES_CONCURRENCY` | For queue `forward_rules`. |
| `JOB_QUEUE_DRAFT_SUGGEST_CONCURRENCY` | For queue `draft_suggest`. |
| `GLOBAL_MAX_CONCURRENT_JOBS` | Cross-queue cap (semaphore) in each worker process. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | (When tracing enabled) export endpoint for local collector / Jaeger. |

Exact names may be adjusted to match **`internal/configuration`** style in **`svc/`**; the table captures **intent**.

---

## 11. Migration from synchronous Phase 1 sync

Phase 1 may run **sync inline** in the HTTP handler for debugging ([§12.1](initial.md#121-phase-1--microsoft-mail-read--message-store)). When moving to Asynq:

1. **`POST .../sync`** creates **`job_runs`** (`pending`), enqueues task, returns **`job_run_id`** immediately.
2. **Worker** calls the existing **`SyncService`** (or extracted use case) with **`run_id`** already known so the service **updates** the row instead of generating a new UUID at the end—avoid duplicate or orphan rows.
3. Keep **Graph** and **token refresh** logic inside **application** + **outbound adapters** unchanged except for **context timeouts** appropriate for longer-running work.

---

## 12. Testing

- **Unit:** Payload decode, status transitions, semaphore acquire/release ordering (no double-release on panic path).
- **Integration:** Redis test container or embedded Redis if available; enqueue from test API, assert **`job_runs`** terminal state and **`GET /api/runs/{id}`** JSON.
- **Concurrency:** Assert second sync for same `account_id` is rejected or deduped per [§5.3](#53-at-most-one-sync-per-account_id).

---

## 13. References

- [Technical specification — initial.md](initial.md) (system context, `job_runs`, API, scheduler, security, phases).
- [PRD — initial.md](../prds/initial.md) (provenance, scheduled vs on-demand runs, dashboard contract).
- [Asynq documentation](https://github.com/hibiken/asynq/wiki) (Server, Client, Scheduler, uniqueness, retries).

---

*End of addendum. Implementation PRs should reference this document alongside [initial.md](initial.md) for queue and observability review.*
