import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
    discardIssue: vi.fn(),
    getApiHealth: vi.fn(),
    addIssueItem: vi.fn(),
    updateProject: vi.fn(),
    updateProjectMember: vi.fn(),
    getCurrentPosition: vi.fn(),
    listProjectFacts: vi.fn(),
    createProjectFact: vi.fn(),
    confirmFactVersion: vi.fn(),
    rejectFactVersion: vi.fn(),
    extractProject: vi.fn(),
    listProjectContradictions: vi.fn(),
    resolveContradiction: vi.fn(),
    listProjectDecisions: vi.fn(),
    createProjectDecision: vi.fn(),
    confirmDecision: vi.fn(),
    withdrawDecision: vi.fn(),
    askProject: vi.fn(),
    getProjectAttention: vi.fn(),
  };
});

const getProject = vi.mocked(auth.getProject);
const getProjectTimeline = vi.mocked(auth.getProjectTimeline);
const createManualItem = vi.mocked(auth.createManualItem);
const listContacts = vi.mocked(auth.listContacts);
const listProjectIssues = vi.mocked(auth.listProjectIssues);
const createProjectIssue = vi.mocked(auth.createProjectIssue);
const discardIssue = vi.mocked(auth.discardIssue);
const getApiHealth = vi.mocked(auth.getApiHealth);
const addIssueItem = vi.mocked(auth.addIssueItem);
const getCurrentPosition = vi.mocked(auth.getCurrentPosition);
const listProjectFacts = vi.mocked(auth.listProjectFacts);
const createProjectFact = vi.mocked(auth.createProjectFact);
const confirmFactVersion = vi.mocked(auth.confirmFactVersion);
const extractProject = vi.mocked(auth.extractProject);
const listProjectContradictions = vi.mocked(auth.listProjectContradictions);
const resolveContradiction = vi.mocked(auth.resolveContradiction);
const listProjectDecisions = vi.mocked(auth.listProjectDecisions);
const confirmDecision = vi.mocked(auth.confirmDecision);
const askProject = vi.mocked(auth.askProject);
const getProjectAttention = vi.mocked(auth.getProjectAttention);

function selectMode(name: RegExp) {
  fireEvent.click(screen.getByRole("tab", { name }));
}

function newClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

function renderPage(client: QueryClient, entry = "/projects/p1") {
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[entry]}>
        <Routes>
          <Route path="/projects/:id" element={<ProjectDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const issue = (over: Partial<auth.IssueListItem> = {}): auth.IssueListItem => ({
  id: "iss1",
  organisation_id: "o1",
  project_id: "p1",
  title: "Pump P-03",
  current_position_note: "",
  status: "open",
  awaiting_me: false,
  item_count: 0,
  source: "human",
  created_at: "2026-03-03T00:00:00Z",
  updated_at: "2026-03-03T00:00:00Z",
  ...over,
});

const issueDetail = (over: Partial<auth.IssueListItem> = {}): auth.IssueDetail => ({
  ...issue(over),
  items: [],
});

describe("Project workspace UI", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listContacts.mockResolvedValue([]);
    listProjectIssues.mockResolvedValue([]);
    getCurrentPosition.mockResolvedValue({ facts: [], decisions: [] });
    listProjectFacts.mockResolvedValue([]);
    listProjectContradictions.mockResolvedValue([]);
    listProjectDecisions.mockResolvedValue([]);
    getProjectAttention.mockResolvedValue({
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
    getApiHealth.mockResolvedValue({ status: "ok", llm: true });
    askProject.mockResolvedValue({ answer: "", citations: [], confidence: 0 });
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

  it("creates an issue from the project page", async () => {
    createProjectIssue.mockResolvedValue(issueDetail());
    renderPage(newClient());

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    selectMode(/^Open$/i);
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

  it("creates an issue from a timeline row with that row pre-attached", async () => {
    createProjectIssue.mockResolvedValue(issueDetail());
    renderPage(newClient());

    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: /^new issue…$/i })[0]!);
    expect(await screen.findByText(/pre-attached from timeline/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", {
        title: "Teams note",
        current_position_note: undefined,
        item_refs: [{ manual_item_id: "man1" }],
      }),
    );
  });

  it("attaches a timeline item to an issue", async () => {
    listProjectIssues.mockResolvedValue([issue()]);
    addIssueItem.mockResolvedValue(issueDetail());
    renderPage(newClient());

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
    renderPage(newClient());

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

  it("shows current position, says it is derived, and creates a confirmed fact", async () => {
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
      versions: [],
    });

    renderPage(newClient());

    const position = await screen.findByLabelText(/current position/i);
    expect(position).toHaveTextContent("Pump P-03 duty");
    expect(position).toHaveTextContent("90 kW");
    // §8.4: the panel states where the position comes from, so confirming a
    // fact and the position changing is not a coincidence the operator has to
    // infer.
    expect(position).toHaveTextContent(/derived from confirmed facts and accepted decisions/i);

    selectMode(/^Position$/i);
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

  // R4 exit criterion: the words "interpret", "reconcile" and "interpretation"
  // do not appear in the UI. They are this spec family's own stage names.
  it("never names the pipeline's stages", async () => {
    listProjectFacts.mockResolvedValue([]);
    renderPage(newClient());

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    for (const mode of [/^Trail$/i, /^Position$/i, /^Open$/i]) {
      selectMode(mode);
      const text = document.body.textContent ?? "";
      expect(text).not.toMatch(/interpret/i);
      expect(text).not.toMatch(/reconcile/i);
    }
  });

  it("reports the extraction watermark and queues a run on Check now", async () => {
    getProject.mockResolvedValue({
      id: "p1",
      organisation_id: "o1",
      name: "Cooling Upgrade",
      code: "DC01",
      keywords: [],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      last_extracted_at: new Date(Date.now() - 4 * 60 * 1000).toISOString(),
    });
    extractProject.mockResolvedValue({ status: "queued", job_id: "j1", chain_id: "ch1" });

    renderPage(newClient());

    expect(await screen.findByText(/reviewed 4m ago/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /check now/i }));
    await waitFor(() => expect(extractProject).toHaveBeenCalledWith("token", "p1"));
    // A queued chain is in flight until the watermark advances.
    expect(await screen.findByText(/reviewing…/i)).toBeInTheDocument();
  });

  it("says when extraction has never run", async () => {
    renderPage(newClient());
    expect(await screen.findByText(/not reviewed yet/i)).toBeInTheDocument();
  });

  // §8.2: proposed fact versions, proposed decisions and open contradictions
  // are one queue, not three scattered across two tabs.
  it("collects every pending confirmation into one panel", async () => {
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
            value_json: 75,
            value_text: "75",
            unit: "kW",
            source: "user",
            created_at: "2026-03-01T00:00:00Z",
            evidence: [],
          },
          {
            id: "v2",
            fact_id: "f1",
            status: "proposed",
            value_json: 90,
            value_text: "90",
            unit: "kW",
            source: "llm",
            created_at: "2026-03-02T00:00:00Z",
            evidence: [
              { id: "e1", fact_version_id: "v2", message_id: "msg1", added_at: "2026-03-02T00:00:00Z" },
              { id: "e2", fact_version_id: "v2", manual_item_id: "man1", added_at: "2026-03-02T00:00:00Z" },
            ],
          },
        ],
      },
    ]);
    listProjectDecisions.mockResolvedValue([
      {
        id: "d1",
        organisation_id: "o1",
        project_id: "p1",
        statement: "Proceed with 90 kW duty",
        status: "proposed",
        source: "llm",
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        evidence: [],
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
    confirmFactVersion.mockResolvedValue({
      id: "f1",
      organisation_id: "o1",
      project_id: "p1",
      subject_key: "pump.p03.duty_kw",
      label: "Pump P-03 duty",
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      versions: [],
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

    renderPage(newClient());

    const panel = await screen.findByRole("region", { name: /needs your confirmation/i });
    // All three kinds, in one place, each saying what it would change.
    expect(panel).toHaveTextContent("Pump P-03 duty");
    expect(panel).toHaveTextContent("75 kW → 90 kW");
    expect(panel).toHaveTextContent("Proceed with 90 kW duty");
    expect(panel).toHaveTextContent(/active "75 kW" vs proposed "90 kW"/);
    // And where it came from.
    expect(panel).toHaveTextContent("from 2 messages · from the model");

    fireEvent.click(screen.getAllByRole("button", { name: /^confirm$/i })[0]!);
    await waitFor(() =>
      expect(confirmFactVersion).toHaveBeenCalledWith("token", "v2", {
        supersedes_version_id: "v1",
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: /^keep proposed$/i }));
    await waitFor(() =>
      expect(resolveContradiction).toHaveBeenCalledWith("token", "c1", {
        resolution: "supersede",
        keep_fact_version_id: "v-proposed",
      }),
    );
  });

  it("accepts a proposed decision from the confirmation panel", async () => {
    listProjectDecisions.mockResolvedValue([
      {
        id: "d1",
        organisation_id: "o1",
        project_id: "p1",
        statement: "Proceed with 90 kW duty",
        status: "proposed",
        source: "user",
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        evidence: [],
      },
    ]);
    confirmDecision.mockResolvedValue({
      id: "d1",
      organisation_id: "o1",
      project_id: "p1",
      statement: "Proceed with 90 kW duty",
      status: "accepted",
      source: "user",
      created_at: "2026-03-03T00:00:00Z",
      updated_at: "2026-03-03T00:00:00Z",
      evidence: [],
    });

    renderPage(newClient());

    const panel = await screen.findByRole("region", { name: /needs your confirmation/i });
    expect(panel).toHaveTextContent("Proceed with 90 kW duty");
    fireEvent.click(screen.getByRole("button", { name: /^accept$/i }));
    await waitFor(() => expect(confirmDecision).toHaveBeenCalledWith("token", "d1"));
  });

  // R4 exit criterion: every fact, decision and issue shows its evidence count
  // and source.
  it("shows provenance on issues and discards one", async () => {
    listProjectIssues.mockResolvedValue([
      issue({ id: "iss1", title: "Seal leak", item_count: 3, source: "llm" }),
      issue({ id: "iss2", title: "Raised by hand", item_count: 1, source: "human" }),
    ]);
    discardIssue.mockResolvedValue(issueDetail({ id: "iss1", discarded_at: "2026-03-04T00:00:00Z" }));

    renderPage(newClient());

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    selectMode(/^Open$/i);
    expect(await screen.findByText("Seal leak")).toBeInTheDocument();
    expect(screen.getByText("from 3 messages · from the model")).toBeInTheDocument();
    expect(screen.getByText("from 1 message · from a person")).toBeInTheDocument();

    fireEvent.click(screen.getAllByRole("button", { name: /^discard$/i })[0]!);
    await waitFor(() => expect(discardIssue).toHaveBeenCalledWith("token", "iss1"));
  });

  it("keeps a discarded issue out of the open list", async () => {
    listProjectIssues.mockResolvedValue([
      issue({ id: "iss1", title: "Seal leak" }),
      issue({ id: "iss2", title: "Should not have been raised", discarded_at: "2026-03-04T00:00:00Z" }),
    ]);

    renderPage(newClient());

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    selectMode(/^Open$/i);
    expect(await screen.findByText("Seal leak")).toBeInTheDocument();
    expect(screen.queryByText("Should not have been raised")).not.toBeInTheDocument();
  });

  // §8.4: empty states carry the definition rather than an apology.
  it("teaches what facts and issues are when there are none", async () => {
    renderPage(newClient());

    expect(await screen.findByText("Cooling Upgrade")).toBeInTheDocument();
    selectMode(/^Position$/i);
    expect(
      screen.getByText(/facts are values that are currently true about this project/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/decisions are choices this project has committed to/i),
    ).toBeInTheDocument();

    selectMode(/^Open$/i);
    expect(
      screen.getByText(/issues are open questions or work someone has to act on/i),
    ).toBeInTheDocument();
  });

  it("asks Project AI and shows answer with citations", async () => {
    askProject.mockResolvedValue({
      answer: "Pump P-03 duty is 90 kW",
      citations: [{ type: "fact_version", id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" }],
      confidence: 0.95,
    });
    renderPage(newClient());

    expect(await screen.findByRole("region", { name: /ask project ai/i })).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText(/ask a grounded question/i), {
      target: { value: "What is Pump P-03 duty?" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^ask$/i }));
    await waitFor(() =>
      expect(askProject).toHaveBeenCalledWith("token", "p1", "What is Pump P-03 duty?"),
    );
    expect(await screen.findByText(/Pump P-03 duty is 90 kW/i)).toBeInTheDocument();
    expect(screen.getByText(/Citations:/i)).toBeInTheDocument();
  });

  it("opens Position mode from ?mode= and keeps Trail as default", async () => {
    renderPage(newClient());
    expect(await screen.findByRole("tab", { name: /^Trail$/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tabpanel", { name: /^Trail$/i })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /^facts$/i })).not.toBeInTheDocument();

    cleanup();

    renderPage(newClient(), "/projects/p1?mode=position");
    expect(await screen.findByRole("tab", { name: /^Position$/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tabpanel", { name: /^Position$/i })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /^facts$/i })).toBeInTheDocument();
  });
});
