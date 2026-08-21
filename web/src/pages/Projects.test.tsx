import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import ProjectsPage from "@/pages/Projects";
import UnassignedPage from "@/pages/Unassigned";
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
          <Route path="/unassigned" element={<UnassignedPage />} />
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
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));
    await waitFor(() => expect(createProject).toHaveBeenCalled());
  });

  it("unassigned sections render and assign hits API", async () => {
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
    ]);
    assignMessageProject.mockResolvedValue({ status: "committed", project_id: "p1" });
    wrap(<UnassignedPage />, "/unassigned");
    expect(await screen.findByText("Needs a home")).toBeInTheDocument();
    expect(screen.getByText("Maybe cooling")).toBeInTheDocument();
    expect(screen.getByText(/Needs confirmation/i)).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /assign thread/i }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: /this message only/i }).length).toBeGreaterThan(0);
  });
});
