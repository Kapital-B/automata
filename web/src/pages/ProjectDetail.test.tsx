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
    addIssueItem: vi.fn(),
    updateProject: vi.fn(),
    updateProjectMember: vi.fn(),
  };
});

const getProject = vi.mocked(auth.getProject);
const getProjectTimeline = vi.mocked(auth.getProjectTimeline);
const createManualItem = vi.mocked(auth.createManualItem);
const listContacts = vi.mocked(auth.listContacts);
const listProjectIssues = vi.mocked(auth.listProjectIssues);
const createProjectIssue = vi.mocked(auth.createProjectIssue);
const addIssueItem = vi.mocked(auth.addIssueItem);

describe("Project timeline UI", () => {
  beforeEach(() => {
    getProject.mockReset();
    getProjectTimeline.mockReset();
    createManualItem.mockReset();
    listContacts.mockReset();
    listProjectIssues.mockReset();
    createProjectIssue.mockReset();
    addIssueItem.mockReset();
    listContacts.mockResolvedValue([]);
    listProjectIssues.mockResolvedValue([]);
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
    fireEvent.click(screen.getByRole("button", { name: /new issue/i }));
    fireEvent.change(screen.getByPlaceholderText(/pump p-03/i), {
      target: { value: "Pump P-03" },
    });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));
    await waitFor(() =>
      expect(createProjectIssue).toHaveBeenCalledWith("token", "p1", { title: "Pump P-03" }),
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
});
