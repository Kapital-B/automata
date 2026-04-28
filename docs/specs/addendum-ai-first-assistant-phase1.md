# Technical Specification Addendum: AI-First Assistant Phase 1

**Status:** Draft  
**Parent PRD:** [AI-first assistant addendum](../prds/addendum-ai-first-assistant.md)  
**Related PRD:** [Initial PRD](../prds/initial.md)  
**Related spec:** [Initial technical specification](initial.md)  
**Last updated:** 2026-04-28  

## 1. Scope

This document specifies **Phase 1 - MVP: State-Aware Assistant Home** from the AI-first assistant PRD addendum. Phase 1 replaces the current mock assistant landing page with a real, state-aware home backed by existing `svc/` APIs and existing `web/` data models.

Phase 1 is intentionally not a full conversational agent. It does not introduce encrypted conversation persistence, free-form LLM chat, tool execution, rule creation from chat, or send-from-chat. Those belong to later assistant phases.

The goal is to make `/` useful immediately:

- show the user's current attention state;
- make open action items visible without opening Today;
- show real account and provenance information;
- provide state-aware suggestion cards that route to existing product surfaces;
- preserve the Today page as the structured review view.

## 2. Current State

Relevant existing frontend files:

- `web/src/pages/Index.tsx` renders `ChatPage` inside `AppShell`.
- `web/src/pages/Chat.tsx` is currently mock-backed:
  - imports mock accounts from `web/src/lib/mock-data`;
  - uses static suggestions;
  - returns mock assistant replies from `mockReply`;
  - shows "Replies are mocked while we wire up the backend."
- `web/src/pages/Today.tsx` already fetches `GET /api/summaries`, displays action items and FYIs, and provides refresh/done/dismiss actions.
- `web/src/lib/auth.ts` already exposes the API client functions needed for Phase 1.
- `web/src/hooks/useAccountsData.ts` already maps `GET /api/accounts` into UI accounts.
- `web/src/components/AccountBadge.tsx` already provides provenance chips.
- `web/src/components/AppSidebar.tsx` already has an Assistant nav item and Today/Drafts/Runs/Accounts navigation.

Relevant existing backend routes:

| Route | Existing client function | Phase 1 use |
| ----- | ------------------------ | ----------- |
| `GET /api/accounts` | `listAccounts` | connected account count, status, account labels |
| `GET /api/summaries?account_id=...` | `getSummary` | action item count, FYI count, latest summary timestamp, source item cards |
| `GET /api/drafts?account_id=...` | `listDraftSuggestions` | draft count if cheap enough to fetch |
| `GET /api/runs?account_id=...&limit=...` | `listRuns` | recent run status, failed/running indicators |
| `POST /api/accounts/{id}/summaries/refresh` | `refreshSummary` | optional Phase 1 refresh CTA |
| `POST /api/action-items/{id}/done` | `markActionItemDone` | optional Phase 1 action card control |
| `POST /api/fyi/{id}/dismiss` | `dismissFYI` | optional Phase 1 FYI card control |

No new backend routes are required for Phase 1 if the UI composes these existing endpoints. A backend aggregation endpoint may be added later for performance, but Phase 1 should start with frontend composition.

## 3. Non-Goals for Phase 1

- Persisting assistant conversations.
- Encrypting assistant conversation bodies.
- Sending natural-language prompts to the LLM.
- Returning synthesized free-form assistant answers from the backend.
- Creating or editing forwarding rules from chat.
- Generating or sending drafts from chat.
- Slack, Linear, MCP, or Google Workspace connector execution.
- Replacing Today, Inbox, Drafts, Rules, Runs, Accounts, or Settings.

## 4. User Experience

### 4.1 Route Behavior

`/` remains the assistant route and is still rendered through `AppShell`.

`Index.tsx` should continue to be lightweight, but it should render a real assistant home component instead of a mock chat experience. Acceptable implementation options:

- evolve `ChatPage` into `AssistantHomePage`;
- or create `AssistantHome.tsx` and have `Index.tsx` render it;
- or split the page into smaller components while keeping the route unchanged.

The URL `/today` remains the structured daily review route.

### 4.2 Empty and Loading States

The assistant home must handle:

- auth is loading: inherited from `ProtectedRoute`;
- accounts query loading: show assistant skeleton or "Loading workspace...";
- no connected accounts: show a connect-account CTA;
- connected accounts but no summary: show "No summary generated yet" and offer "Refresh summary";
- summary exists but no action items: show a calm "Nothing needs attention" state;
- API errors: show recoverable inline error cards, not a blank assistant page.

### 4.3 No Accounts State

If `GET /api/accounts` returns zero accounts, the assistant home should not show generic assistant suggestions. It should show:

- headline: "Connect an email account to start";
- copy explaining that the assistant needs at least one Microsoft mailbox;
- primary CTA linking to `/accounts`;
- secondary copy noting Work or school and Personal Outlook are supported.

This satisfies the initial spec requirement that the connect flow is discoverable without reading docs.

### 4.4 Attention Strip

When at least one account exists, the top of the assistant page should show a compact attention strip.

Minimum tiles:

- `Action items`: count of open action items from `GET /api/summaries`.
- `FYI`: count of FYI items from `GET /api/summaries`.
- `Accounts`: connected count and total account count.
- `Latest summary`: relative time from `summary.snapshot.created_at`, or "never".

Optional tile if data is already fetched:

- `Drafts ready`: count of draft suggestions from `GET /api/drafts`.

Do not show placeholder values such as `-` for optional counts if the data is not fetched. Omit the tile instead.

### 4.5 Outstanding Action Banner

If open action items exist, show a high-priority banner or card before generic suggestions:

> You have 3 open action items across 2 accounts.

Controls:

- `Review in Today` -> `/today`
- `Show source messages` -> `/inbox` with account/message links where practical

The banner should include account provenance. For example:

- group by account with `AccountBadge`;
- or show mini chips such as `Work: 2`, `Personal: 1`.

### 4.6 State-Aware Suggestions

Replace static suggestions with generated suggestions from current state.

Suggestion source priority:

1. No accounts:
   - "Connect your first Microsoft account" -> `/accounts`
2. Open action items:
   - "Review 3 open action items" -> `/today`
   - "Open the source inbox messages" -> `/inbox`
3. FYIs exist:
   - "Review 5 FYIs" -> `/today`
4. No summary:
   - "Refresh today's summary" -> refresh CTA or `/today`
5. Drafts exist:
   - "Review 2 ready drafts" -> `/drafts`
6. Recent failed runs:
   - "Inspect failed job run" -> `/runs`
7. Default connected state:
   - "Sync inboxes" -> account-specific sync flow or `/inbox`
   - "Manage categories and schedules" -> `/settings`

Phase 1 suggestion cards route to existing pages or trigger existing low-risk API calls. They do not open a free-form LLM chat.

### 4.7 Source Cards

Phase 1 should show concise cards for existing summary-derived objects:

Action item card fields:

- text;
- due date or overdue status if available;
- `AccountBadge`;
- source message link: `/inbox?message_id={message_id}&account_id={account_id}`;
- optional `Done` button if reusing `markActionItemDone`.

FYI card fields:

- text;
- `AccountBadge`;
- source message link;
- optional `Dismiss` button if reusing `dismissFYI`.

Card volume limits:

- show at most 3 action items on the assistant home;
- show at most 2 FYIs on the assistant home;
- link to Today for full lists.

### 4.8 Connector Chips

Connector chips should be accurate:

- Email:
  - `connected` if at least one account has `connection_status = connected`;
  - `needs connection` if no accounts;
  - `attention` if one or more accounts are `error` or `expired`.
- Slack, Linear, MCP servers:
  - `soon`;
  - disabled visual style;
  - no action except possibly routing to a future placeholder or Accounts.

Do not present future connectors as usable.

### 4.9 Composer Behavior

Phase 1 may keep a composer visually, but it must not imply real LLM execution.

Allowed Phase 1 composer behavior:

- disabled input with copy such as "Ask mode is coming in Phase 2";
- or a small command palette for predefined actions;
- or clicking state-aware suggestions fills local UI text but routes to existing pages.

If text input remains enabled, responses must be deterministic and clearly scoped to the predefined MVP commands. Do not call the LLM and do not persist conversations in Phase 1.

Recommended Phase 1 choice: replace free-form mock replies with state-aware cards and disable arbitrary send until Phase 2.

## 5. Data Composition

### 5.1 Account Scope

The assistant page receives `accountFilter` from `AppShell`:

- `all`: aggregate across all connected accounts;
- account id: restrict summary, drafts, and run queries to that account where APIs support it.

For `all`, use:

- `getSummary(accessToken)` for rolled-up summary if the backend supports all-account summary response;
- otherwise fetch per-account summaries and merge client-side with visible account provenance.

Existing `Today.tsx` currently calls `getSummary(accessToken!, activeAccountID)`, where `activeAccountID` is undefined for all accounts. Phase 1 should follow the same behavior unless backend evidence requires per-account fallback.

### 5.2 Query Keys

Use React Query keys that include:

- access token;
- account scope;
- data kind.

Suggested keys:

- `["assistant-home", "summary", accessToken, activeAccountID ?? "all"]`
- `["assistant-home", "drafts", accessToken, activeAccountID ?? "all"]`
- `["assistant-home", "runs", accessToken, activeAccountID ?? "all"]`

Existing global keys such as `["summary", accessToken, activeAccountID]` may be reused if shared invalidation with Today is more valuable than separation. Whichever strategy is chosen, invalidating after `markActionItemDone`, `dismissFYI`, or refresh must update both assistant home and Today.

### 5.3 Derived View Model

Build a local view model in the assistant page or a hook such as `useAssistantHomeData`.

Suggested type:

```ts
type AssistantHomeState = {
  scope: "all" | { accountId: string; label: string };
  accounts: UiAccount[];
  connectedAccounts: UiAccount[];
  erroredAccounts: UiAccount[];
  snapshot: SummarySnapshot | null;
  actionItems: SummaryActionItem[];
  fyi: SummaryFYI[];
  draftsReady?: number;
  latestRun?: JobRun;
  failedRuns: JobRun[];
  suggestions: AssistantSuggestion[];
};
```

Suggested suggestion type:

```ts
type AssistantSuggestion = {
  id: string;
  title: string;
  description: string;
  kind: "link" | "mutation";
  href?: string;
  action?: "refresh_summary" | "sync_accounts";
  priority: number;
};
```

### 5.4 Provenance Mapping

For every `SummaryActionItem` and `SummaryFYI`, find the matching `UiAccount` by `account_id`.

If no account is found:

- render the item with an "unknown account" fallback badge;
- do not hide the item;
- avoid claiming it belongs to "all accounts".

This preserves the parent PRD invariant that provenance ambiguity is a defect.

## 6. API Usage

### 6.1 Required Calls

On assistant home load:

1. `GET /api/accounts`
2. If accounts exist: `GET /api/summaries` or `GET /api/summaries?account_id={id}`

Optional calls:

3. `GET /api/drafts` or `GET /api/drafts?account_id={id}`
4. `GET /api/runs?limit=5` or `GET /api/runs?account_id={id}&limit=5`

### 6.2 Mutations

Phase 1 may reuse existing low-risk mutations:

- `POST /api/accounts/{id}/summaries/refresh`
- `POST /api/action-items/{id}/done`
- `POST /api/fyi/{id}/dismiss`

When mutations succeed:

- invalidate assistant summary data;
- invalidate Today summary data;
- invalidate runs when a job is queued;
- show toast feedback.

### 6.3 No New Backend Endpoint Required

Do not add a dedicated `GET /api/assistant/home` in Phase 1 unless frontend composition proves too slow or too duplicative.

If added later, the endpoint should return the same derived view model and must not include encrypted conversation content because Phase 1 has no persisted assistant conversations.

## 7. Frontend Implementation Plan

### 7.1 Components

Recommended component split:

- `web/src/pages/AssistantHome.tsx`
  - route-level page;
  - receives `accountFilter`.
- `web/src/hooks/useAssistantHomeData.ts`
  - composes accounts, summary, drafts, and runs.
- `web/src/components/assistant/AttentionStrip.tsx`
  - metric tiles.
- `web/src/components/assistant/ActionItemsPreview.tsx`
  - limited action item cards.
- `web/src/components/assistant/FYIPreview.tsx`
  - limited FYI cards.
- `web/src/components/assistant/AssistantSuggestions.tsx`
  - state-aware suggestion cards.
- `web/src/components/assistant/ConnectorChips.tsx`
  - connector availability display.

It is acceptable to implement fewer files initially if the page remains readable.

### 7.2 Files to Change

Minimum:

- `web/src/pages/Chat.tsx`
  - remove `mock-data` dependency;
  - remove `mockReply`;
  - replace static `SUGGESTIONS` with derived suggestions;
  - render real attention and provenance state.

Recommended:

- Add `web/src/pages/AssistantHome.tsx`.
- Update `web/src/pages/Index.tsx` to render `AssistantHome`.
- Leave `Chat.tsx` unused or delete it in a later cleanup.

Supporting:

- `web/src/lib/auth.ts`
  - add helper functions only if existing functions are insufficient.
- `web/src/components/AppSidebar.tsx`
  - optionally add a badge for open action item count next to Today.

### 7.3 Navigation Badges

Phase 1 should add at least one persistent action-item indicator outside the assistant page.

Preferred option:

- Add a Today nav badge in `AppSidebar`.
- Use a lightweight summary query or pass in a shared count from a top-level provider only if this does not cause excessive API calls.

Fallback option:

- Keep the indicator only on the assistant page in Phase 1, but add a documented follow-up task for sidebar badge support.

## 8. Error Handling

### 8.1 Accounts Error

If accounts fail to load:

- show "Could not load accounts";
- provide retry;
- keep navigation available.

### 8.2 Summary Error

If summary fails to load:

- show "Could not load today's intelligence";
- offer retry;
- keep account connection status visible.

### 8.3 Drafts or Runs Error

Draft and run data are secondary. If they fail:

- omit those tiles or show a subtle warning;
- do not block the page.

## 9. Accessibility

- Attention strip tiles must have readable text labels, not color-only meaning.
- Suggestion cards must be buttons or links with accessible names.
- Connector chips that are disabled must communicate "coming soon" in text.
- Action item and FYI cards must expose source links with descriptive labels.
- Keyboard users must be able to reach all CTAs.
- Loading and error states should not trap focus.

## 10. Performance

- Avoid polling on the assistant home in Phase 1 unless a user has just queued a job.
- Use React Query caching to avoid duplicate calls between Assistant and Today.
- Limit preview cards to small counts.
- Do not fetch full message bodies on the assistant home.
- Do not fetch all runs; use small limits.
- Avoid per-account summary fan-out unless backend all-account summary is unavailable.

## 11. Security and Privacy

- Phase 1 does not persist assistant conversations.
- Phase 1 must not log email-derived summary content beyond existing API behavior.
- Do not expose provider message ids unless already exposed in existing message APIs.
- Do not add analytics containing action item text, FYI text, draft bodies, or message previews.
- Continue to rely on existing API auth and account scoping.

Note: the PRD sets assistant conversation retention to 90 days by default for later phases. That retention requirement is not implemented in Phase 1 because Phase 1 has no conversation persistence.

## 12. Testing

### 12.1 Unit Tests

Add tests for derived state logic if implemented as pure helpers:

- no accounts -> connect CTA suggestions;
- accounts but no summary -> refresh suggestion;
- action items -> action banner and review suggestion;
- FYIs -> FYI suggestion;
- drafts -> drafts suggestion;
- failed runs -> runs suggestion;
- account scope filters displayed items.

### 12.2 Component Tests

Use React Testing Library where available:

- renders no-account CTA;
- renders attention strip counts from mocked query data;
- renders account badges for action items and FYIs;
- does not render mock reply copy;
- "Review in Today" links to `/today`;
- source message links include both `message_id` and `account_id`.

### 12.3 Manual Verification

Manual checks:

1. Login with no accounts:
   - `/` shows connect CTA.
2. Login with connected account and no summary:
   - `/` shows connected account state and refresh path.
3. Account with summary/action items:
   - `/` shows open action count immediately.
   - Action items include account badges.
   - Today still shows the full list.
4. Account with FYIs:
   - `/` shows FYI count and preview.
5. Account with failed run:
   - `/` surfaces a route to Runs.
6. Account filter changed from all to one account:
   - counts and cards update to that account's scope.

## 13. Acceptance Criteria

Phase 1 is complete when:

- `/` no longer depends on `web/src/lib/mock-data`.
- `/` no longer returns mock assistant replies.
- `/` shows a real attention strip backed by existing APIs.
- A user with open action items sees them or a clear count immediately on load.
- A user with no connected accounts sees a first-class connect-account CTA.
- Surfaced action items and FYIs include account provenance.
- Suggestions are state-aware and route to existing product surfaces.
- Today remains available and consistent with assistant home data.
- Tests cover the main derived-state branches or component states.

## 14. Future Phases

Phase 2 should introduce encrypted assistant conversation persistence, resumable conversations, and constrained read-only assistant answers. Phase 1 should avoid building temporary data structures that conflict with the later `assistant_conversations` and `assistant_messages` model.

---

*This Phase 1 spec intentionally uses existing APIs and product state. It makes the home experience AI-first without introducing unbounded agent behavior before provenance, persistence, and safety controls are implemented.*
