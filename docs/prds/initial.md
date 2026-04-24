# PRD: Personal Email Automation and Daily Intelligence

**Status:** Draft  
**Owner:** Product (single-user / self-hosted context)  
**Last updated:** 2026-04-24

## 1. Summary

This document defines a personal automation product that helps manage email through a **React-based dashboard**, a **FastAPI** backend, and a **locally hosted LLM** (for example via LM Studio). The system summarizes incoming mail, surfaces action items and important but non-urgent information, categorizes messages, supports **rule-based forwarding** to allowlisted addresses, and **auto-drafts** replies in-app (with explicit send approval). The backend runs scheduled jobs (for example daily summarization) and on-demand tasks (for example “refresh summary now”).

A **core requirement** is support for **multiple connected email accounts**. Every piece of data—messages, summaries, categories, drafts, and audit logs—**must be attributable to a specific account** (full **provenance**). No user-facing or stored artifact should be ambiguous about its source account.

## 2. Problem

Managing several mailboxes and high volume is time-consuming. A local LLM can extract structure (action items, categories, reply intent) on demand, but only if email is integrated safely, on a **repeatable** schedule, and with **clear sourcing** when more than one account is connected.

## 3. Goals

1. **Unify** multiple mailboxes in one place with a single dashboard, without losing the identity of which account each item came from.
2. **Reduce** triage time via daily and on-demand **summaries** with **action items** and **FYI highlights**.
3. **Classify** messages into user-meaningful categories (for example important, spam, finance, and others as defined in configuration).
4. **Automate** safe, **allowlisted** forwarding based on rules (for example “messages that look like invoices” to a specific address).
5. **Support** in-app **drafts** of replies for messages that need a response, with a path to enrich prompts from **other data sources** later; **sending** (when implemented) is explicit and **never** implicit from a draft alone.
6. **Schedule** background work (for example daily sync and summary) while still allowing **manual refresh**.

## 4. Non-Goals (Initial Release)

- Storing or editing drafts in the mailbox provider’s “Drafts” folder (drafts are **application-local**; see [§5.2](#52-multi-account-identity--provenance)).
- Replacing the full mail client (threading, calendar, contacts as first-class products).
- Team collaboration, shared mailboxes, or role-based access within the product (single principal operator; multi-tenant org product is out of scope unless later specified).
- Hosting the LLM inside this service (the LLM is an **external** OpenAI-compatible endpoint, for example LM Studio on localhost).

## 5. Key Design Principles

### 5.1 Provenance and multi-account

- The system **must** allow **connecting more than one** email account.
- For **every** user-visible object and for **internal processing records**, the design **must** record a stable **Account** (or `account_id`) reference:
  - Raw and normalized **messages** and attachments metadata.
  - **Summaries** (daily, windows, and “refresh” snapshots) and all **LLM outputs** (when stored).
  - **Labels/categories** applied to a message.
  - **Forward** actions and their outcomes.
  - **In-app drafts** and send attempts.
  - **Scheduled jobs** and on-demand **runs** (which account or accounts they targeted).
- **Provenance** means: given any row or API response, a client or operator can answer: **which account** this came from, and **which sync or run** produced it, where applicable.
- **Cross-account views** (for example “today’s summary across all accounts”) are allowed only if each contributing segment is still labeled with **account** (and, where relevant, message IDs scoped to that account’s provider state).

### 5.2 Multi-account identity and “workspace”

- Introduce a first-class **Account** entity: human-readable name, connection state, and provider-specific identity. **Microsoft mail** (both **work or school** and **personal Outlook** / Outlook.com, Hotmail, Live) is accessed via the **Microsoft Graph** with delegated permissions as in the technical spec, including **read and send** on day one for each connected account; in-app **drafts** remain in the app’s store only. Work and personal mailboxes are **separate** connections (separate `account_id`s) and may coexist.
- **User** (single operator) owns many **Accounts**. APIs and the UI should never merge streams without an explicit **account** dimension in the data model and in the interface (badges, filters, or clear sections per account, plus optional “all accounts” with per-line provenance).

### 5.3 Local LLM and structured output

- LLM calls **should** use **constrained, JSON-oriented** prompts and **schemas** (or equivalent) so outputs are **machine-parseable**; invalid JSON should be retried or repaired with a small number of controlled steps, since local models behave best when forced into **JSON** rather than free-form prose for extraction tasks.

## 6. User-Facing Capabilities

### 6.1 React dashboard

- A **web UI** to view summaries, browse categorized mail (with filters), manage rules and allowlists, and review in-app **drafts** and send actions.
- The UI must make **account** obvious on every list and detail view (provenance as a UX requirement).
- **Connect accounts (required flow):** The product **must** include a first-class **flow to add, confirm, and disconnect** one or more mailboxes. Users start from a dedicated **Settings / Accounts** (or equivalent) area: choose **account type** for Microsoft mail (**work or school** vs **personal** Outlook) so sign-in is routed to the right identity endpoint, then complete **Microsoft sign-in and consent** in the browser, return to the app, and see the new account with **name, primary email, and connection status** (connected, error, needs sign-in). The same area lists **all connected accounts** and supports **disconnect** and **reconnect** when tokens expire. Empty states must prompt the user to connect at least one account before the rest of the dashboard is useful. The technical spec details the **step-by-step UI** and the **backend OAuth** contract: [UI flow (§10.1)](../specs/initial.md#101-account-connection-flow-ui), [OAuth / callback (§6.1)](../specs/initial.md#61-oauth-and-account-connection-backend).

### 6.2 Daily and on-demand summaries

- **Daily summary** of incoming email **per account** and an optional **rolled-up** view across accounts where each line or section remains tagged with **account**.
- **Action items** that are for the user (obligations, questions directed at the user, deadlines when inferable).
- **FYI / awareness** content: important to know, no immediate action required.
- A control to **refresh** the summary on demand; refresh must not erase provenance and should record a new **summary run** identifier for audit and debugging.

### 6.3 Categorization

- A configurable set of **categories** (for example important, spam, finance, and others). Assignments are stored **per message** and **per account** (message IDs are not globally unique across accounts).

### 6.4 Forwarding rules (allowlist only)

- Users define **rules** (conditions + action: forward to an address) where the destination is restricted to a **user-maintained allowlist** of email addresses to prevent mistaken forwards.
- **Each rule** is scoped: either global with allowlist check, or **per account** (recommended) so the same condition can do different things per mailbox.
- Execution must be **auditable** (which account, which message, which rule, outcome, timestamp).

### 6.5 Auto-drafted replies and sending

- For messages that **require a response**, the system may generate **draft text** stored **in the application only**.
- **Sending** (or forwarding via send) is a **separate, explicit** action; day-one product intent includes **read and send** for connected **Microsoft** mail (work and personal) at the **provider integration** layer, but **sending a draft** remains user-confirmed in the product flow.
- The architecture should reserve space for **future context** (other APIs, files, or notes) to be injected into draft prompts **per account** or **per user** without conflating data from two accounts.

### 6.6 Backend and operations

- **FastAPI** serves APIs and (if used) a thin auth/session layer for the dashboard.
- **Scheduled tasks** run periodic sync, summarization, and reporting; **on-demand** jobs map to the same internal pipeline with explicit run metadata (**account** + job type + time range).

## 7. Security and Trust (Product-Level)

- **Credentials and tokens** are stored per **Account**, encrypted at rest where the stack supports it; no credential sharing between accounts in logic or logs.
- **Provenance in logs:** operational logs for sync, LLM calls, and sends include **account_id** and non-sensitive correlation IDs.
- **Allowlisted forwarding** and any send path are high-risk: surface **clear UI** and **opt-in** rule enablement; consider dry-run or digest modes as later enhancements.

## 8. Success Metrics (Personal / Qualitative to Start)

- Time to “know what needs doing today” decreases versus checking each account manually.
- No confusion about **which account** a summary line or draft refers to in practice (qualitative, but treat ambiguous UI as a defect).
- Forward rules run only to **allowlisted** addresses with visible audit history.

## 9. Dependencies and Assumptions

- **Microsoft mail** (work or school, and **personal** Outlook) connects via **Microsoft Graph** (delegated OAuth) with at least **read and send** for accounts where the user has completed consent. **Work** tenants may require **admin consent** or block apps per policy; **personal** Microsoft accounts (MSA) use consumer consent and do not involve a corporate tenant. Exact scopes, authority (`organizations` vs `consumers`), and Entra **supported account types** for the app registration are in the technical spec; in-app **drafts** do **not** require mailbox draft-folder write access.
- **Local LLM** exposes an **OpenAI-compatible** HTTP API; availability of the model is an environmental assumption (automation may **skip or retry** LLM steps when the endpoint is down, without mis-attributing data across accounts).

## 10. Out of Scope for This PRD (Handled Elsewhere)

- **Technical spec:** API shape, database schema (including `accounts`, `message_provenance` fields, job runs), exact JSON schemas for LLM calls, and Graph sync strategy (for example delta queries).
- **Runbooks:** Azure/Entra app registration **including personal Microsoft accounts** (see spec), **admin consent** and org review where applicable (work), and consumer (personal) app consent experience.

## 11. Open Questions

- **Google Workspace (resolved direction, later phase):** A **subsequent** implementation phase will add **Google work mail** (Gmail for your organization) as another **provider** alongside Microsoft; the same **per-account** provenance and dashboard model apply. Details are in the [technical spec — §13.1](../specs/initial.md#131-planned-phase-google-workspace). Open items for that work: **Google Cloud** project + OAuth consent screen, which **Gmail API** scopes, and (for some orgs) **domain-wide delegation** vs per-user OAuth—see the spec.
- Whether **per-account** LLM prompt overrides are needed (tone, industry jargon).
- Retention: how long to keep **raw** vs **summary** data per **account** for privacy and storage.

## 12. Phased delivery

Work is **split into ordered phases** so each stage ships something testable. **Provenance and multi-account** are built into the data model from the first mail-related phase, not added later. The **canonical** breakdown (goals, exit criteria, optional reordering of forward rules vs in-app send) is in the **technical spec**: [docs/specs/initial.md — §12 Implementation phases](../specs/initial.md#12-implementation-phases).

**At a glance:**

| Order | Product slice |
| ----- | --------------- |
| 1 | API + database foundation |
| 2 | Connect **Microsoft** mail (work and/or **personal** Outlook), sync into a **per-account** message store |
| 3 | Incremental sync, scheduled and manual jobs, run history |
| 4 | LLM **categorization** (JSON) |
| 5 | **Summaries** (action items + FYI) and on-demand **refresh** |
| 6 | **Forward** rules with **allowlist** and audit |
| 7 | In-app **drafts** and **explicit send** |
| 8 | **Hardening** and full **dashboard** UX (account switcher, polish, tests) |
| (later) | **Google Workspace** (work) mailbox via Gmail API: connect + sync + send on par with Microsoft accounts ([§13.1 in spec](../specs/initial.md#131-planned-phase-google-workspace)) |

*Initial release focuses on **Microsoft** mail; Google **work** mail is a planned follow-on phase, not a v1 launch blocker.*

---

*This PRD is the product definition input for the implementation spec. Engineering should treat **multi-account** support and **provenance** as non-negotiable invariants, not as add-on fields.*
