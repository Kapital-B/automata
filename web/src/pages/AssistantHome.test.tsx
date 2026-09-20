import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
    getOverview: vi.fn(),
    listActivity: vi.fn(),
    markActionItemDone: vi.fn(),
    getApiHealth: vi.fn(),
    askAcross: vi.fn(),
  };
});

const mockedUseAssistantHomeData = vi.mocked(useAssistantHomeData);
const getAttention = vi.mocked(auth.getAttention);
const getOverview = vi.mocked(auth.getOverview);
const listActivity = vi.mocked(auth.listActivity);
const getApiHealth = vi.mocked(auth.getApiHealth);
const askAcross = vi.mocked(auth.askAcross);

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
        colorVar: "acct-1" as const,
      },
    ],
    connectedAccounts: [
      {
        id: "a1",
        label: "Work",
        primaryEmail: "work@example.com",
        kind: "work" as const,
        status: "connected" as const,
        colorVar: "acct-1" as const,
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

describe("AssistantHomePage", () => {
  beforeEach(() => {
    mockedUseAssistantHomeData.mockReset();
    getAttention.mockReset();
    getOverview.mockReset();
    listActivity.mockReset();
    getApiHealth.mockReset();
    askAcross.mockReset();
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
    getOverview.mockResolvedValue({
      counts: {
        needs_you: 0,
        triage_unassigned: 0,
        triage_provisional: 0,
        open_contradictions: 0,
        provisional_facts: 0,
        proposed_decisions: 0,
        active_projects: 0,
      },
      projects: [],
    });
    listActivity.mockResolvedValue({ items: [] });
    getApiHealth.mockResolvedValue({ status: "ok", llm: true });
  });

  it("shows the overview heading and an empty attention list with CTAs", async () => {
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    expect(await screen.findByRole("heading", { name: "Across your projects" })).toBeInTheDocument();
    expect(screen.getByText(/Nothing waiting on you/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Projects$/i })).toHaveAttribute("href", "/projects");
    expect(screen.getByRole("link", { name: /^Triage$/i })).toHaveAttribute("href", "/triage");
  });

  it("renders the metric card row, including zeroes", async () => {
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    // A card that vanishes at zero makes the row jump between loads.
    for (const label of ["Needs you", "Triage", "Contradictions", "Unconfirmed", "Projects"]) {
      expect(await screen.findByRole("link", { name: new RegExp(`^${label}, \\d+$`) })).toBeInTheDocument();
    }
    expect(screen.getByRole("link", { name: "Triage, 0" })).toHaveAttribute("href", "/triage");
  });

  it("surfaces counts on the cards", async () => {
    getOverview.mockResolvedValue({
      counts: {
        needs_you: 4,
        triage_unassigned: 2,
        triage_provisional: 1,
        open_contradictions: 3,
        provisional_facts: 5,
        proposed_decisions: 2,
        active_projects: 9,
      },
      projects: [],
    });
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    expect(await screen.findByRole("link", { name: "Triage, 3" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Contradictions, 3" })).toBeInTheDocument();
    // Unconfirmed merges provisional facts and proposed decisions.
    expect(screen.getByRole("link", { name: "Unconfirmed, 7" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Projects, 9" })).toBeInTheDocument();
  });

  it("shows what changed across projects, grouped and attributed", async () => {
    const now = new Date();
    listActivity.mockResolvedValue({
      items: [
        {
          kind: "decision_accepted",
          occurred_at: now.toISOString(),
          project_id: "p1",
          project_code: "DC01",
          project_name: "Cooling",
          title: "Proceed with 90 kW",
          ref_type: "decision",
          ref_id: "d1",
          source: "llm",
        },
        {
          kind: "contradiction_opened",
          occurred_at: new Date(now.getTime() - 26 * 60 * 60 * 1000).toISOString(),
          project_id: "p1",
          project_code: "DC01",
          project_name: "Cooling",
          title: "Duty stated twice",
          ref_type: "contradiction",
          ref_id: "c1",
        },
      ],
    });
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();

    expect(await screen.findByRole("heading", { name: "What changed" })).toBeInTheDocument();
    expect(screen.getByText("Decision accepted")).toBeInTheDocument();
    expect(screen.getByText("Contradiction opened")).toBeInTheDocument();
    // Provenance matters now that the LLM writes facts and decisions.
    expect(screen.getByText("llm")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Proceed with 90 kW" })).toHaveAttribute(
      "href",
      "/projects/p1?mode=position",
    );
    expect(screen.getByRole("link", { name: "Duty stated twice" })).toHaveAttribute(
      "href",
      "/projects/p1?mode=open",
    );
    expect(screen.getByRole("heading", { name: "Today" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Yesterday" })).toBeInTheDocument();
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
        {
          id: "mail:i1",
          why_me: "mail_action_item",
          title: "Reply to invoice",
          ref_type: "action_item",
          ref_id: "i1",
          account_id: "a1",
          message_id: "m1",
        },
      ],
      counts: {
        total: 2,
        issue_assignee: 0,
        member_role: 0,
        provisional_fact: 0,
        provisional_decision: 1,
        open_contradiction: 0,
        mail_action_item: 1,
      },
    });
    getOverview.mockResolvedValue({
      counts: {
        needs_you: 2,
        triage_unassigned: 2,
        triage_provisional: 1,
        open_contradictions: 0,
        provisional_facts: 0,
        proposed_decisions: 1,
        active_projects: 1,
      },
      projects: [
        {
          id: "p1",
          code: "DC01",
          name: "Cooling",
          teaser: "Duty: 90 kW",
          last_activity_at: new Date().toISOString(),
          attention_count: 2,
        },
      ],
    });
    mockedUseAssistantHomeData.mockReturnValue(baseMailState({ draftsReady: 2 }));
    renderPage();

    expect(await screen.findByRole("heading", { name: "Across your projects" })).toBeInTheDocument();
    expect(screen.getByText(/Confirm decision: Proceed with 90 kW/i)).toBeInTheDocument();
    expect(screen.getByText("Reply to invoice")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Action items" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Suggestions" })).not.toBeInTheDocument();

    expect(await screen.findByRole("heading", { name: "Projects" })).toBeInTheDocument();
    expect(screen.getByText("DC01")).toBeInTheDocument();
    // The teaser arrives with the overview, not from a per-project request.
    expect(await screen.findByText(/Duty: 90 kW/i)).toBeInTheDocument();
    expect(screen.getByText(/2 needs you/i)).toBeInTheDocument();

    expect(screen.getByText(/3 items waiting to be assigned/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /2 drafts ready/i })).toHaveAttribute("href", "/drafts");
    expect(
      screen.getByRole("link", { name: /Confirm decision: Proceed with 90 kW/i }),
    ).toHaveAttribute("href", "/projects/p1?mode=position");
    expect(screen.getByRole("link", { name: /Reply to invoice/i })).toHaveAttribute(
      "href",
      "/inbox?message_id=m1&account_id=a1",
    );
  });

  it("places Ask between the cards and the actions", async () => {
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    await screen.findByRole("heading", { name: "Across your projects" });

    const cards = screen.getByRole("navigation", { name: /overview/i });
    const ask = screen.getByRole("region", { name: /ask across projects/i });
    const actions = screen.getByRole("heading", { name: "Needs you" });

    // Ask sits after the card row and before the attention list.
    expect(cards.compareDocumentPosition(ask) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(ask.compareDocumentPosition(actions) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("shows five actions and expands to the rest", async () => {
    const items = Array.from({ length: 8 }, (_, i) => ({
      id: `decision:d${i}`,
      why_me: "provisional_decision",
      title: `Confirm decision number ${i}`,
      project_id: "p1",
      project_name: "Cooling",
      ref_type: "decision",
      ref_id: `d${i}`,
    }));
    getAttention.mockResolvedValue({
      items,
      counts: {
        total: 8,
        issue_assignee: 0,
        member_role: 0,
        provisional_fact: 0,
        provisional_decision: 8,
        open_contradiction: 0,
        mail_action_item: 0,
      },
    });
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();

    expect(await screen.findByText("Confirm decision number 0")).toBeInTheDocument();
    expect(screen.getByText("Confirm decision number 4")).toBeInTheDocument();
    // A long queue should not push the rest of the page off screen.
    expect(screen.queryByText("Confirm decision number 5")).not.toBeInTheDocument();

    const expand = screen.getByRole("button", { name: /show all 8/i });
    expect(expand).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(expand);

    expect(await screen.findByText("Confirm decision number 7")).toBeInTheDocument();
    const collapse = screen.getByRole("button", { name: /show fewer/i });
    expect(collapse).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(collapse);
    await waitFor(() =>
      expect(screen.queryByText("Confirm decision number 7")).not.toBeInTheDocument(),
    );
  });

  it("does not offer to expand five or fewer actions", async () => {
    getAttention.mockResolvedValue({
      items: [
        {
          id: "decision:d1",
          why_me: "provisional_decision",
          title: "Only one",
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
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    renderPage();
    expect(await screen.findByText("Only one")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /show all/i })).not.toBeInTheDocument();
  });

  it("asks across projects and shows cited answer", async () => {
    mockedUseAssistantHomeData.mockReturnValue(baseMailState());
    askAcross.mockResolvedValue({
      answer: "Pump P-03 duty is 90 kW on DC01",
      citations: [
        {
          type: "fact_version",
          id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
          project_id: "p1",
          project_code: "DC01",
          project_name: "Cooling",
        },
      ],
      confidence: 0.9,
    });
    renderPage();
    expect(await screen.findByRole("region", { name: /ask across projects/i })).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText(/ask across projects/i), {
      target: { value: "What is pump duty?" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^Ask$/i }));
    await waitFor(() => expect(askAcross).toHaveBeenCalledWith("token", "What is pump duty?"));
    expect(await screen.findByText(/Pump P-03 duty is 90 kW on DC01/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /DC01 · fact_version · aaaaaaaa/i })).toHaveAttribute(
      "href",
      "/projects/p1?mode=position",
    );
  });
});
