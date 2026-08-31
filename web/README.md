# Automata web app

React + Vite + TypeScript UI for the Automata mail assistant. It talks to the Go `svc` API.

## Setup

```bash
cd web
npm install
cp .env.example .env   # optional
```

- **`VITE_API_BASE_URL`** — Base URL of the API (no trailing slash).

**Floci local (primary path):** start the backend with `./scripts/local_up` + `./scripts/local_deploy` from the repo root, then set `VITE_API_BASE_URL` to the printed `api_gateway_url` (Floci API Gateway), not bare `http://localhost:8080`. See [svc/README.md](../svc/README.md#local-development-aws-parity-via-floci--target-path) and [docs/specs/aws-deployment.md](../docs/specs/aws-deployment.md).

**Debug shortcut only:** `JOBS_INLINE=true go run ./cmd/server` still works for one-off local debugging, but the default path is API Gateway + Lambda + DynamoDB Streams on Floci.

## Run locally

```bash
npm run dev
```

Open the URL Vite prints (usually `http://localhost:5173`). Ensure the API is running and reachable at `VITE_API_BASE_URL`.

## Quality checks

```bash
npm run lint    # ESLint
npm test        # Vitest
npm run build   # production bundle
```

---

## Supported UI flows (high level)

| Area | What to try |
|------|-------------|
| **Auth** | Register / login, or Microsoft/Google OAuth per environment config. |
| **Accounts** | Connect mailbox (M365 work/personal), sync, delete account. |
| **Inbox** | List/filter by category and **project**, open message, HTML body refresh via sync, create draft, **manual forward**, **project assign** control. |
| **Projects** | Create with code + keywords; open timeline; paste correspondence; **Current position** strip (facts + decisions); **Ask Project AI**; **Interpretations** inbox; **Reconcile** + **Contradictions**; Facts / Decisions rails; issues; suggest / interpret when LLM is on. |
| **Home** | Needs my input (project attention + mail actions from `GET /api/attention`), recent projects, optional Ask. |
| **Unassigned** | Queue of mail/pastes without a committed project; assign to project. |
| **People** | Contacts from mail; merge suggestions. |
| **Rules** | Forward allowlist, create rules (**paused by default**), toggle enable, run rules now. |
| **Drafts** | List/edit/send/discard draft suggestions. |
| **Runs** | Background job history. |
| **Settings** | Summary + schedule configuration. |
| **Assistant** | Phase 1 home: action items, drafts, failed runs, and **unassigned counts** when present. |

---

## Manual verification checklist

Use this after backend (`svc`) and worker are up, with a real or test M365 account where possible.

1. **Auth** — Sign in; confirm `/api/me`-backed session and nav works.
2. **Connect** — Connect an account; confirm it appears with correct label/email and “connected”.
3. **Sync** — Run sync; confirm Inbox populates and runs list shows a sync job.
4. **Categorize** — Run categorize (new / re-categorize); confirm categories on messages and run completes.
5. **Summarize** — Refresh a mailbox summary from Runs or Inbox; confirm snapshot, action items, FYI update and run metadata. Home should show mail actions from `GET /api/attention`.
6. **Drafts** — From Inbox or Channel pulse, queue draft generation; open draft, edit, discard or send as appropriate.
7. **Forwarding (rules)** — Add allowlist addresses; create a rule (starts **paused**); enable intentionally; run rules; confirm behavior and **Runs** / job visibility.
8. **Forwarding (manual)** — From Inbox detail, **Forward** → pick allowlisted address → confirm; verify toast and that Graph forward succeeds for that message.
10. **Projects** — Create `DC01` with keywords; sync/assign mail; paste a Teams note; create issue Pump P-03; attach items; delete a source message and confirm the issue remains.
10b. **Facts** — Add `pump.p03.duty_kw` = 75 kW (confirm); propose 90 kW and confirm with supersede; Current position shows 90 kW; history shows 75 kW superseded; delete evidence message and confirm the fact remains.
10c. **Interpret** — With LLM on, paste/assign or click Interpret; pending candidates appear; dismiss without creating a fact.
10d. **Reconcile** — With pending interpretations, click Reconcile; compatible values reinforce; conflicting low-confidence claims open a contradiction; resolve with Keep proposed / Reject proposed.
10e. **Decisions / Needs my input** — Add a decision and confirm; Home shows a Needs my input row (and nav badge) for provisional facts/decisions, open contradictions, and mail actions.
10f. **Ask Project AI** — With LLM on and an active fact (e.g. duty 90 kW), ask “What is Pump P-03 duty?” and confirm the answer cites a fact version.
11. **Unassigned** — Confirm badge/count and assign remaining items.
12. **Suggest issue** — With LLM configured, propose from unassigned timeline items and confirm create; without LLM, the button stays disabled.

---

## Notes

- **Home mail actions** can be marked **Done** per item; there is no bulk “mark all reviewed” API yet.
- **Global search** was removed from the shell until there is a unified search API.
