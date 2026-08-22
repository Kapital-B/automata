import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AssistantHomePage from "@/pages/AssistantHome";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";
import * as auth from "@/lib/auth";

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
    getAttention: vi.fn(),
    listProjects: vi.fn(),
    getUnassignedSummary: vi.fn(),
    getCurrentPosition: vi.fn(),
    markActionItemDone: vi.fn(),
  };
});

const mockedUseAssistantHomeData = vi.mocked(useAssistantHomeData);
const getAttention = vi.mocked(auth.getAttention);
const listProjects = vi.mocked(auth.listProjects);
const getUnassignedSummary = vi.mocked(auth.getUnassignedSummary);
const getCurrentPosition = vi.mocked(auth.getCurrentPosition);

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

function baseMailState(overrides: Partial<ReturnType<typeof useAssistantHomeData>> = {}) {
  return {
    activeAccountID: undefined,
    accounts: [
      {
        id: "a1",
        label: "Work",
        primaryEmail: "work@example.com",
        kind: "work" as const,
        status: "connected" as const,
        colorVar: "acct-1",
      },
    ],
    connectedAccounts: [
      {
        id: "a1",
        label: "Work",
        primaryEmail: "work@example.com",
        kind: "work" as const,
        status: "connected" as const,
        colorVar: "acct-1",
      },
    ],
    erroredAccounts: [],
    summary: null,
    actionItems: [] as auth.SummaryActionItem[],
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
    ...overrides,
  };
}

describe("AssistantHomePage U2", () => {
  beforeEach(() => {
    mockedUseAssistantHomeData.mockReset();
    getAttention.mockReset();
    listProjects.mockReset();
    getUnassignedSummary.mockReset();
    getCurrentPosition.mockReset();
    getAttention.mockResolvedValue({
      items: [],
      counts: {
        total: 0,
        issue_assignee: 0,
        member_role: 0,
        provisional_fact: 0,
        provisional_decision: 0,
        open_contradiction: 0,
        mail_action_item: 0,
      },
    });
    listProjects.mockResolvedValue([]);
    getUnassignedSummary.mockResolvedValue({ unassigned: 0, provisional: 0 });
    getCurrentPosition.mockResolvedValue({ facts: [], decisions: [] });
  });

  it("shows empty Needs my input with Projects and Triage CTAs", async () => {
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    expect(await screen.findByRole("heading", { name: "Needs my input" })).toBeInTheDocument();
    expect(screen.getByText(/Nothing waiting on you/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Projects$/i })).toHaveAttribute("href", "/projects");
    expect(screen.getByRole("link", { name: /^Triage$/i })).toHaveAttribute("href", "/triage");
  });

  it("makes Needs my input the hero list merging attention and mail", async () => {
    getAttention.mockResolvedValue({
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
    });
    listProjects.mockResolvedValue([
      {
        id: "p1",
        organisation_id: "o1",
        name: "Cooling",
        code: "DC01",
        keywords: [],
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-08-01T00:00:00Z",
      },
    ]);
    getUnassignedSummary.mockResolvedValue({ unassigned: 2, provisional: 1 });
    getCurrentPosition.mockResolvedValue({
      facts: [
        {
          fact_id: "f1",
          subject_key: "duty",
          label: "Duty",
          version_id: "v1",
          value_json: 90,
          value_text: "90 kW",
          evidence_count: 1,
        },
      ],
      decisions: [],
    });
    mockedUseAssistantHomeData.mockReturnValue(
      baseMailState({
        actionItems: [
          {
            id: "i1",
            account_id: "a1",
            message_id: "m1",
            text: "Reply to invoice",
            created_at: "2026-08-01T00:00:00Z",
            is_overdue: false,
          },
        ],
        draftsReady: 2,
      }),
    );
    renderPage();

    expect(await screen.findByRole("heading", { name: "Needs my input" })).toBeInTheDocument();
    expect(screen.getByText(/Confirm decision: Proceed with 90 kW/i)).toBeInTheDocument();
    expect(screen.getByText("Reply to invoice")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Action items" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Suggestions" })).not.toBeInTheDocument();

    expect(await screen.findByRole("heading", { name: "Recent projects" })).toBeInTheDocument();
    expect(screen.getByText("DC01")).toBeInTheDocument();
    expect(await screen.findByText(/Duty: 90 kW/i)).toBeInTheDocument();

    expect(screen.getByText(/3 items waiting to be assigned/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /2 drafts ready/i })).toHaveAttribute("href", "/drafts");
  });
});
