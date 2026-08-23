# PRD Addendum: Project-Oriented UI Shell

**Status:** Draft  
**Owner:** Product  
**Last updated:** 2026-08-22  
**Parent PRD:** [docs/prds/initial.md](initial.md)  
**Supersedes (chrome / IA only):** [AI-First Assistant Experience](addendum-ai-first-assistant.md) for **navigation and landing composition** — mail safety, provenance, and structured drill-downs remain in force  
**Depends on:** [Wave 1](addendum-project-correspondence.md), [Wave 2](addendum-project-correspondence-wave2.md)  
**Related:** [Wave 3](addendum-project-correspondence-wave3.md) (Path B / live connectors — **out of this UI slice**)

This addendum freezes a **full UI information-architecture redesign** for the pivot from mail-assistant-first to **project memory first**, while keeping a **user-focused Home** as the default landing page.

Backend contracts from Waves 1–2 are assumed. This document is authoritative for **routes, page jobs, and composition**. Visual polish can iterate; IA should not thrash.

---

## 1. Summary

Automata’s shell becomes:

| Mode | Job |
| ---- | --- |
| **Home** | What needs **me** across projects and channels — **list-first**, with **optional multi-project Ask** |
| **Project workspace** | What is true / what happened / what is open **here** |
| **Triage** | What still needs filing |
| **People** | Who the contacts are |
| **More** | Inbox, drafts, rules, connectors, runs, settings |

**Resolved product choices (2026-08-22):**

1. Home is **list-first + optional Ask** (not chat-first).  
2. **Inbox lives under More** — not primary nav.  
3. **Multi-project Ask on Home is in scope** for this redesign (product is pre-production; refine citations/grounding in place).  
4. **Path B (invites / members UI) is deferred** — do not block the shell redesign on Wave 3a–3b.

---

## 2. Problem

The current shell is a flat mailbox IA (Assistant, Today, Inbox, Projects, Unassigned, People, Drafts, Rules, …) with project features bolted onto Project Detail as rails. Operators must assemble “what is true on DC01” and “what needs me” across too many equal-weight destinations. That no longer matches Waves 1–2.

---

## 3. Goals

1. Make **Projects** the primary working mode; keep **Home** as the personal landing.  
2. Unify attention into one **Needs my input** list on Home (project attention + mail action items).  
3. Ship **multi-project Ask** on Home with mandatory citations and no silent commits.  
4. Collapse Project Detail rails into a clear **Trail | Position | Open** workspace.  
5. Demote channel tooling (Inbox, Drafts, Rules) without deleting it.  
6. Preserve parent safety: send/forward confirm, account provenance, no invented facts.

---

## 4. Non-goals

- Path B invite/members UI (deferred).  
- Live Slack/Teams connector chrome beyond existing placeholders (Wave 3).  
- Removing mail automation (drafts/rules/sync) — only demoting nav.  
- Replacing Wave 2 APIs; prefer adapting UI to existing endpoints, then extending Ask for multi-project.  
- Pixel-perfect visual redesign as a blocker — IA and page jobs first.

---

## 5. Navigation

### 5.1 Primary (always visible)

| Item | Route | Job |
| ---- | ----- | --- |
| Home | `/` | Needs me + recent projects + optional Ask |
| Projects | `/projects` | Index; badges for needs-me / triage |
| Triage | `/triage` | Filing queue (evolves `/unassigned`) |
| People | `/people` | Contacts directory |

### 5.2 More (secondary)

| Item | Route | Job |
| ---- | ----- | --- |
| Inbox | `/inbox` | Mail channel browser |
| Drafts | `/drafts` | Draft suggestions |
| Rules | `/rules` | Forward rules |
| Connectors | `/connectors` or `/accounts` | Mail (+ future) connections |
| Runs | `/runs` | Job history |
| Settings | `/settings` | Schedules / prefs |

**Redirects:** `/today` → `/` (or Home section anchors). `/unassigned` → `/triage`. `/` replaces Assistant as named landing (route may stay `/`).

---

## 6. Home (`/`)

### 6.1 Composition (first viewport)

One composition, brand + job:

1. Product identity (Automata)  
2. Headline oriented to the operator (e.g. needs-you framing — not a second product pitch)  
3. **Needs my input** list as the hero content (not metric cards)  
4. Optional: compact Ask affordance (input + submit) — secondary to the list  

No detached promo chips on the hero. No competing “Assistant vs Today” dual homes.

### 6.2 Needs my input (list-first)

Single ranked list. Each row:

- `why_me` (human label)  
- Title  
- Project code/name when project-scoped  
- Deep link into the right project mode (Position / Open / issue / contradiction) or mail action target  

Sources (merged client- or server-side):

- `GET /api/attention` (Wave 2)  
- Mail action items from summary (existing)  

Empty state: clear + CTAs to Projects / Triage.

### 6.3 Below the fold

- **Recent projects** — up to ~5 with a one-line current-position teaser when available  
- **Triage waiting** — count + link  
- Optional **channel pulse** — drafts ready / inbox new (links into More routes)

### 6.4 Multi-project Ask (in scope)

**Job:** Answer questions that span the operator’s projects using structured state + evidence; cite sources; never silent-commit facts/decisions.

**UX:**

- Input on Home (“Ask across projects…”)  
- Answer panel with citations (project code · type · id · deep link)  
- Insufficient context → say so  

**API (new or extended):** e.g. `POST /api/ask` with `{ "question" }` scoped to home-org projects the caller can access (Path A: all home-org projects they created/own; later Path B: memberships). Reuse Project AI grounding patterns; pack active facts, recent decisions, open contradictions, and light timeline snippets **per project**, with hard prompt isolation labels so the model cannot blend evidence across projects without citing which project.

**Invariants:**

- Citations must resolve to known ids in the packed context (same filter discipline as project Ask).  
- Do not mix two mail `account_id`s in one LLM mail snippet pack without labeling (parent provenance).  
- Ask may *propose* interpretation candidates only via existing provisional flows — not activate facts.

---

## 7. Projects index (`/projects`)

- List/grid of projects: code, name, needs-me count, last activity  
- Create project  
- Prefer “needs me” and recent over a flat dump  

---

## 8. Project workspace (`/projects/:id`)

Sticky chrome: code, name, role (edit elsewhere), **Current position strip**, primary actions (Paste, Ask this project).

### Modes

| Mode | Job | Content |
| ---- | --- | ------- |
| **Trail** | Chronology | Timeline (mail + paste + later connectors); row actions → issue / evidence |
| **Position** | Truth | Facts, decisions, contradictions, supersession history; confirm / resolve / reconcile entry |
| **Open** | Work | Issues; provisional facts; pending interpretations; project-scoped attention |

Default mode: **Trail** for history-heavy sessions; deep links from Home Needs me may open **Position** or **Open** directly (`?mode=`).

Issue detail remains a child route under the project.

**Ask (project-scoped):** slide-over or panel — existing `POST /api/projects/{id}/ask`.

---

## 9. Triage (`/triage`)

Evolve Unassigned:

- Unassigned mail + manual items  
- Later: provisional assignments needing confirm  
- Primary action: assign to project (and optionally kick interpret)

---

## 10. People / More

Unchanged jobs; Inbox demoted. Connectors page may rename Accounts over time without blocking.

---

## 11. Phased delivery (UI)

| Slice | Delivers | Exit criteria |
| ----- | -------- | ------------- |
| **U1** | Nav IA: Home / Projects / Triage / People + More; redirects from Today/Unassigned | Primary nav has four items; Inbox only under More — **done 2026-08-22** |
| **U2** | Home list-first Needs me + below-fold recent/triage | Single attention list is the first viewport focus — **done 2026-08-22** |
| **U3** | Project workspace Trail \| Position \| Open | Rails gone as permanent chrome; modes work — **done 2026-08-22** |
| **U4** | Multi-project Ask on Home + API | Cross-project question returns cited answer or insufficiency; no fact writes — **done 2026-08-22** |
| **U5** | Polish: deep links from Needs me → mode, empty states, triage rename | DC01 contradiction from Home opens Position/Open correctly — **done 2026-08-23** |

Path B UI is **not** a U-slice here.

---

## 12. Success criteria

- New operator understands Home vs Project without reading docs.  
- “What needs me?” is answerable on Home without visiting Inbox.  
- “What is duty on DC01?” works from project Ask **and** from Home multi-project Ask with citations.  
- Mail workflows still reachable under More.  
- No Path B UI shipped under this addendum.

---

## 13. Relationship to AI-first assistant addendum

That addendum made chat the **control surface**. This addendum keeps Home as the landing but makes **attention lists** the control surface and Ask **optional**. Structured pages remain; Projects become the main drill-down, not Inbox.

Where the two conflict on chrome: **this document wins**.

---

## 14. Open implementation notes

- Prefer extending attention merge on the server (`GET /api/attention` includes mail action items) so Home has one fetch — optional follow-up.  
- Multi-project Ask context budget: cap projects (e.g. recent + those with open attention) rather than dumping entire org.  
- Visual language: workbench clarity; avoid generic “AI purple” chrome; brand-first Home per product design rules.

---

*Implementation PRs should name the UI slice (U1–U5). Do not land Path B invites under this addendum.*
