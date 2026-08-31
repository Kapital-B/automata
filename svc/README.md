# svc/ — Go API

HTTP service per [docs/specs/initial.md](../docs/specs/initial.md): **app auth** (email/password, Microsoft, Google), **mailbox** connect (Microsoft Graph), SQLite storage, JWT API access.

Hosting and the background-job redesign are specified in [docs/specs/aws-deployment.md](../docs/specs/aws-deployment.md).

## Requirements

- Go **1.22+**
- Docker / Docker Compose v2 (for the Floci local path; see below)
- Terraform **1.10.x** (Floci deploy)
- Microsoft Entra app: **mail** redirect `MS_REDIRECT_URI` (for Floci, `http://localhost:4566/restapis/<api-id>/local/_user_request_/api/accounts/callback`; for direct debug, `http://localhost:8080/api/accounts/callback`) with delegated `Mail.Read`, `Mail.Send`, `User.Read`, `offline_access`.
- Same app (or another) **sign-in**: add redirect `MS_AUTH_REDIRECT_URI` (for Floci, `http://localhost:4566/restapis/<api-id>/local/_user_request_/api/auth/microsoft/callback`; for direct debug, `http://localhost:8080/api/auth/microsoft/callback`) with delegated `openid`, `offline_access`, `email`, `profile` (v2 endpoint).
- **Google** (optional): OAuth Web client with redirect `GOOGLE_REDIRECT_URI` (default `APP_PUBLIC_URL/api/auth/google/callback`).

---

## Local development (AWS parity via Floci) — target path

Once [aws-deployment.md](../docs/specs/aws-deployment.md) Phases 1–4 land, day-to-day local runs use the **same Lambda / DynamoDB Streams / EventBridge shape as AWS**:

| Piece | Local |
| ----- | ----- |
| AWS APIs | [Floci](https://floci.io/) on `http://localhost:4566` (credentials `test` / `test`) |
| Product DB | Postgres in Compose (stand-in for Aurora DSQL) |
| Jobs | DynamoDB + Streams on Floci → worker Lambda |
| Scheduler | EventBridge on Floci → scheduler Lambda |
| HTTP | API Gateway on Floci → API Lambda |

Redis / Asynq are **not** part of this path.

### Prerequisites

- Docker Compose v2
- Go, Terraform 1.10.x
- Node 20+ (for `web/`)

### First-time setup

```bash
# from repo root
./scripts/local_configure
# edit svc/local.env — MS_*, ENCRYPTION_KEY, JWT_SECRET, and redirect URIs after first deploy
```

### Start emulators

```bash
./scripts/local_up
# Floci:   http://localhost:4566
# Postgres: localhost:5432  user/db/password automata
```

### Deploy API + jobs stack into Floci

```bash
./scripts/local_deploy
# prints api_gateway_url and invokes the migrate Lambda
```

`local_deploy` auto-detects `amd64` vs `arm64` from `uname -m`; override with `LAMBDA_ARCH`.
Re-run it after Lambda or Terraform changes.

### Logs

```bash
./scripts/local_logs
# stream worker and scheduler logs appear on the Floci container
```

### Web UI

```bash
cd web
# set VITE_API_BASE_URL to api_gateway_url from local_deploy
npm ci && npm run dev
```

**Debug shortcut only (not the default):** `JOBS_INLINE=true go run ./cmd/server` skips Floci and runs the job inline in the HTTP process.

The parity path uses four separate `provided.al2023` bootstrap archives:
- `cmd/api` for API Gateway REST proxy
- `cmd/scheduler` for EventBridge ticks
- `cmd/worker` for DynamoDB Streams
- `cmd/migrate` for explicit migration invokes

---

## Transitional path (Redis / Asynq)

Redis / Asynq are still in-tree for older flows and tests, but they are no longer the primary local path:

```bash
cd svc
cp .env.example .env
# edit .env — MS_*, ENCRYPTION_KEY (32 chars), JWT_SECRET (≥32 chars)

set -a && source .env && set +a
# terminal 1
go run ./cmd/server
# terminal 2
go run ./cmd/worker
```

If you still need the old Redis queue locally, start Redis separately (for example `docker run --rm -p 6379:6379 redis:7-alpine`):

```bash
docker run --rm -p 6379:6379 redis:7-alpine
```

- **Health:** `GET http://localhost:8080/api/health`
- **Start OAuth:** `POST /api/accounts` with body `{"provider":"m365","ms_account_kind":"work"}` or `"personal"`
- Open **`authorization_url`** in a browser: use the URL from **parsed** JSON (e.g. `jq -r .authorization_url`) so query parameters stay intact. Pasting from raw JSON that contains `\u0026` instead of `&` can break the request and trigger Azure errors such as **AADSTS900144** (missing `scope`).
- Microsoft redirects to `GET /api/accounts/callback?code=...&state=...`, then **302** to `{DASHBOARD_BASE_URL}{OAUTH_SUCCESS_PATH}?account_id=...`
- **Sync inbox:** `POST /api/accounts/{id}/sync` returns quickly with `job_run_id` and queues work
- **Categorize inbox:** `POST /api/accounts/{id}/categorize` (requires `LLM_BASE_URL` + `LLM_MODEL`) returns quickly with `job_run_id` and queues work
- **Track progress:** `GET /api/runs` / `GET /api/runs/{id}`; Floci/AWS use DynamoDB-backed runs with `X-Next-Cursor`, nullable `started_at` / `finished_at`, and `POST /api/runs/{id}/cancel`
- **List messages:** `GET /api/messages?account_id={uuid}`
- **List categories:** `GET /api/categories`

## Async run flow (transitional)

`cmd/server` can still enqueue sync/categorize tasks to Redis via Asynq when `JOBS_INLINE=false` and Redis is configured.

In the primary Floci/AWS flow, API requests enqueue into DynamoDB, the stream worker Lambda advances each job, and the scheduler Lambda handles due schedules / lease recovery.

UI and API consumers should treat the runs API as the durable source of truth for run state. On Floci/AWS the backing store is the DynamoDB jobs table ([aws-deployment.md](../docs/specs/aws-deployment.md)).

## Auth

| Method | Endpoint |
| ------ | -------- |
| Register | `POST /api/auth/register` `{"email","password"}` → `user_id`, `access_token`, `refresh_token` |
| Login | `POST /api/auth/login` `{"email","password"}` → `access_token`, `refresh_token` |
| Refresh | `POST /api/auth/refresh` `{"refresh_token":"..."}` → new `access_token` + `refresh_token` (rotation) |
| Microsoft login | `GET /api/auth/microsoft` → `authorization_url`; callback redirects to `DASHBOARD_BASE_URL` + `AUTH_SUCCESS_PATH#access_token=...&refresh_token=...` (URL **fragment**, not query—keeps tokens out of server logs/Referer) |
| Google login | `GET /api/auth/google` (if configured) → same fragment redirect for `/api/auth/google/callback` |
| Current user | `GET /api/me` with `Authorization: Bearer <access_token>` |

**Linking:** Microsoft/Google sign-in uses the IdP **email** claim. If a user already exists with that email (e.g. registered with password), the external identity is **attached** to the same `users` row (`user_identities`).

## Dev fallback user

If a request has **no** `Authorization: Bearer` header, middleware treats the caller as **`AUTH_DEFAULT_USER_ID`** (default matches the seeded `dev@localhost` user from migration `002_users_auth.sql`). Use a real Bearer token in staging/production.

## Mail (unchanged flow, now per logged-in user)

Send `Authorization: Bearer <jwt>` (or omit in dev for default user).

- `POST /api/accounts` — start mailbox OAuth (state ties connection to JWT user).
- `GET /api/accounts/callback` — Microsoft returns here; redirects to `OAUTH_SUCCESS_PATH?account_id=...`
- `GET /api/accounts`, `POST .../sync`, `GET /api/messages`, etc. — scoped to the authenticated user.

Open **`authorization_url`** from parsed JSON (`jq -r .authorization_url`) so `&` in query strings is not broken.

## Project correspondence (Wave 1)

| Area | Endpoints |
| ---- | --------- |
| People | `GET/POST /api/contacts`, `GET /api/contacts/{id}`, merge |
| Projects | `GET/POST /api/projects`, `GET/PATCH /api/projects/{id}`, member patch |
| Assignment | `POST /api/messages/{id}/project-assignment`, Unassigned `GET /api/unassigned` |
| Timeline / paste | `GET /api/projects/{id}/timeline`, `POST /api/manual-items` |
| Issues | `GET/POST /api/projects/{id}/issues`, `GET/PATCH /api/issues/{id}`, items attach/detach |
| Suggest | `POST /api/projects/{id}/issues/suggest` (requires `LLM_BASE_URL` + `LLM_MODEL`; confirm via create) |
| Facts (Wave 2a) | `GET/POST /api/projects/{id}/facts`, `GET /api/facts/{id}`, confirm/reject versions, evidence attach/detach |
| Current position | `GET /api/projects/{id}/current-position` (active facts + recent accepted decisions) |
| Interpret (Wave 2b) | `POST /api/projects/{id}/interpret`, `GET /api/projects/{id}/interpretations`, `POST /api/interpretations/{id}/dismiss` (LLM; provisional only — does not write facts) |
| Reconcile (Wave 2c) | `POST /api/projects/{id}/reconcile`, `GET /api/projects/{id}/contradictions`, `POST /api/contradictions/{id}/resolve` (applies fact candidates; opens contradictions when unsafe) |
| Decisions (Wave 2d) | `GET/POST /api/projects/{id}/decisions`, `POST /api/decisions/{id}/confirm`, `POST /api/decisions/{id}/withdraw`, `PATCH /api/decisions/{id}` |
| Needs My Input (Wave 2d) | `GET /api/attention` (project items + open mail action items), `GET /api/projects/{id}/attention` (derived `why_me` items + counts) |
| Ask Project AI (Wave 2e) | `POST /api/projects/{id}/ask` `{ "question" }` → answer + validated citations |
| Ask across projects (UI U4) | `POST /api/ask` `{ "question" }` → cross-project answer + project-scoped citations (capped to recent + open-attention projects) |

`GET /api/health` returns `{ "status": "ok", "llm": true|false }`.

## Layout

Hexagonal structure under `internal/`: `domain/`, `application/`, `adapters/` (see spec §2.4).

## Tests

```bash
go test ./...
```
