# Automata web app

React + Vite + TypeScript UI for the Automata mail assistant. It talks to the Go `svc` API.

## Setup

```bash
cd web
npm install
cp .env.example .env   # optional; defaults to http://localhost:8080
```

- **`VITE_API_BASE_URL`** — Base URL of the API (no trailing slash). Example: `http://localhost:8080`.

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
| **Inbox** | List/filter by category, open message, HTML body refresh via sync, create draft, **manual forward** (allowlisted destination + optional comment). |
| **Today** | Summary snapshot, action items (mark done), FYI dismiss, refresh summary, draft shortcuts. |
| **Rules** | Forward allowlist, create rules (**paused by default**), toggle enable, run rules now. |
| **Drafts** | List/edit/send/discard draft suggestions. |
| **Runs** | Background job history. |
| **Settings** | Summary + schedule configuration. |
| **Assistant** | Phase 1 home (non-conversational): mirrors Today-style signals with account filter. |

---

## Manual verification checklist

Use this after backend (`svc`) and worker are up, with a real or test M365 account where possible.

1. **Auth** — Sign in; confirm `/api/me`-backed session and nav works.
2. **Connect** — Connect an account; confirm it appears with correct label/email and “connected”.
3. **Sync** — Run sync; confirm Inbox populates and runs list shows a sync job.
4. **Categorize** — Run categorize (new / re-categorize); confirm categories on messages and run completes.
5. **Summarize** — Refresh summary from Today; confirm snapshot, action items, FYI update and run metadata.
6. **Drafts** — From Today or Inbox, queue draft generation; open draft, edit, discard or send as appropriate.
7. **Forwarding (rules)** — Add allowlist addresses; create a rule (starts **paused**); enable intentionally; run rules; confirm behavior and **Runs** / job visibility.
8. **Forwarding (manual)** — From Inbox detail, **Forward** → pick allowlisted address → confirm; verify toast and that Graph forward succeeds for that message.
9. **Runs** — Confirm job rows for sync, categorize, summarize, draft, forward_rules transitions (pending → running → success/failed as applicable).

---

## Notes

- **Today → “Mark all reviewed”** is disabled with “coming later” until a bulk API exists; use per-item **Done** on action items.
- **Global search** was removed from the shell until there is a unified search API.
