import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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
    listProjectTodos: vi.fn(),
    completeProjectTodo: vi.fn(),
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
    vi.mocked(auth.listProjectTodos).mockResolvedValue([]);
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

    expect(await screen.findByRole("heading", { name: "Cooling Upgrade" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^new issue$/i }));
    fireEvent.change(await screen.findByLabelText(/^title$/i), { target: { value: "Pump P-03" } });
    fireEvent.click(screen.getByRole("button", { name: /^create issue$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", {
        title: "Pump P-03",
        current_position_note: undefined,
        item_refs: undefined,
      }),
    );
  });

  function rowActions(title: string) {
    const row = screen.getByText(title).closest("li")!;
    fireEvent.click(within(row).getByRole("button", { name: /^actions$/i }));
    return row;
  }

  it("creates an issue from correspondence with that item pre-attached", async () => {
    createProjectIssue.mockResolvedValue(issueDetail());
    renderPage(newClient());

    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    const row = rowActions("Teams note");
    fireEvent.click(within(row).getByRole("button", { name: /new issue from this/i }));
    expect(await screen.findByText(/pre-attached from the correspondence/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/^title$/i)).toHaveValue("Teams note");
    fireEvent.click(screen.getByRole("button", { name: /^create issue$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", {
        title: "Teams note",
        current_position_note: undefined,
        item_refs: [{ manual_item_id: "man1" }],
      }),
    );
  });

  it("attaches correspondence to an issue from its actions", async () => {
    listProjectIssues.mockResolvedValue([issue()]);
    addIssueItem.mockResolvedValue(issueDetail());
    renderPage(newClient());

    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    // Actions stay out of the way until asked for.
    expect(screen.queryByLabelText(/attach to issue/i)).not.toBeInTheDocument();
    const row = rowActions("Teams note");
    fireEvent.change(within(row).getByLabelText(/attach to issue/i), { target: { value: "iss1" } });
    fireEvent.click(within(row).getByRole("button", { name: /^attach$/i }));
    await waitFor(() =>
      expect(addIssueItem).toHaveBeenCalledWith("token", "iss1", {
        message_id: undefined,
        manual_item_id: "man1",
      }),
    );
  });

  it("lists correspondence newest first and pastes a note", async () => {
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

    const list = await screen.findByRole("list", { name: "Correspondence" });
    const titles = within(list).getAllByRole("listitem").map((li) => li.textContent ?? "");
    expect(titles[0]).toContain("Teams note");
    expect(titles[1]).toContain("Outlook: pump");
    expect(within(list).getByRole("link", { name: "Outlook: pump" })).toHaveAttribute(
      "href",
      "/inbox?message_id=msg1&account_id=acc1",
    );

    fireEvent.click(screen.getByRole("button", { name: /paste correspondence/i }));
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: "90 kW is approved" } });
    fireEvent.click(screen.getByRole("button", { name: /add to timeline/i }));
    await waitFor(() =>
      expect(createManualItem).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({ body_text: "90 kW is approved", project_id: "p1" }),
      ),
    );
  });

  it("filters correspondence by source on the server", async () => {
    renderPage(newClient());
    expect(await screen.findByText("Teams note")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("radio", { name: "Mail" }));
    await waitFor(() =>
      expect(getProjectTimeline).toHaveBeenLastCalledWith("token", "p1", expect.objectContaining({ source: "mail" })),
    );
    fireEvent.click(screen.getByRole("switch", { name: /not on an issue/i }));
    await waitFor(() =>
      expect(getProjectTimeline).toHaveBeenLastCalledWith(
        "token",
        "p1",
        expect.objectContaining({ unassigned_to_issue: true }),
      ),
    );
  });

  const dutyFact = (): auth.FactDetail => ({
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
      {
        id: "v0",
        fact_id: "f1",
        status: "superseded",
        value_json: 75,
        value_text: "75",
        unit: "kW",
        source: "llm",
        created_at: "2026-03-01T00:00:00Z",
        evidence: [],
      },
    ],
  });

  // The position is the first thing on the page after what needs you, not a
  // tab: it is what the page is for.
  it("shows the current position up front, with where it came from and its history", async () => {
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
      decisions: [{ decision_id: "d1", statement: "Proceed with 90 kW", status: "accepted", evidence_count: 2 }],
    });
    listProjectFacts.mockResolvedValue([dutyFact()]);
    listProjectDecisions.mockResolvedValue([
      {
        id: "d1",
        organisation_id: "o1",
        project_id: "p1",
        statement: "Proceed with 90 kW",
        status: "accepted",
        source: "llm",
        created_at: "2026-03-03T00:00:00Z",
        updated_at: "2026-03-03T00:00:00Z",
        evidence: [],
      },
    ]);

    renderPage(newClient());

    const position = await screen.findByRole("region", { name: /current position/i });
    await waitFor(() => expect(position).toHaveTextContent("90 kW"));
    expect(position).toHaveTextContent("Pump P-03 duty");
    expect(position).toHaveTextContent(/derived from confirmed facts and accepted decisions/i);
    expect(position).toHaveTextContent("from 1 message · from a person");
    expect(position).toHaveTextContent("Proceed with 90 kW");
    expect(position).toHaveTextContent("from 2 messages · from the model");
    fireEvent.click(within(position).getByRole("button", { name: /1 earlier value/i }));
    expect(within(position).getByText("75 kW")).toBeInTheDocument();
  });

  it("records a new fact with an identifier derived from its name", async () => {
    listProjectFacts.mockResolvedValue([dutyFact()]);
    createProjectFact.mockResolvedValue(dutyFact());
    renderPage(newClient());

    fireEvent.click(await screen.findByRole("button", { name: /^record fact$/i }));
    fireEvent.change(await screen.findByLabelText(/^what it is$/i), { target: { value: "Pump P-03 flow" } });
    fireEvent.change(screen.getByLabelText(/^value$/i), { target: { value: "12" } });
    fireEvent.change(screen.getByLabelText(/^unit/i), { target: { value: "L/s" } });
    fireEvent.click(screen.getByRole("button", { name: /save fact/i }));
    await waitFor(() =>
      expect(createProjectFact).toHaveBeenCalledWith(
        "token",
        "p1",
        expect.objectContaining({ subject_key: "pump_p_03_flow", label: "Pump P-03 flow", value: 12, unit: "L/s", confirm: true }),
      ),
    );
  });

  it("updates an existing fact as a new version that replaces the current one", async () => {
    listProjectFacts.mockResolvedValue([dutyFact()]);
    createProjectFact.mockResolvedValue(dutyFact());
    renderPage(newClient());

    fireEvent.click(await screen.findByRole("button", { name: /^record fact$/i }));
    fireEvent.change(await screen.findByLabelText(/^fact$/i), { target: { value: "pump.p03.duty_kw" } });
    expect(screen.queryByLabelText(/^what it is$/i)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/^unit/i)).toHaveValue("kW");
    fireEvent.change(screen.getByLabelText(/^new value$/i), { target: { value: "95" } });
    fireEvent.click(screen.getByRole("button", { name: /save fact/i }));
    await waitFor(() =>
      expect(createProjectFact).toHaveBeenCalledWith(
        "token",
        "p1",
        expect.objectContaining({ subject_key: "pump.p03.duty_kw", value: 95, supersedes_version_id: "v1" }),
      ),
    );
  });

  // R4 exit criterion: the words "interpret", "reconcile" and "interpretation"
  // do not appear in the UI. They are this spec family's own stage names.
  it("never names the pipeline's stages, and has no mode tabs", async () => {
    renderPage(newClient());
    expect(await screen.findByRole("heading", { name: "Cooling Upgrade" })).toBeInTheDocument();
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/interpret/i);
    expect(text).not.toMatch(/reconcile/i);
    expect(screen.queryByRole("tab")).not.toBeInTheDocument();
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

  // A queued run that never advances the watermark must say so. Reverting to
  // the previous watermark made a wedged pipeline look idle.
  it("reports a stalled run rather than reverting to the old watermark", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const watermark = new Date(Date.now() - 4 * 60 * 1000).toISOString();
    getProject.mockResolvedValue({
      id: "p1",
      organisation_id: "o1",
      name: "Cooling Upgrade",
      code: "DC01",
      keywords: [],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      last_extracted_at: watermark,
    });
    // The endpoint answers 202 for a run already in flight, so a wedged
    // pipeline is indistinguishable from a fresh queue at this point.
    extractProject.mockResolvedValue({ status: "already_running" });

    try {
      renderPage(newClient());
      expect(await screen.findByText(/reviewed 4m ago/i)).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: /check now/i }));
      expect(await screen.findByText(/reviewing…/i)).toBeInTheDocument();

      // Past the ceiling with the watermark unmoved.
      await vi.advanceTimersByTimeAsync(2 * 60 * 1000 + 5000);
      expect(await screen.findByText(/last check didn.t finish/i)).toBeInTheDocument();
      expect(screen.queryByText(/reviewing…/i)).not.toBeInTheDocument();
      // Check now stays available so the operator can retry.
      expect(screen.getByRole("button", { name: /check now/i })).toBeEnabled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("says when extraction has never run", async () => {
    renderPage(newClient());
    expect(await screen.findByText(/not reviewed yet/i)).toBeInTheDocument();
  });

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

    const panel = await screen.findByRole("region", { name: /^needs you$/i });
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

    fireEvent.click(screen.getByRole("button", { name: /^keep new value$/i }));
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

    const panel = await screen.findByRole("region", { name: /^needs you$/i });
    expect(panel).toHaveTextContent("Proceed with 90 kW duty");
    fireEvent.click(screen.getByRole("button", { name: /^accept$/i }));
    await waitFor(() => expect(confirmDecision).toHaveBeenCalledWith("token", "d1"));
  });

  // R4 exit criterion: every fact, decision and issue shows its evidence count
  // and source.
  it("shows provenance on issues and discards one after asking", async () => {
    listProjectIssues.mockResolvedValue([
      issue({ id: "iss1", title: "Seal leak", item_count: 3, source: "llm" }),
      issue({ id: "iss2", title: "Raised by hand", item_count: 1, source: "human" }),
    ]);
    discardIssue.mockResolvedValue(issueDetail({ id: "iss1", discarded_at: "2026-03-04T00:00:00Z" }));

    renderPage(newClient());

    const issues = await screen.findByRole("region", { name: /open issues/i });
    expect(await within(issues).findByText("Seal leak")).toBeInTheDocument();
    expect(within(issues).getByText("from 3 messages · from the model")).toBeInTheDocument();
    expect(within(issues).getByText("from 1 message · from a person")).toBeInTheDocument();

    fireEvent.click(within(issues).getByRole("button", { name: "Discard Seal leak" }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /discard issue/i }));
    await waitFor(() => expect(discardIssue).toHaveBeenCalledWith("token", "iss1"));
  });

  it("keeps a discarded issue out of the open list", async () => {
    listProjectIssues.mockResolvedValue([
      issue({ id: "iss1", title: "Seal leak" }),
      issue({ id: "iss2", title: "Should not have been raised", discarded_at: "2026-03-04T00:00:00Z" }),
    ]);
    renderPage(newClient());
    expect(await screen.findByText("Seal leak")).toBeInTheDocument();
    expect(screen.queryByText("Should not have been raised")).not.toBeInTheDocument();
  });

  it("lists issues awaiting you first, and other attention in Needs you", async () => {
    listProjectIssues.mockResolvedValue([
      issue({ id: "iss1", title: "Older one", updated_at: "2026-03-01T00:00:00Z" }),
      issue({ id: "iss2", title: "Yours", awaiting_me: true, updated_at: "2026-02-01T00:00:00Z" }),
    ]);
    getProjectAttention.mockResolvedValue({
      items: [
        { id: "a1", why_me: "issue_assignee", title: "Yours", project_id: "p1", ref_type: "issue", ref_id: "iss2" },
        // The confirmation rows cover proposals; this must not be listed twice.
        { id: "a2", why_me: "provisional_fact", title: "Confirm fact", project_id: "p1", ref_type: "fact_version", ref_id: "v9" },
      ],
      counts: { total: 2, issue_assignee: 1, member_role: 0, provisional_fact: 1, provisional_decision: 0, open_contradiction: 0, mail_action_item: 0 },
    });
    renderPage(newClient());

    const issues = await screen.findByRole("region", { name: /open issues/i });
    const items = await within(issues).findAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Yours");
    expect(items[0]).toHaveTextContent("Awaiting you");

    const needs = screen.getByRole("region", { name: /^needs you$/i });
    expect(within(needs).getByRole("link", { name: /assigned to you.*yours/i })).toHaveAttribute("href", "/projects/p1/issues/iss2");
    expect(within(needs).queryByText("Confirm fact")).not.toBeInTheDocument();
  });

  it("shares the project's to-dos with everyone on it, keeping a teammate's mail private", async () => {
    vi.mocked(auth.completeProjectTodo).mockResolvedValue({ status: "done" });
    vi.mocked(auth.listProjectTodos).mockResolvedValue([
      {
        id: "t1", text: "Send the datasheet", owner_user_id: "u1", owner_label: "You", is_mine: true,
        created_at: "2026-09-20T00:00:00Z", account_id: "a1", message_id: "m1", issue_id: "iss2", issue_title: "Pump P-03 duty",
      },
      {
        id: "t2", text: "Chase the drawing", owner_user_id: "u2", owner_label: "sam@example.com", is_mine: false,
        created_at: "2026-09-21T00:00:00Z",
      },
    ]);
    // The caller's own to-do also arrives as attention; it must not be listed twice.
    getProjectAttention.mockResolvedValue({
      items: [
        { id: "mail:t1", why_me: "mail_action_item", title: "Send the datasheet", project_id: "p1", ref_type: "action_item", ref_id: "t1", account_id: "a1", message_id: "m1" },
      ],
      counts: { total: 1, issue_assignee: 0, member_role: 0, provisional_fact: 0, provisional_decision: 0, open_contradiction: 0, mail_action_item: 1 },
    });
    renderPage(newClient());

    const todos = await screen.findByRole("region", { name: /^to-dos/i });
    expect(within(todos).getByRole("link", { name: "Send the datasheet" })).toHaveAttribute("href", "/inbox?message_id=m1&account_id=a1");
    expect(within(todos).getByRole("link", { name: "On: Pump P-03 duty" })).toHaveAttribute("href", "/projects/p1/issues/iss2");
    expect(within(todos).getByText("You")).toBeInTheDocument();
    // A teammate's to-do names them and does not link to their mail.
    expect(within(todos).getByText("Chase the drawing")).toBeInTheDocument();
    expect(within(todos).queryByRole("link", { name: "Chase the drawing" })).not.toBeInTheDocument();
    expect(within(todos).getByText("sam@example.com")).toBeInTheDocument();

    const needs = screen.getByRole("region", { name: /^needs you$/i });
    expect(within(needs).queryByText("Send the datasheet")).not.toBeInTheDocument();

    fireEvent.click(within(todos).getByRole("button", { name: "Mark “Chase the drawing” done" }));
    await waitFor(() => expect(auth.completeProjectTodo).toHaveBeenCalledWith("token", "p1", "t2"));
  });

  it("leaves the to-do section out when the project has none", async () => {
    renderPage(newClient());
    await screen.findByRole("region", { name: /^needs you$/i });
    await waitFor(() => expect(auth.listProjectTodos).toHaveBeenCalled());
    expect(screen.queryByRole("region", { name: /^to-dos/i })).not.toBeInTheDocument();
  });

  it("shows an email by subject only, never its raw body", async () => {
    getProjectTimeline.mockResolvedValue([
      {
        source: "mail",
        occurred_at: "2026-03-01T10:00:00Z",
        title: "Outlook: pump",
        snippet: "<html><body><div style=\"color:red\">pump sizing</div>",
        contacts: [],
        account_id: "acc1",
        account_label: "Work",
        message_id: "msg1",
      },
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
    ]);
    renderPage(newClient());
    const list = await screen.findByRole("list", { name: "Correspondence" });
    expect(within(list).getByRole("link", { name: "Outlook: pump" })).toBeInTheDocument();
    expect(within(list).queryByText(/<html>/)).not.toBeInTheDocument();
    expect(within(list).queryByText(/pump sizing/)).not.toBeInTheDocument();
    expect(within(list).getByText("Consider 90 kW")).toBeInTheDocument();
  });

  it("says plainly when nothing needs you", async () => {
    renderPage(newClient());
    const needs = await screen.findByRole("region", { name: /^needs you$/i });
    expect(await within(needs).findByText(/nothing is waiting on you/i)).toBeInTheDocument();
  });

  // §8.4: empty states carry the definition rather than an apology.
  it("teaches what facts, decisions and issues are when there are none", async () => {
    renderPage(newClient());
    expect(await screen.findByText(/facts are values that are currently true about this project/i)).toBeInTheDocument();
    expect(screen.getByText(/decisions are choices this project has committed to/i)).toBeInTheDocument();
    expect(await screen.findByText(/issues are open questions or work someone has to act on/i)).toBeInTheDocument();
  });

  it("answers questions about the project, saying how many sources it used", async () => {
    askProject.mockResolvedValue({
      answer: "Pump P-03 duty is 90 kW",
      citations: [{ type: "fact_version", id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" }],
      confidence: 0.95,
    });
    renderPage(newClient());

    const ask = await screen.findByRole("region", { name: /ask this project/i });
    await waitFor(() => expect(within(ask).getByLabelText("Question")).toBeEnabled());
    fireEvent.change(within(ask).getByLabelText("Question"), { target: { value: "What is Pump P-03 duty?" } });
    fireEvent.click(within(ask).getByRole("button", { name: /^ask$/i }));
    await waitFor(() => expect(askProject).toHaveBeenCalledWith("token", "p1", "What is Pump P-03 duty?"));
    expect(await within(ask).findByText(/Pump P-03 duty is 90 kW/i)).toBeInTheDocument();
    expect(within(ask).getByText(/based on 1 source/i)).toBeInTheDocument();
  });

  it("edits project details including client and description", async () => {
    vi.mocked(auth.updateProject).mockResolvedValue({} as auth.ProjectListItem);
    vi.mocked(auth.updateProjectMember).mockResolvedValue({} as auth.ProjectMember);
    renderPage(newClient());

    fireEvent.click(await screen.findByRole("button", { name: /edit details/i }));
    fireEvent.change(await screen.findByLabelText(/^client$/i), { target: { value: "Acme" } });
    fireEvent.change(screen.getByLabelText(/^keywords$/i), { target: { value: "chiller, P-03, chiller" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(auth.updateProject).toHaveBeenCalledWith(
        "token",
        "p1",
        expect.objectContaining({ name: "Cooling Upgrade", client: "Acme", keywords: ["chiller", "P-03"] }),
      ),
    );
    expect(auth.updateProjectMember).toHaveBeenCalledWith("token", "p1", expect.objectContaining({ role: "ME" }));
  });

  it("archives only after asking", async () => {
    vi.mocked(auth.updateProject).mockResolvedValue({} as auth.ProjectListItem);
    renderPage(newClient());
    fireEvent.click(await screen.findByRole("button", { name: /^archive$/i }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(auth.updateProject).not.toHaveBeenCalled();
  });

  // Home and activity link to sections; old ?mode= links still land.
  it("scrolls to the section a link names", async () => {
    const scrolled: string[] = [];
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = function (this: Element) {
      scrolled.push(this.id);
    };
    try {
      renderPage(newClient(), "/projects/p1#issues");
      await waitFor(() => expect(scrolled).toContain("issues"));
      cleanup();
      renderPage(newClient(), "/projects/p1?mode=position");
      await waitFor(() => expect(scrolled).toContain("position"));
    } finally {
      Element.prototype.scrollIntoView = original;
    }
  });
});
