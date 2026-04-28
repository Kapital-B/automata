import { describe, expect, it } from "vitest";
import { buildAssistantSuggestions, type AssistantHomeState } from "@/hooks/useAssistantHomeData";

function baseState(): AssistantHomeState {
  return {
    activeAccountID: undefined,
    accounts: [],
    connectedAccounts: [],
    erroredAccounts: [],
    summary: null,
    actionItems: [],
    fyi: [],
    draftSuggestions: [],
    draftsReady: 0,
    runs: [],
    latestRun: undefined,
    failedRuns: [],
    suggestions: [],
  };
}

describe("buildAssistantSuggestions", () => {
  it("returns connect CTA with no connected accounts", () => {
    const out = buildAssistantSuggestions(baseState());
    expect(out[0]?.id).toBe("connect-first-account");
    expect(out[0]?.href).toBe("/accounts");
  });

  it("prioritizes action-item suggestions", () => {
    const state = baseState();
    state.connectedAccounts = [
      {
        id: "a1",
        label: "Work",
        primaryEmail: "work@example.com",
        kind: "work",
        status: "connected",
        colorVar: "acct-1",
      },
    ];
    state.actionItems = [
      { id: "i1", account_id: "a1", message_id: "m1", text: "Reply", is_overdue: false },
    ];
    const out = buildAssistantSuggestions(state);
    expect(out[0]?.id).toBe("review-action-items");
    expect(out[1]?.id).toBe("open-source-messages");
  });
});
