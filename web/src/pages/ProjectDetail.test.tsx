import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProjectDetailPage from "@/pages/ProjectDetail";
import * as auth from "@/lib/auth";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({
    accounts: [
      {
        id: "acc1",
        label: "Work",
        primaryEmail: "work@ex.com",
        colorVar: "acct-1",
        status: "connected",
      },
    ],
  }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    getProject: vi.fn(),
    getProjectTimeline: vi.fn(),
    createManualItem: vi.fn(),
    listContacts: vi.fn(),
    listProjectIssues: vi.fn(),
    createProjectIssue: vi.fn(),
    suggestProjectIssue: vi.fn(),
    getApiHealth: vi.fn(),
    addIssueItem: vi.fn(),
    updateProject: vi.fn(),
    updateProjectMember: vi.fn(),
    getCurrentPosition: vi.fn(),
    listProjectFacts: vi.fn(),
    createProjectFact: vi.fn(),
    confirmFactVersion: vi.fn(),
    rejectFactVersion: vi.fn(),
    listProjectInterpretations: vi.fn(),
    interpretProject: vi.fn(),
    dismissInterpretation: vi.fn(),
    listProjectContradictions: vi.fn(),
    reconcileProject: vi.fn(),
    resolveContradiction: vi.fn(),
    listProjectDecisions: vi.fn(),
    createProjectDecision: vi.fn(),
    confirmDecision: vi.fn(),
    withdrawDecision: vi.fn(),
  };
});

const getProject = vi.mocked(auth.getProject);
const getProjectTimeline = vi.mocked(auth.getProjectTimeline);
const createManualItem = vi.mocked(auth.createManualItem);
const listContacts = vi.mocked(auth.listContacts);
const listProjectIssues = vi.mocked(auth.listProjectIssues);
const createProjectIssue = vi.mocked(auth.createProjectIssue);
const suggestProjectIssue = vi.mocked(auth.suggestProjectIssue);
const getApiHealth = vi.mocked(auth.getApiHealth);
const addIssueItem = vi.mocked(auth.addIssueItem);
const getCurrentPosition = vi.mocked(auth.getCurrentPosition);
const listProjectFacts = vi.mocked(auth.listProjectFacts);
const createProjectFact = vi.mocked(auth.createProjectFact);
const listProjectInterpretations = vi.mocked(auth.listProjectInterpretations);
const interpretProject = vi.mocked(auth.interpretProject);
const dismissInterpretation = vi.mocked(auth.dismissInterpretation);
const listProjectContradictions = vi.mocked(auth.listProjectContradictions);
const reconcileProject = vi.mocked(auth.reconcileProject);
const resolveContradiction = vi.mocked(auth.resolveContradiction);
const listProjectDecisions = vi.mocked(auth.listProjectDecisions);

describe("Project timeline UI", () => {
  beforeEach(() => {
    getProject.mockReset();
    getProjectTimeline.mockReset();
    createManualItem.mockReset();
    listContacts.mockReset();
    listProjectIssues.mockReset();
    createProjectIssue.mockReset();
    suggestProjectIssue.mockReset();
    getApiHealth.mockReset();
    addIssueItem.mockReset();
    getCurrentPosition.mockReset();
    listProjectFacts.mockReset();
    createProjectFact.mockReset();
    listProjectInterpretations.mockReset();
    interpretProject.mockReset();
    dismissInterpretation.mockReset();
    listProjectContradictions.mockReset();
    reconcileProject.mockReset();
    resolveContradiction.mockReset();
    listProjectDecisions.mockReset();
    listContacts.mockResolvedValue([]);
    listProjectIssues.mockResolvedValue([]);
    getCurrentPosition.mockResolvedValue({ facts: [], decisions: [] });
    listProjectFacts.mockResolvedValue([]);
    listProjectInterpretations.mockResolvedValue([]);
    listProjectContradictions.mockResolvedValue([]);
    listProjectDecisions.mockResolvedValue([]);
    getApiHealth.mockResolvedValue({ status: "ok", llm: true });
    suggestProjectIssue.mockResolvedValue({
      title: "Pump P-03",
      item_refs: [{ manual_item_id: "man1" }],
      confidence: 0.9,
      reason: "mentions P-03",
    });
    getProject.mockResolvedValue({
      id: "p1",
      organisation_id: "o1",
      name: "Cooling Upgrade",
      code: "DC01",
      keywords: [],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      member: { id: "m1", project_id: "p1", user_id: "u1", role: "ME", created_at: "", updated_at: "" },
    });
    getProjectTimeline.mockResolvedValue([
      {
        source: "manual",
        occurred_at: "2026-03-02T15:00:00Z",
        title: "Teams note",
        snippet: "Consider 90 kW",
        contacts: [],
        manual_item_id: "man1",
        channel: "teams",
        body_text: "Consider 90 kW",
      },
      {
        source: "mail",
        occurred_at: "2026-03-01T10:00:00Z",
        title: "Outlook: pump",
        snippet: "pump sizing",
        contacts: [],
        account_id: "acc1",
        account_label: "Work",
        message_id: "msg1",
      },
    ]);
  });

  it("suggests an issue and creates with item refs on confirm", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    createProjectIssue.mockResolvedValue({
      id: "iss1",
      organisation_id: "o1",
      project_id: "p1",
      title: "Pump P-03",
      current_position_note: "",
      status: "open",
      awaiting_me: false,
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      items: [],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /suggest issue/i }));
    await waitFor(() => expect(suggestProjectIssue).toHaveBeenCalledWith("token", "p1"));
    expect(await screen.findByDisplayValue("Pump P-03")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", {
        title: "Pump P-03",
        current_position_note: undefined,
        item_refs: [{ manual_item_id: "man1" }],
      }),
    );
  });

  it("creates an issue from the project page", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    createProjectIssue.mockResolvedValue({
      id: "iss1",
      organisation_id: "o1",
      project_id: "p1",
      title: "Pump P-03",
      current_position_note: "",
      status: "open",
      awaiting_me: false,
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      items: [],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^new issue$/i }));
    fireEvent.change(screen.getByPlaceholderText(/pump p-03/i), {
      target: { value: "Pump P-03" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", {
        title: "Pump P-03",
        current_position_note: undefined,
        item_refs: undefined,
      }),
    );
  });

  it("attaches a timeline item to an issue", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    listProjectIssues.mockResolvedValue([
      {
        id: "iss1",
        organisation_id: "o1",
        project_id: "p1",
        title: "Pump P-03",
        current_position_note: "",
        status: "open",
        awaiting_me: false,
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
      },
    ]);
    addIssueItem.mockResolvedValue({
      id: "iss1",
      organisation_id: "o1",
      project_id: "p1",
      title: "Pump P-03",
      current_position_note: "",
      status: "open",
      awaiting_me: false,
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      items: [],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    const attachSelect = screen.getAllByLabelText(/attach to issue/i)[0]!;
    fireEvent.change(attachSelect, { target: { value: "iss1" } });
    fireEvent.click(screen.getAllByRole("button", { name: /^attach$/i })[0]!);
    await waitFor(() =>
      expect(addIssueItem).toHaveBeenCalledWith("token", "iss1", {
        message_id: undefined,
        manual_item_id: "man1",
      }),
    );
  });

  it("renders timeline mail and manual in order and paste submits", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    createManualItem.mockResolvedValue({
      id: "man2",
      organisation_id: "o1",
      channel: "whatsapp",
      occurred_at: "2026-03-03T00:00:00Z",
      title: "WA",
      body_text: "approved",
      project_id: "p1",
      assignment_status: "committed",
      created_at: "2026-03-03T00:00:00Z",
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    expect(screen.getByText("Outlook: pump")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /paste correspondence/i }));
    fireEvent.change(screen.getByLabelText(/^body$/i), {
      target: { value: "90 kW is approved" },
    });
    fireEvent.click(screen.getByRole("button", { name: /add to timeline/i }));
    await waitFor(() =>
      expect(createManualItem).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          body_text: "90 kW is approved",
          project_id: "p1",
        }),
      ),
    );
  });

  it("shows current position and creates a confirmed fact", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    getCurrentPosition.mockResolvedValue({
      facts: [
        {
          fact_id: "f1",
          subject_key: "pump.p03.duty_kw",
          label: "Pump P-03 duty",
          version_id: "v1",
          value_json: 90,
          value_text: "90",
          unit: "kW",
          evidence_count: 1,
        },
      ],
      decisions: [],
    });
    listProjectFacts.mockResolvedValue([
      {
        id: "f1",
        organisation_id: "o1",
        project_id: "p1",
        subject_key: "pump.p03.duty_kw",
        label: "Pump P-03 duty",
        created_at: "2026-03-01T00:00:00Z",
        updated_at: "2026-03-02T00:00:00Z",
        versions: [
          {
            id: "v1",
            fact_id: "f1",
            status: "active",
            value_json: 90,
            value_text: "90",
            unit: "kW",
            source: "user",
            created_at: "2026-03-02T00:00:00Z",
            evidence: [],
          },
        ],
      },
    ]);
    createProjectFact.mockResolvedValue({
      id: "f2",
      organisation_id: "o1",
      project_id: "p1",
      subject_key: "pump.p03.flow",
      label: "Pump P-03 flow",
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      versions: [
        {
          id: "v2",
          fact_id: "f2",
          status: "active",
          value_json: 12,
          value_text: "12",
          unit: "L/s",
          source: "user",
          created_at: "2026-03-03T00:00:00Z",
          evidence: [],
        },
      ],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const position = await screen.findByLabelText(/current position/i);
    expect(position).toHaveTextContent("Pump P-03 duty");
    expect(position).toHaveTextContent("90 kW");
    expect(screen.getByRole("heading", { name: /^facts$/i })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /^add fact$/i }));
    fireEvent.change(screen.getByPlaceholderText("pump.p03.duty_kw"), {
      target: { value: "pump.p03.flow" },
    });
    fireEvent.change(screen.getByPlaceholderText("Pump P-03 duty"), {
      target: { value: "Pump P-03 flow" },
    });
    fireEvent.change(screen.getByPlaceholderText("90"), { target: { value: "12" } });
    fireEvent.change(screen.getByPlaceholderText("kW"), { target: { value: "L/s" } });
    fireEvent.click(screen.getByRole("button", { name: /save fact/i }));

    await waitFor(() =>
      expect(createProjectFact).toHaveBeenCalledWith(
        "token",
        "p1",
        expect.objectContaining({
          subject_key: "pump.p03.flow",
          label: "Pump P-03 flow",
          value: 12,
          unit: "L/s",
          confirm: true,
        }),
      ),
    );
  });

  it("lists pending interpretations and dismisses them", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    listProjectInterpretations.mockResolvedValue([
      {
        id: "interp1",
        organisation_id: "o1",
        project_id: "p1",
        status: "pending",
        reason: "duty language",
        confidence: 0.8,
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        sources: [{ id: "s1", interpretation_id: "interp1", manual_item_id: "man1" }],
        candidates: [
          {
            kind: "fact",
            subject_key: "pump.p03.duty_kw",
            label: "Pump P-03 duty",
            value: 90,
            unit: "kW",
            confidence: 0.8,
            reason: "Teams note",
          },
        ],
      },
    ]);
    dismissInterpretation.mockResolvedValue({
      id: "interp1",
      organisation_id: "o1",
      project_id: "p1",
      status: "dismissed",
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      sources: [],
      candidates: [],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: /^interpretations$/i })).toBeInTheDocument();
    expect(await screen.findByText(/Pump P-03 duty/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^dismiss$/i }));
    await waitFor(() => expect(dismissInterpretation).toHaveBeenCalledWith("token", "interp1"));
  });

  it("reconciles pending interpretations and resolves contradictions", async () => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    listProjectInterpretations.mockResolvedValue([
      {
        id: "interp1",
        organisation_id: "o1",
        project_id: "p1",
        status: "pending",
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        sources: [],
        candidates: [
          {
            kind: "fact",
            subject_key: "pump.p03.duty_kw",
            label: "Pump P-03 duty",
            value: 90,
            unit: "kW",
            confidence: 0.4,
          },
        ],
      },
    ]);
    listProjectContradictions.mockResolvedValue([
      {
        id: "c1",
        organisation_id: "o1",
        project_id: "p1",
        status: "open",
        summary: 'pump.p03.duty_kw: active "75 kW" vs proposed "90 kW"',
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        sides: [
          { id: "s1", contradiction_id: "c1", fact_version_id: "v-active" },
          { id: "s2", contradiction_id: "c1", fact_version_id: "v-proposed" },
        ],
      },
    ]);
    reconcileProject.mockResolvedValue({
      processed_interpretations: 1,
      outcomes: [{ kind: "fact", outcome: "contradiction", reason: "conflict" }],
      contradictions_opened: 1,
    });
    resolveContradiction.mockResolvedValue({
      id: "c1",
      organisation_id: "o1",
      project_id: "p1",
      status: "resolved",
      summary: "done",
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      sides: [],
    });

    render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={["/projects/p1"]}>
          <Routes>
            <Route path="/projects/:id" element={<ProjectDetailPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByRole("heading", { name: /^contradictions$/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^reconcile$/i }));
    await waitFor(() => expect(reconcileProject).toHaveBeenCalledWith("token", "p1"));
    fireEvent.click(screen.getByRole("button", { name: /^keep proposed$/i }));
    await waitFor(() =>
      expect(resolveContradiction).toHaveBeenCalledWith("token", "c1", {
        resolution: "supersede",
        keep_fact_version_id: "v-proposed",
      }),
    );
  });
});
