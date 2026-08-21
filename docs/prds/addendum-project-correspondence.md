# PRD Addendum: Project Correspondence Intelligence

**Status:** Draft  
**Owner:** Product (single-user / self-hosted context)  
**Last updated:** 2026-08-21  
**Parent PRD:** [docs/prds/initial.md](initial.md)  
**Related addenda:** [AI-first assistant](addendum-ai-first-assistant.md)  
**Source vision:** [AI Project Correspondence Agent](AI%20Project%20Correspondence%20Agent.pdf) (August 2026)

This addendum freezes the first move from Automata’s mailbox-first product toward that vision. It does not replace the parent PRD. Mail accounts, provenance, explicit send approval, allowlisted forwarding, and auditable job runs remain non-negotiable.

A companion technical spec is [docs/specs/addendum-project-correspondence-wave1.md](../specs/addendum-project-correspondence-wave1.md). The PRD remains authoritative for product intent; the spec is authoritative for schema, APIs, and invariants.

---

## 1. Summary

Automata today answers: **“What needs my attention across my connected mailboxes?”**

This addendum adds a second question, without dropping the first: **“What is the current position of this project, and how did we get there?”**

The organising unit for that second question is the **Project**. Communication channels remain data sources. The first-class objects introduced here are **Project**, **Contact**, and **Issue**. **Facts**, **decisions**, supersession, contradiction detection, and two-stage state reconciliation are named so they are not invented early — they are **out of scope for this addendum**.

Wave 1 **ships** as a personal, single-operator product (Path A): one logged-in user, no invites, no shared UI. The **domain must not collapse Contact and Profile into one record.** A contact is someone in *your* address book, including people on *your* projects. A profile is an independent Automata account. Those stay separate until someone **explicitly** invites or links them. Signing up must not join a person to a project they only appear on as someone else’s contact.

## 2. Relationship to existing product

| Surface | Role after this addendum |
| ------- | ------------------------ |
| Assistant home | Still the default attention surface: what I should know, decide, or do next. |
| Today | Still the structured daily review of action items and FYIs. |
| Inbox | Still the mail list, filterable by project. |
| **Unassigned** (new) | Dedicated queue of correspondence with no committed project. |
| Drafts, Rules, Runs, Accounts | Unchanged. |
| **Projects** (new) | Chronological correspondence, issue trails, assignment. |
| **People** (new) | Your contact address book (not other Automata profiles). |

The assistant must not pretend that a project timeline exists until Projects ship. Connector chips for Slack, Linear, and MCP remain “coming soon” until those providers ingest into the same correspondence model.

Parent PRD non-goals still apply: Automata is not a replacement mail client, not a CRM, and not a generic task manager.

## 3. Strategic freeze: ship Path A, keep Contact and Profile distinct

**What Wave 1 ships:** one operator, their mailboxes, their project UI. No organisation switcher, invitations, or permission matrix.

**What Wave 1 must not close:** inviting a Profile onto a project later; linking *your* Contact to *their* Profile; assigning work to a contact who still has no login.

**What Wave 1 must not do:** treat a Contact as a not-yet-activated User. If Sarah is a contact on a project you own, and she independently creates an Automata account, she is **not** on that project. She remains your contact. She has her own profile and her own projects. You connect later — typically by inviting her onto the project — and only then may your contact be linked to her profile.

### 3.1 Two person models

| | **Contact** | **Profile** (User) |
| --- | --- | --- |
| What it is | A person in **your** address book | An Automata **account** |
| Who owns it | Your organisation (your People list) | The person who signed up |
| How it appears | Sender/participant on correspondence; listed on your project | Login, mailboxes, their own projects |
| Access to your project | None | Only after an explicit invite (or equivalent) |
| Wave 1 | Infer from mail, merge aliases | The logged-in operator (already exists) |

A **Contact ↔ Profile link** is a separate, optional, explicit relationship. It is not implied by matching email, by appearing on a timeline, or by creating an account.

Typical later sequence:

1. You own DC01. Sarah appears as a contact (from email). She is a participant on the project, not a member with access.
2. Sarah signs up for Automata on her own. She gets a profile and an empty workspace. She cannot see DC01. Your People list is unchanged.
3. You invite Sarah onto DC01. She accepts. Her **profile** becomes a project **member** (access).
4. After accept, you **confirm a link** from your contact “Sarah” to that profile (suggested by matching email). The link is not created by accept itself.

Invite grants access. Link is a separate confirmation after accept. They are two facts: **access** (profile on project) vs **identity join** (your contact = that profile).

If two of your contacts would both match that profile, you **merge those contacts first**, then link the surviving contact. A profile links to at most one contact per organisation.

### 3.2 Required shape (even with one user)

These are implementation constraints for this addendum, not Wave 1 features.

1. **Organisation at signup.** Registering a profile creates that profile’s default organisation in the same transaction. It owns **that operator’s** contacts, projects, and issues. It is an address-book and project container, not a shared directory of every human with a matching email.
2. **Contact and Profile are different aggregates.** Do not put `user_id` on Contact as “this is actually a user.” Use an explicit link table (or equivalent) that is created only by invite/link flows. Signup must not write that link and must not grant project membership.
3. **Two ways to be “on” a project.**
   - **Participant (Contact):** appears on correspondence; no product access. Wave 1.
   - **Member (Profile):** can open the project. Wave 1 has exactly one: the creating user. Extra members come from invite, not from contact inference.
4. **Role fields live on the right relation.** The operator’s discipline/scope on DC01 is on **their member row** (a Profile membership). A contact’s “consultant, mechanical” notes can live on a **project-participant** or contact-on-project row later — not by turning the contact into a member.
5. **Work can be assigned to a Contact without them having a Profile.** After invite, work can be assigned to a **member Profile**. Linking contact to profile is how your address book and that member line up. `awaiting_me` is derived when the assignee is the current user’s profile, not a stored status.
6. **Identity uniqueness is per address book.** Contact emails are unique within **your** organisation’s contacts. The same email may exist as Sarah’s **profile** login and as your **contact** identity without being the same row. Auto-merge across those graphs is forbidden.

### 3.3 Still out of Wave 1 product scope

Do not build: invites, org admin, project ACL UI, shared inboxes, contact↔profile linking UI, or “Sarah can log in and see DC01.” Do not auto-join or auto-link on signup. Do not require a contact to have a profile in order to appear on a timeline or be named as an assignee.

## 4. Problem

Mailbox intelligence is necessary and insufficient for project work.

A technical question may arrive by email, be discussed on Teams, confirmed on WhatsApp, and closed in a revised drawing. Automata can summarise the email. It cannot yet:

- treat the **project** as the primary object;
- recognise that the same **person** appeared on three channels;
- group those items into one **issue trail**;
- keep a durable **current position** as information changes.

The Correspondence Agent PRD’s core technical problem is: cross-channel project classification + identity resolution + issue linking + temporal reasoning + user-responsibility reasoning + evidence-grounded reconciliation.

This addendum takes only the first three: **classification (project)**, **identity (contact)**, **issue linking**. Temporal reconciliation and role-aware attention wait for Wave 2.

## 5. Goals

1. Introduce **Project**, **Contact**, and **Issue** as first-class product objects, in a new bounded context beside accounts and messages — not as mail categories or action-item labels.
2. Assign every correspondence item to a project, provisionally to a project, or to **Unassigned**, with confidence and a reason the user can correct.
3. Resolve people across identities (email first; phone and chat ids as aliases) into a canonical **Contact** in the operator’s address book. Low-confidence merges require confirmation. Contacts must remain linkable **later** to a **Profile**, only through an explicit invite or link — never by signup or email match alone.
4. Group related correspondence into an **Issue** with a user-correctable status, a trail of evidence, and an optional assignee (**Contact** for someone in the address book, **Profile** for “me” / a member). Wave 1 may default assignee to the operator’s profile.
5. Make the **project correspondence timeline** the primary project interface. AI labels supplement the timeline; they do not replace it.
6. Accept **manual correspondence** (pasted WhatsApp, Teams notes, call notes) into the same timeline so the model is not Graph-shaped before live connectors exist.
7. Preserve existing mail automation: sync, categories, summaries, drafts, forwarding, and the assistant.

## 6. Non-goals (this addendum)

- **Facts**, **decisions**, superseded-information registers, contradiction detection, and two-stage interpret-then-reconcile. Those are Wave 2.
- **Needs My Input** as a role-aware engine. Wave 1 may show existing action items on a project; it must not claim discipline-aware routing.
- **Ask Project AI** over structured project state. Assistant Phase 2+ remains mail-intelligence Q&A until Wave 2.
- Live Slack, Teams, WhatsApp, SMS, call, or meeting ingest. Manual paste is the non-email path.
- **Shipping** shared projects: invitations, organisation admin, project ACL, contact↔profile linking UI, or a second login seeing the same DC01. Modelling **separate** Contact and Profile aggregates, project **members** (profiles) vs **participants** (contacts), and an optional explicit link so those can land is **in** scope for the spec (see [§3](#3-strategic-freeze-ship-path-a-keep-contact-and-profile-distinct)).
- Auto-joining a new Automata account to a project because that email already exists as a contact.
- Replacing the assistant home with a correspondence-only UI.
- Implementing project as a `category_definitions` slug.
- A generic task list, RFI module, document-authoring tool, or ERP.
- Autonomous send or any relaxation of explicit confirmation.

## 7. Design principles

### 7.1 Two organising units, both visible

- **Account** remains the provenance unit for anything that came from a connected mailbox (`account_id` on the source message, and on derived mail intelligence).
- **Project** is the organising unit for correspondence intelligence.
- Cross-project views are allowed only if each item still shows project (or Unassigned) **and**, for mail-derived items, account provenance.

### 7.2 Categories are not projects

Categories (`important`, `spam`, `finance`, …) are a user-controlled vocabulary on a message. Project assignment is a different axis: “this communication is about DC01.” A message may have a category and a project, one of them, or neither (Unassigned).

### 7.3 Evidence is immutable

Original correspondence is never overwritten by AI or by user correction.

- User corrections change **assignment**, **contact merges**, **issue links**, and **interpretation labels**.
- They do not edit stored body text, timestamps, provider ids, or attachments.
- Duplicate provider events must not create duplicate correspondence records. External source ids remain unique per account (mail) or per manual-ingest identity.

### 7.4 Identity layers

| Layer | Answers | Wave 1 |
| ----- | ------- | ------ |
| Channel identity | Which mailbox, email address, or pasted sender string was this? | Required. Mail uses existing `account_id` + `from_json`. |
| Contact (your address book) | Which person in *my* People list is this? | Required. `Contact` + `contact_identities`, unique within *my* organisation. |
| Profile (their account) | Do they have an Automata login? | The operator already has a profile (`users`). Other people’s profiles are out of Wave 1 UI. |
| Project participant | Are they on this project’s correspondence? | Contact linked as participant via assignment / timeline. No access. |
| Project member | Can they open this project in Automata? | Member row on a **Profile**. Wave 1: creating user only. |
| Contact ↔ Profile link | Have we explicitly said this contact is that account? | Not created in Wave 1. Must be modellable without merging the rows. |

Connected **mail accounts** are the operator’s mailboxes. **Contacts** are people in the operator’s address book (clients, consultants, colleagues). **Profile** is the logged-in user (and, later, other accounts). The operator does not need a self-contact; they act as a profile. Other people do not need a profile to be contacts.

### 7.5 Mail is live; other channels are pasted

Microsoft (and later Google) mail remains the live ingest path. WhatsApp, Teams, SMS, and call notes enter Wave 1 only as **manual correspondence items** with `source = manual` (or equivalent), a timestamp the user supplies, and optional original-text paste. Adding a live provider later must map into the same correspondence model without changing Project / Contact / Issue.

### 7.6 Assistant and project surfaces coexist

- Assistant / Today: **attention** (“what needs me”).
- Project: **position** (“what is true on DC01, and the trail”).
- Wave 1 does not make the assistant the project timeline, and does not make the timeline the daily inbox.

### 7.7 Contacts and profiles stay separate until we connect them

Wave 1 will only have one profile (the operator). Implementation must still assume:

- a **Contact** is your record of a person, even if they later have an Automata account;
- a **Profile** is their account, with its own organisation, mailboxes, and projects;
- appearing as a contact on a project you own does **not** grant access when they sign up;
- **invite** grants a profile membership on the project;
- **link** is a **confirmation after accept**, not a side effect of accept or of matching email.

If a design stores other people only as `users` with `invite_pending`, or folds Profile into `contacts.user_id` that gets filled on signup, it is the wrong design: it either forces everyone through registration or auto-joins them to your project.

## 8. Domain model (product-level)

New aggregates live in a **project-intelligence** bounded context (`organisations`, `contacts`, `projects`, `project_members`, `project_participants` or equivalent, `issues` in domain/application packages). **Profile** is the existing `users` (and later session/auth) model, not a contact subtype. Mail sync, categorization, summaries, drafts, and rules stay in the existing messages context. The join is correspondence assignment, contact participants, and (later) profile membership.

Wave 1 creates one **organisation** for the operator **at signup** (not an org-admin UI; not deferred until first project). That organisation owns **this profile’s** contacts and projects. Another person who signs up gets **their own** organisation. Matching emails across those graphs do not merge them.

### 8.1 Correspondence item

A correspondence item is one communication in a project timeline.

Wave 1 sources:

- A synced **message** (existing `messages` row). The correspondence item is a view or thin wrapper over that row, not a second copy of the body.
- A **manual** item: user-pasted text, chosen source label (WhatsApp, Teams, SMS, call, meeting, note), timestamp, and optional participants.

Every item has:

- timestamp;
- source (outlook, gmail, manual/whatsapp, …);
- participants (contacts, once resolved);
- subject or title where relevant;
- original text / evidence;
- link back to the mail message when the source is a mailbox;
- `account_id` when mail-derived;
- project assignment (including Unassigned);
- optional issue link(s).

### 8.2 Contact

A **Contact** is a person in **this organisation’s address book** — your People list. It is not an Automata account.

- Display name, optional company.
- One or more **identities**: `email`, `phone`, later `slack`, `teams`, `display_name_hint`.
- Identity values are unique per **owning organisation** for a given kind (the same email cannot belong to two of *your* contacts unless you have merged them). The same email **may** exist on someone else’s **profile** without being this contact.
- **No `user_id` on the contact as identity.** An optional **ContactProfileLink** is how your contact is joined to an account. Wave 1 does not write this row. Signup must not write it. Invite-accept must not write it. A later confirmation step writes it, and only for **one** contact per profile per organisation. If two contacts would link to the same profile, **merge the contacts first**.
- The system may **suggest** that two of *your* identities are the same contact. Low confidence requires confirmation. High confidence may auto-merge only within your address book (exact email). Never auto-link to a Profile.
- Wave 1 bootstrap: upsert identities from message `from_json` and, where stored, recipients. Do not require an LLM.
- Do not create a self-contact for the operator unless a later spec needs it for “me as a participant.” The operator is a Profile.

### 8.3 Profile

A **Profile** is an Automata user account (existing `users`): login, connected mailboxes, their organisation, their projects.

- Independent of anyone else’s contacts.
- Creating a profile never copies, claims, or joins another organisation’s contacts or projects, even when emails match.
- Wave 1 has one profile in play: the operator. Other profiles exist in the world the moment someone else registers; they are not members of this operator’s projects.

### 8.4 Project

A **Project** belongs to an organisation (the creator’s). It is the primary intelligence object.

Minimum fields: name, **required structured code** (unique per organisation), optional description, optional keywords / aliases (for assignment), optional client.

Project codes are structured, not free text. Wave 1 format: 2–8 characters, `A–Z` then `A–Z` or `0–9`, stored uppercase (examples: `DC01`, `HVAC2`, `PLANT`). The code is a high-confidence assignment signal (whole-token match in subject/body). Display name remains free text (“Cooling Upgrade”). Drawing numbers such as `M-402` are not project codes.

Two relations to people — do not collapse them:

**Members (Profiles) — access**

- Who can open the project in Automata.
- Stores the member’s role, discipline, responsibilities, current work / scope, approval authority, out-of-scope notes.
- Wave 1: one row, the creating **profile**. `created_by` is audit; the member row is who has access.
- Further members are added only by invite (out of Wave 1 UI).

**Participants (Contacts) — correspondence people**

- Who appears on the project trail (and can be named as an external assignee).
- Implied by assigned correspondence in Wave 1; a dedicated participant row is allowed if it is cheaper than inferring every time.
- Participants have **no** access.

Do not model Sarah-the-contact as a member. Do not model the operator as a contact in order to store their role.

### 8.5 Correspondence-to-project assignment

Incoming correspondence is evaluated against known projects.

**Thread is the unit; a message may override.** Assigning a mail message to a project assigns the Graph `conversation_id` thread on that account, unless the operator marks a single message as an override. New messages in the thread inherit the thread assignment. Messages with no `conversation_id`, and all manual items, are always per-item. Reassigning the thread does not move messages that have an override. The Unassigned page lists items whose **effective** assignment is none.

Signals (Wave 1, cheapest first):

- project code or name in subject or body;
- keywords;
- participants / email domains already associated with the project;
- thread / conversation history already assigned;
- user override.

Outcomes:

| Confidence | Behaviour |
| ---------- | --------- |
| High | Assign automatically. User can reassign. |
| Medium | Assign **provisionally**. Visible as needs confirmation. |
| Low / none | **Unassigned correspondence**. |

Every assignment stores: project (or unassigned), confidence, reason, source (`rule`, `llm`, `user`), and timestamps. Reassignment is a new interpretation, not an edit of the original item.

### 8.6 Issue

An **Issue** groups correspondence that concerns the same underlying question, problem, or design thread (example: Pump P-03 sizing).

- Belongs to one project.
- Has a title and a short current-position note (user- or AI-proposed; user-correctable).
- Optional **assignee**, which is *either* a **Contact** (someone in your address book, no access implied) *or* a **Profile** (a project member). Wave 1: default assignee is the operator’s profile (“me”); the picker may also name a contact. Do not store `awaiting_me` as a status — derive it when the assignee is the current profile.
- Status vocabulary for Wave 1: `open`, `awaiting_input`, `resolved`. Additional PRD states (`proposed_resolution`, `reopened`, …) may be added later without changing the object.
- Status may be AI-proposed and must be user-correctable.
- Trail membership is a set of correspondence items plus optional explicit notes, ordered by correspondence timestamp.
- An item may belong to more than one issue only if the user (or a later spec) needs it; default is **one primary issue**. Engineering should not invent multi-issue membership in Wave 1 unless the spec says so.

Issues are not a generic task app. They are delegable: Wave 1 delegates to a contact (they will not see it in Automata until they are invited). After invite, you can assign to their **profile**; linking your contact to that profile is how your People list shows they are the same person. Existing `action_items` remain mail-summary obligations. Wave 1 may *show* open action items whose source message sits on the project; it must not migrate or replace them. If action items later grow an assignee, use the same Contact-vs-Profile rule, not a boolean “for me.”

### 8.7 Facts (deferred, named only)

A **Fact** would be a current valid project assertion with history (Pump P-03 = 90 kW, previously 75 kW, superseded on 14 August, evidence linked).

Wave 1 must not add a facts table, a “current position” generated from the latest message, or silent overwrite of earlier conclusions. The issue’s current-position note is a label, not an authoritative fact register.

## 9. User-facing capabilities (Wave 1)

### 9.1 People

People is the operator’s **contact** address book, not a directory of Automata profiles.

- List contacts inferred from mail, plus any created by hand. Do not list other Automata accounts.
- Open a contact: identities, recent correspondence, suggested merges (within this address book).
- Confirm or reject a suggested merge.
- Manually add an identity (email, phone).
- Do not show “linked profile” as a live control in Wave 1. The model must allow that field later; the UI must not imply that signing up will attach them here.

### 9.2 Projects

- Create, rename, and archive a project (structured **code** required).
- Set keywords and **member** fields for the operator’s profile (role / scope). Those fields live on the member row, not on a self-contact and not only as columns on the project.
- Open a project into the timeline-first interface.

### 9.3 Assignment and Unassigned

- **Unassigned** is a dedicated page (`/unassigned`), not only an Inbox filter: the queue of correspondence with no committed project (plus provisional items that need confirmation).
- Inbox may still filter by project.
- Assign or reassign one **thread** (default for mail) or one **message** (override).
- Confirm provisional assignments.
- See why the system chose a project (reason + confidence).

### 9.4 Project timeline

Opening a project shows:

- header: name, code, current user’s membership role;
- correspondence timeline as the dominant surface (source, time, sender contact, subject, snippet);
- filters: all, mail, manual, unassigned-to-issue, my attention (existing action items on this project’s mail — not a new engine);
- an issues list / rail;
- empty state that explains paste + assign if the project has no items.

Each timeline row links to original evidence (Inbox message, or the pasted text). Account badges remain on mail-derived rows.

### 9.5 Issues

- Create an issue on a project (manual or from a suggested title).
- Attach / detach correspondence.
- Change status.
- Set or change **assignee** (Wave 1: operator profile by default; picker among project contacts).
- Open the trail as a chronological subset of the project timeline.

### 9.6 Manual ingest

- From a project (or Unassigned): paste text, pick source type, set time, optional participants.
- The item is real correspondence: it can be assigned, linked to an issue, and shown on the timeline.
- Manual items have no `account_id`. Provenance is `source = manual` plus the creating user for audit. The item still belongs to the **organisation / project**, not to a private per-user silo.

## 10. UX posture

Recommended navigation additions: **Projects**, **People** (contacts), **Unassigned**. Do not bury Projects under Settings. The logged-in **profile** remains Accounts / Settings, not the People list.

Do not remove Assistant, Today, or Inbox.

Suggested project layout (desktop): header and a compact issue/attention strip above a **full-width timeline**. A secondary column for issues and assignment prompts is allowed; the chronological record must remain visually dominant.

The assistant home may later gain a “3 unassigned items” or “2 provisional project assignments” chip. That is optional in Wave 1 and must use real counts, never placeholders.

## 11. AI in Wave 1

Wave 1 AI is optional and thin. Correctness of the objects matters more than automation.

Allowed:

- Deterministic assignment from code / name / keyword / known participants.
- A constrained JSON proposal: `project_id` or unassigned, `confidence`, `reason`, optional `issue_title`, optional contact matches already in the database.
- User confirmation for medium confidence and for new issues.

Not allowed in Wave 1:

- Writing authoritative project state from a single message.
- Inferring facts, decisions, or supersession.
- Marking issues resolved because a later message “sounds done” without user confirmation.
- Mixing two accounts’ mail in one LLM call (parent PRD provenance still applies).

LLM outputs that affect assignment or issues are interpretations: they must record `source`, `confidence`, and `run_id` where a job produced them, matching existing artifact discipline.

## 12. Phased delivery (this addendum)

Each slice should be shippable and testable. Do not start Issues before Contacts and Projects exist. Do not start Facts in any of these slices.

| Slice | Name | Delivers | Exit criteria |
| ----- | ---- | -------- | ------------- |
| **1a** | Contacts | Organisation created at signup; Contact + identities unique **within that org**; no `user_id` on contact; bootstrap from mail From/To; People UI; suggested merge with confirm | After sync, senders appear as contacts; one email cannot sit on two contacts in *this* org; a profile with the same email elsewhere is a different row; a merge is user-confirmed |
| **1b** | Projects + assignment | Project CRUD with structured code; **member** row for the creating profile; thread assignment + per-message override; **Unassigned page**; keyword/code auto-assign | Operator can create `DC01`, assign a thread by hand, override one message, work the Unassigned queue; contacts on the trail are not members; reassignment does not mutate the message |
| **1c** | Timeline + manual ingest | `/projects/:id` timeline; paste correspondence; account badges on mail rows | DC01 timeline shows Outlook mail plus one pasted Teams/WhatsApp item in time order |
| **1d** | Issues | Issue CRUD; link items; assignee is profile **or** contact; derived awaiting-me; trail view; optional LLM issue-title suggestion | Pump P-03 exists; several items sit on it; assignee can be the operator profile or a contact; evidence still opens the source |

**Wave 1 is done** when the core acceptance story below works without live WhatsApp or Teams.

**Wave 2** (separate addendum): two-stage reconciliation, facts, decision register, role-aware Needs My Input, auto-resolution of attention, Ask Project AI over structured state.

**Wave 3** (separate addendum): live Slack / Teams / WhatsApp / SMS / transcripts / document revisions, mapping into the same correspondence model. **Enabling** Path B (invite a profile onto a project, then optionally link your contact to that profile) is a product slice in this wave or later. The **shape** required for Path B is already a Wave 1 spec constraint ([§3](#3-strategic-freeze-ship-path-a-keep-contact-and-profile-distinct)).

## 13. Invariants

1. Mail-derived rows still carry `account_id`. Cross-account APIs never drop it.
2. Correspondence evidence is append-only. Corrections attach new interpretation rows (assignment, issue link, contact merge).
3. `(account_id, provider_message_id)` uniqueness is unchanged. Manual items have their own stable id; they are not fake Graph messages.
4. Project assignment is not a category. The category tables stay as they are.
5. Unassigned is a **dedicated page** (and a first-class queue), not a null the UI hides.
6. Wave 1 **authorisation** is still the logged-in profile (no ACL product). Contacts and projects belong to **that profile’s organisation**, created at signup. Another profile’s signup is a different organisation.
7. Parent safety rules still apply: no send without explicit confirmation; no forward off allowlist; no secrets in logs.
8. **Contact ≠ Profile.** Signup, email match, and appearing on a project as a contact must not grant membership or write a contact↔profile link. Invite-accept grants membership to a profile. Link is a **confirmation after accept**. Two contacts cannot link to the same profile; **merge first**.
9. Specs must keep **project members** (profiles, access) separate from **project participants** (contacts, correspondence).
10. **Contacts and issues outlive mail.** They are project memory: no time-based purge in Wave 1, and deleting or later-expiring a message must not cascade-delete the contact or the issue.

## 14. Core acceptance scenario (Wave 1)

**Project:** DC01 Cooling Upgrade  
**Operator role (stored, not yet used for routing):** Mechanical Engineer  

1. Outlook mail: “Please use the 75 kW pump shown on Rev B.” → assigned to DC01 (hand or high-confidence code match). Contact created for the sender.
2. Pasted Teams note: “We should consider increasing this to 90 kW.” → same project, later timestamp.
3. Pasted WhatsApp: “90 kW is approved. Please update the drawing.”
4. Outlook mail: “Please find M-402 Rev C showing the 90 kW pump.”
5. Operator creates or confirms issue **Pump P-03 Sizing** and attaches these items.
6. Timeline shows all four in order, with source labels and evidence links. Issue trail shows the same set.

Wave 1 **does not** require the system to treat 90 kW as current fact or 75 kW as superseded. That is the Wave 2 test (Correspondence Agent PRD §39, events 2–4). Wave 1 only requires that the trail is one object the operator can open.

## 15. Success criteria

Wave 1 succeeds when:

- Communications from mail and paste appear in one project timeline.
- Every item retains original source and evidence.
- People in the timeline are **contacts** in the operator’s address book, not raw email strings and not Automata profiles.
- Assignment to project (including Unassigned) is visible, explainable, and user-correctable.
- Related items can be grouped into an issue trail; assignee can be the operator’s profile or a contact.
- The schema can add a second **profile member** via invite, and a **contact↔profile link**, without migrating contacts into users or auto-joining on signup.
- Assistant, Today, Inbox, drafts, and rules still work with account provenance intact.
- No fact/decision register has been smuggled in as a side table on issues.

## 16. Resolved decisions

These were open in an earlier draft and are now frozen.

| Topic | Decision |
| ----- | -------- |
| Mail thread vs message | A mail **thread** (`conversation_id` on that account) is the assignment unit. A **per-message override** is allowed. Manual items are always per-item. |
| Organisation creation | The default organisation is created **at profile signup**, in the same transaction as the user row. |
| Contact ↔ profile link | **Confirmation after invite accept.** Accept grants membership only. Link is a second, explicit confirm (email match may *suggest*). |
| Two contacts, one profile | **Merge is required.** A profile links to at most one contact in an organisation. Do not dual-link. |
| Project codes | **Structured and required.** Wave 1: `^[A-Z][A-Z0-9]{1,7}$`, unique per organisation, stored uppercase (e.g. `DC01`). |
| Unassigned UX | **Dedicated page**, plus optional Inbox filters. |
| Retention | **Contacts and issues last longer than mail.** No time-based purge of contacts/issues in Wave 1; message deletion must not destroy them. |

## 17. What this addendum does not change

- Parent PRD phases for mail, summaries, forwarding, and drafts.
- AI-first assistant phases 1–5 (attention, chat over mail intelligence, triage, drafts, rules).
- Microsoft / Google account connection and Graph/Gmail provenance.
- Local/OpenAI-compatible LLM hosting assumptions.

It **does** change the product’s destination: Automata remains the personal operator console in Wave 1, and becomes the correspondence substrate for project memory that can later be shared. Wave 1 is the substrate and the first project objects. It is not yet the Correspondence Agent’s state engine, and it is not yet a multi-user product — but it must not have to be remodelled to become one.

---

*Implementation PRs should reference this addendum and [the Wave 1 spec](../specs/addendum-project-correspondence-wave1.md). That spec must not introduce facts, live non-email connectors, ACL **enforcement**, invites UI, or auto-link-on-signup. It **must** keep Contact and Profile as separate aggregates, organisation-at-signup, structured project codes, thread assignment with per-message override, a dedicated Unassigned page, project **members** vs **participants**, issue assignee as profile or contact, longer retention for contacts/issues, and a place for an explicit contact↔profile link that Wave 1 does not write.*
