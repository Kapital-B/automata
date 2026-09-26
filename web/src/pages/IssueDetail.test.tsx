import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import IssueDetailPage from "@/pages/IssueDetail";
import * as auth from "@/lib/auth";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token", user: { userId: "u1" } }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    getIssue: vi.fn(),
    updateIssue: vi.fn(),
    listContacts: vi.fn(),
    markActionItemDone: vi.fn(),
    removeIssueItem: vi.fn(),
  };
});

const getIssue = vi.mocked(auth.getIssue);
const updateIssue = vi.mocked(auth.updateIssue);
const markActionItemDone = vi.mocked(auth.markActionItemDone);

// Radix Select reaches for pointer capture and scrolling, which jsdom lacks.
beforeAll(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => {};
  Element.prototype.scrollIntoView ??= () => {};
});

function issue(overrides: Partial<auth.IssueDetail> = {}): auth.IssueDetail {
  return {
    id: "iss1",
    organisation_id: "o1",
    project_id: "p1",
    title: "Pump P-03 duty",
    current_position_note: "",
    status: "open",
    assignee_user_id: "u1",
    awaiting_me: true,
    item_count: 1,
    source: "llm",
    created_at: "2026-09-20T00:00:00Z",
    updated_at: "2026-09-20T00:00:00Z",
    items: [],
    todos: [
      { id: "t1", text: "Send the datasheet", account_id: "a1", message_id: "m1", created_at: "2026-09-21T00:00:00Z" },
    ],
    ...overrides,
  } as auth.IssueDetail;
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/projects/p1/issues/iss1"]}>
        <Routes>
          <Route path="/projects/:id/issues/:issueId" element={<IssueDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function chooseStatus(name: string) {
  fireEvent.keyDown(screen.getByRole("combobox", { name: "Status" }), { key: "ArrowDown" });
  fireEvent.click(screen.getByRole("option", { name }));
}

describe("IssueDetailPage to-dos", () => {
  beforeEach(() => {
    getIssue.mockReset();
    updateIssue.mockReset();
    markActionItemDone.mockReset();
    vi.mocked(auth.listContacts).mockResolvedValue([]);
    getIssue.mockResolvedValue(issue());
    updateIssue.mockResolvedValue(issue({ status: "resolved", todos: [] }));
    markActionItemDone.mockResolvedValue({ status: "ok" });
  });

  it("lists your to-dos from the trail and marks one done", async () => {
    renderPage();
    const list = await screen.findByRole("list", { name: "Your to-dos" });
    expect(within(list).getByRole("link", { name: "Send the datasheet" })).toHaveAttribute(
      "href",
      "/inbox?message_id=m1&account_id=a1",
    );
    fireEvent.click(within(list).getByRole("button", { name: "Mark “Send the datasheet” done" }));
    await waitFor(() => expect(markActionItemDone).toHaveBeenCalledWith("token", "t1"));
  });

  it("hides the section when there are no to-dos", async () => {
    getIssue.mockResolvedValue(issue({ todos: [] }));
    renderPage();
    await screen.findByRole("heading", { name: "Pump P-03 duty" });
    expect(screen.queryByRole("heading", { name: "Your to-dos" })).not.toBeInTheDocument();
  });

  it("asks before resolving with open to-dos, and can close them in the same step", async () => {
    renderPage();
    await screen.findByRole("list", { name: "Your to-dos" });
    chooseStatus("Resolved");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    const dialog = await screen.findByRole("alertdialog", { name: "Resolve this issue?" });
    expect(dialog).toHaveTextContent("1 open to-do");
    expect(updateIssue).not.toHaveBeenCalled();
    fireEvent.click(within(dialog).getByRole("button", { name: "Resolve and mark done" }));
    await waitFor(() =>
      expect(updateIssue).toHaveBeenCalledWith(
        "token",
        "iss1",
        expect.objectContaining({ status: "resolved", complete_todos: true }),
      ),
    );
  });

  it("can resolve without touching the to-dos", async () => {
    renderPage();
    await screen.findByRole("list", { name: "Your to-dos" });
    chooseStatus("Resolved");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click(await screen.findByRole("button", { name: "Resolve only" }));
    await waitFor(() => expect(updateIssue).toHaveBeenCalled());
    expect(updateIssue.mock.calls[0]?.[2]).not.toHaveProperty("complete_todos");
  });

  it("saves other changes without asking", async () => {
    renderPage();
    await screen.findByRole("list", { name: "Your to-dos" });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateIssue).toHaveBeenCalled());
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  });
});
