import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsPage from "@/pages/Projects";
import TriagePage from "@/pages/Triage";
import * as auth from "@/lib/auth";

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

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
    listProjects: vi.fn(),
    createProject: vi.fn(),
    listUnassigned: vi.fn(),
    assignMessageProject: vi.fn(),
  };
});

const listProjects = vi.mocked(auth.listProjects);
const createProject = vi.mocked(auth.createProject);
const listUnassigned = vi.mocked(auth.listUnassigned);
const assignMessageProject = vi.mocked(auth.assignMessageProject);

function wrap(ui: React.ReactNode, path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/projects" element={<ProjectsPage />} />
          <Route path="/triage" element={<TriagePage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Projects UI", () => {
  beforeEach(() => {
    listProjects.mockReset();
    createProject.mockReset();
    listUnassigned.mockReset();
    assignMessageProject.mockReset();
  });

  it("renders projects and creates one", async () => {
    listProjects.mockResolvedValue([
      {
        id: "p1",
        organisation_id: "o1",
        name: "Cooling",
        code: "DC01",
        keywords: [],
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]);
    createProject.mockResolvedValue({
      id: "p2",
      organisation_id: "o1",
      name: "New",
      code: "NW01",
      keywords: [],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    });
    wrap(<ProjectsPage />, "/projects");
    expect(await screen.findByText("Cooling")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /new project/i }));
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: "New" } });
    fireEvent.change(screen.getByLabelText(/^code$/i), { target: { value: "NW01" } });
    fireEvent.click(screen.getByRole("button", { name: /^create project$/i }));
    await waitFor(() => expect(createProject).toHaveBeenCalled());
  });

  const project = (over: Partial<import("@/lib/auth").ProjectListItem>) => ({
    id: "p",
    organisation_id: "o1",
    name: "Project",
    code: "PR01",
    keywords: [] as string[],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  });

  it("searches by name, code, client or keyword, with a count", async () => {
    listProjects.mockResolvedValue([
      project({ id: "p1", name: "Cooling", code: "DC01", keywords: ["chiller"] }),
      project({ id: "p2", name: "Office move", code: "OF02", client: "Acme" }),
    ]);
    wrap(<ProjectsPage />, "/projects");
    expect(await screen.findByText("2 projects")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search projects"), { target: { value: "chill" } });
    expect(screen.getByText("Cooling")).toBeInTheDocument();
    expect(screen.queryByText("Office move")).not.toBeInTheDocument();
    expect(screen.getByText("1 of 2")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Search projects"), { target: { value: "acme" } });
    expect(screen.getByText("Office move")).toBeInTheDocument();
  });

  // The code rule used to surface only as a toast after submitting.
  it("explains a bad code as it is typed, and sends keywords once each", async () => {
    listProjects.mockResolvedValue([]);
    createProject.mockResolvedValue(project({ id: "p9", code: "DC02" }));
    wrap(<ProjectsPage />, "/projects");
    fireEvent.click((await screen.findAllByRole("button", { name: /new project/i }))[0]);
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: "Cooling phase 2" } });
    fireEvent.change(screen.getByLabelText(/^code$/i), { target: { value: "2DC" } });
    expect(screen.getByText("Start with a letter.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^create project$/i })).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/^code$/i), { target: { value: "dc02" } });
    fireEvent.change(screen.getByLabelText(/keywords/i), { target: { value: "chiller, P-03, chiller" } });
    expect(within(screen.getByRole("list", { name: "Keywords" })).getAllByRole("listitem")).toHaveLength(2);
    fireEvent.click(screen.getByRole("button", { name: /^create project$/i }));
    await waitFor(() =>
      expect(createProject).toHaveBeenCalledWith("token", { name: "Cooling phase 2", code: "DC02", keywords: ["chiller", "P-03"] }),
    );
  });

  it("shows archived projects only when asked, marked as archived", async () => {
    listProjects.mockImplementation(async (_t, includeArchived) =>
      includeArchived
        ? [project({ id: "p1", name: "Cooling" }), project({ id: "p3", name: "Old site", code: "OS01", archived_at: "2026-02-01T00:00:00Z" })]
        : [project({ id: "p1", name: "Cooling" })],
    );
    wrap(<ProjectsPage />, "/projects");
    expect(await screen.findByText("Cooling")).toBeInTheDocument();
    expect(screen.queryByText("Old site")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("switch", { name: /show archived/i }));
    expect(await screen.findByText("Old site")).toBeInTheDocument();
    expect(screen.getByText("Archived")).toBeInTheDocument();
    expect(listProjects).toHaveBeenLastCalledWith("token", true);
  });

  it("triage sections render and assign hits API", async () => {
    listProjects.mockResolvedValue([
      {
        id: "p1",
        organisation_id: "o1",
        name: "Cooling",
        code: "DC01",
        keywords: [],
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]);
    listUnassigned.mockResolvedValue([
      {
        kind: "message",
        message_id: "m1",
        account_id: "acc1",
        account_label: "Work",
        subject: "Needs a home",
        conversation_id: "c1",
        received_at: "2026-01-01T00:00:00Z",
        status: "unassigned",
      },
      {
        kind: "message",
        message_id: "m2",
        account_id: "acc1",
        account_label: "Work",
        subject: "Maybe cooling",
        conversation_id: "c2",
        received_at: "2026-01-01T00:00:00Z",
        status: "provisional",
        reason: "name_or_keyword:DC01",
        project_id: "p1",
      },
      {
        kind: "manual",
        manual_item_id: "man1",
        channel: "teams",
        title: "Pasted Teams note",
        occurred_at: "2026-01-02T00:00:00Z",
        status: "unassigned",
      },
    ]);
    assignMessageProject.mockResolvedValue({ status: "committed", project_id: "p1" });
    wrap(<TriagePage />, "/triage");
    expect(await screen.findByRole("heading", { name: "Triage" })).toBeInTheDocument();
    expect(await screen.findByText("Needs a home")).toBeInTheDocument();
    expect(screen.getByText("Maybe cooling")).toBeInTheDocument();
    expect(screen.getByText("Pasted Teams note")).toBeInTheDocument();
    expect(screen.getByText(/Needs confirmation/i)).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /assign thread/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: /this message only/i }).length).toBeGreaterThan(0);
  });

  it("shows a clear empty state when triage has nothing to file", async () => {
    listProjects.mockResolvedValue([]);
    listUnassigned.mockResolvedValue([]);
    wrap(<TriagePage />, "/triage");
    expect(await screen.findByText(/Triage is clear/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Projects$/i })).toHaveAttribute("href", "/projects");
    expect(screen.getByRole("link", { name: /Back to Home/i })).toHaveAttribute("href", "/");
  });
});
