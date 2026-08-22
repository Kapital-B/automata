import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AssistantHomePage from "@/pages/AssistantHome";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/hooks/useAssistantHomeData", () => ({
  useAssistantHomeData: vi.fn(),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    getAttention: vi.fn().mockResolvedValue({
      items: [
        {
          id: "decision:d1",
          why_me: "provisional_decision",
          title: "Confirm decision: Proceed with 90 kW",
          project_id: "p1",
          project_name: "Cooling",
          ref_type: "decision",
          ref_id: "d1",
        },
      ],
      counts: {
        total: 1,
        issue_assignee: 0,
        member_role: 0,
        provisional_fact: 0,
        provisional_decision: 1,
        open_contradiction: 0,
        mail_action_item: 0,
      },
    }),
  };
});

const mockedUseAssistantHomeData = vi.mocked(useAssistantHomeData);

function renderPage() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AssistantHomePage accountFilter="all" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AssistantHomePage", () => {
  beforeEach(() => {
    mockedUseAssistantHomeData.mockReset();
  });

  it("shows connect CTA when no accounts are connected", () => {
    mockedUseAssistantHomeData.mockReturnValue({
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
      isLoading: false,
      accountsError: null,
      summaryError: null,
      draftsError: null,
      runsError: null,
    });
    renderPage();
    expect(screen.getByText("Connect an email account to start")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Accounts" })).toBeInTheDocument();
  });

  it("renders attention and source cards from summary state", async () => {
    mockedUseAssistantHomeData.mockReturnValue({
      activeAccountID: undefined,
      accounts: [
        {
          id: "a1",
          label: "Work",
          primaryEmail: "work@example.com",
          kind: "work",
          status: "connected",
          colorVar: "acct-1",
        },
      ],
      connectedAccounts: [
        {
          id: "a1",
          label: "Work",
          primaryEmail: "work@example.com",
          kind: "work",
          status: "connected",
          colorVar: "acct-1",
        },
      ],
      erroredAccounts: [],
      summary: {
        snapshot: {
          id: "s1",
          run_id: "r1",
          window_start: new Date().toISOString(),
          window_end: new Date().toISOString(),
          general_summary: "summary",
          created_at: new Date().toISOString(),
        },
        action_items: [{ id: "i1", account_id: "a1", message_id: "m1", text: "Reply to invoice", is_overdue: false }],
        fyi: [{ id: "f1", account_id: "a1", message_id: "m2", text: "Policy update" }],
      },
      actionItems: [{ id: "i1", account_id: "a1", message_id: "m1", text: "Reply to invoice", is_overdue: false }],
      fyi: [{ id: "f1", account_id: "a1", message_id: "m2", text: "Policy update" }],
      draftSuggestions: [],
      draftsReady: 0,
      runs: [],
      latestRun: undefined,
      failedRuns: [],
      suggestions: [{ id: "review-action-items", title: "Review 1 open action item", description: "See on Home", href: "/" }],
      isLoading: false,
      accountsError: null,
      summaryError: null,
      draftsError: null,
      runsError: null,
    });
    renderPage();
    expect(screen.getByRole("heading", { name: "Action items" })).toBeInTheDocument();
    expect(screen.getByText("Reply to invoice")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Message m1/i })).toBeInTheDocument();
    expect(screen.queryByText(/Replies are mocked while we wire up the backend/i)).not.toBeInTheDocument();
    expect(await screen.findByText("Needs my input")).toBeInTheDocument();
    expect(await screen.findByText(/Confirm decision: Proceed with 90 kW/i)).toBeInTheDocument();
  });
});
