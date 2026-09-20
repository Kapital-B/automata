import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { Toaster } from "@/components/ui/toaster";
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
      { id: "acc1", label: "Work", primaryEmail: "w@ex.com", colorVar: "acct-1", status: "connected" },
    ],
  }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listProjects: vi.fn(),
    listUnassigned: vi.fn(),
    getUnassignedSummary: vi.fn(),
    assignProjectsBatch: vi.fn(),
    rescanUnassigned: vi.fn(),
  };
});

const listProjects = vi.mocked(auth.listProjects);
const listUnassigned = vi.mocked(auth.listUnassigned);
const getUnassignedSummary = vi.mocked(auth.getUnassignedSummary);
const assignProjectsBatch = vi.mocked(auth.assignProjectsBatch);
const rescanUnassigned = vi.mocked(auth.rescanUnassigned);

function project(id: string, code: string, name: string) {
  return {
    id,
    organisation_id: "o1",
    name,
    code,
    keywords: [],
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function mailItem(over: Partial<auth.UnassignedItem> = {}): auth.UnassignedItem {
  return {
    kind: "message",
    message_id: "m1",
    account_id: "acc1",
    account_label: "Work",
    subject: "Needs a home",
    conversation_id: "c1",
    received_at: "2026-01-01T00:00:00Z",
    status: "unassigned",
    thread_count: 1,
    ...over,
  };
}

function wrap() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/triage"]}>
        <Routes>
          <Route path="/triage" element={<TriagePage />} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

describe("Triage", () => {
  beforeEach(() => {
    listProjects.mockReset();
    listUnassigned.mockReset();
    getUnassignedSummary.mockReset();
    getUnassignedSummary.mockResolvedValue({ unassigned: 0, provisional: 0, not_relevant: 0 });
    assignProjectsBatch.mockReset();
    rescanUnassigned.mockReset();
    listProjects.mockResolvedValue([project("p1", "DC01", "Cooling"), project("p2", "OT02", "Other")]);
    assignProjectsBatch.mockResolvedValue({
      results: [{ id: "m1", ok: true }],
      assigned: 1,
      failed: 0,
    });
  });

  it("offers a one-click confirm for a suggested project", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({
        message_id: "m1",
        subject: "Maybe cooling",
        status: "provisional",
        project_id: "p1",
        reason: "name_or_keyword:DC01",
        confidence: 0.62,
        source: "rule",
      }),
    ]);
    wrap();

    // The suggestion is surfaced, not left for the operator to rediscover.
    const confirm = await screen.findByRole("button", { name: /confirm → DC01 · Cooling/i });
    fireEvent.click(confirm);

    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch).toHaveBeenCalledWith("token", [
      { kind: "message", id: "m1", project_id: "p1", scope: "thread" },
    ]);
  });

  it("explains why a suggestion was made", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({
        status: "provisional",
        project_id: "p1",
        reason: "code:DC01+sender_domain:acme.com",
        confidence: 0.97,
        source: "llm",
      }),
    ]);
    wrap();
    expect(await screen.findByText(/mentions DC01 · Cooling/i)).toBeInTheDocument();
    expect(screen.getByText(/sender at acme\.com files here/i)).toBeInTheDocument();
    expect(screen.getByText(/97% confident/i)).toBeInTheDocument();
    expect(screen.getByText("llm")).toBeInTheDocument();
  });

  it("shows thread rows as one decision with a message count", async () => {
    listUnassigned.mockResolvedValue([mailItem({ thread_count: 20 })]);
    wrap();
    expect(await screen.findByText("20 messages")).toBeInTheDocument();
  });

  it("assigns a multi-row selection in a single request", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "One", conversation_id: "c1" }),
      mailItem({ message_id: "m2", subject: "Two", conversation_id: "c2" }),
      mailItem({ message_id: "m3", subject: "Three", conversation_id: "c3" }),
    ]);
    assignProjectsBatch.mockResolvedValue({
      results: [
        { id: "m1", ok: true },
        { id: "m2", ok: true },
        { id: "m3", ok: true },
      ],
      assigned: 3,
      failed: 0,
    });
    wrap();
    await screen.findByText("One");

    fireEvent.click(screen.getByRole("checkbox", { name: /select all in this group/i }));
    expect(await screen.findByText("3 selected")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText(/project for selected items/i));
    fireEvent.click(await screen.findByRole("option", { name: /DC01 — Cooling/i }));

    const bar = screen.getByRole("region", { name: /bulk assignment/i });
    fireEvent.click(within(bar).getByRole("button", { name: /^assign thread$/i }));

    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    const [, batch] = assignProjectsBatch.mock.calls[0];
    expect(batch).toHaveLength(3);
    expect(batch.map((b) => b.id).sort()).toEqual(["m1", "m2", "m3"]);
    expect(batch.every((b) => b.project_id === "p1")).toBe(true);
  });

  it("confirms every suggestion for a project in one request", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "A", status: "provisional", project_id: "p1" }),
      mailItem({ message_id: "m2", subject: "B", status: "provisional", project_id: "p1" }),
      mailItem({ message_id: "m3", subject: "C", status: "provisional", project_id: "p2" }),
    ]);
    assignProjectsBatch.mockResolvedValue({
      results: [
        { id: "m1", ok: true },
        { id: "m2", ok: true },
      ],
      assigned: 2,
      failed: 0,
    });
    wrap();

    fireEvent.click(await screen.findByRole("button", { name: /confirm all 2/i }));
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    const [, batch] = assignProjectsBatch.mock.calls[0];
    expect(batch.map((b) => b.id).sort()).toEqual(["m1", "m2"]);
    expect(batch.every((b) => b.project_id === "p1")).toBe(true);
  });

  it("reports a partial failure instead of claiming success", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "One", conversation_id: "c1" }),
      mailItem({ message_id: "m2", subject: "Two", conversation_id: undefined }),
    ]);
    assignProjectsBatch.mockResolvedValue({
      results: [
        { id: "m1", ok: true },
        { id: "m2", ok: false, error: "conversation_required" },
      ],
      assigned: 1,
      failed: 1,
    });
    wrap();
    await screen.findByText("One");

    fireEvent.click(screen.getByRole("checkbox", { name: /select all in this group/i }));
    fireEvent.click(screen.getByLabelText(/project for selected items/i));
    fireEvent.click(await screen.findByRole("option", { name: /DC01 — Cooling/i }));
    const bar = screen.getByRole("region", { name: /bulk assignment/i });
    fireEvent.click(within(bar).getByRole("button", { name: /^assign thread$/i }));

    expect(await screen.findByText(/1 assigned, 1 failed/i)).toBeInTheDocument();
    expect(screen.getByText(/conversation required/i)).toBeInTheDocument();
  });

  it("confirms the focused row from the keyboard", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "One", status: "provisional", project_id: "p1" }),
    ]);
    wrap();
    await screen.findByText("One");

    fireEvent.keyDown(window, { key: "Enter" });
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch.mock.calls[0][1][0]).toMatchObject({ id: "m1", project_id: "p1" });
  });

  it("files the focused row to a numbered project", async () => {
    listUnassigned.mockResolvedValue([mailItem({ message_id: "m1", subject: "One" })]);
    wrap();
    await screen.findByText("One");

    fireEvent.keyDown(window, { key: "2" });
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch.mock.calls[0][1][0]).toMatchObject({ id: "m1", project_id: "p2" });
  });

  it("undoes the last batch by clearing what it assigned", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "One", status: "provisional", project_id: "p1" }),
    ]);
    wrap();
    await screen.findByText("One");

    fireEvent.keyDown(window, { key: "Enter" });
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));

    fireEvent.keyDown(window, { key: "u" });
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(2));
    expect(assignProjectsBatch.mock.calls[1][1]).toEqual([
      { kind: "message", id: "m1", project_id: null, scope: "thread" },
    ]);
  });

  it("triggers a rescan", async () => {
    listUnassigned.mockResolvedValue([mailItem()]);
    rescanUnassigned.mockResolvedValue({ run_ids: ["r1"] });
    wrap();
    fireEvent.click(await screen.findByRole("button", { name: /rescan suggestions/i }));
    await waitFor(() => expect(rescanUnassigned).toHaveBeenCalledWith("token"));
  });

  it("keeps an empty queue reassuring", async () => {
    listUnassigned.mockResolvedValue([]);
    wrap();
    expect(await screen.findByText(/Triage is clear/i)).toBeInTheDocument();
  });
});


describe("Triage — not project-related", () => {
  beforeEach(() => {
    listProjects.mockReset();
    listUnassigned.mockReset();
    assignProjectsBatch.mockReset();
    rescanUnassigned.mockReset();
    getUnassignedSummary.mockReset();
    listProjects.mockResolvedValue([project("p1", "DC01", "Cooling")]);
    assignProjectsBatch.mockResolvedValue({
      results: [{ id: "m1", ok: true }],
      assigned: 1,
      failed: 0,
    });
    getUnassignedSummary.mockResolvedValue({ unassigned: 1, provisional: 0, not_relevant: 0 });
  });

  it("marks a row as not project-related without a project", async () => {
    listUnassigned.mockResolvedValue([mailItem({ message_id: "m1", subject: "Newsletter" })]);
    wrap();
    await screen.findByText("Newsletter");

    fireEvent.click(screen.getAllByRole("button", { name: /^not project-related$/i })[0]);
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch.mock.calls[0][1]).toEqual([
      { kind: "message", id: "m1", project_id: null, not_relevant: true, scope: "thread" },
    ]);
  });

  it("marks the whole selection at once", async () => {
    listUnassigned.mockResolvedValue([
      mailItem({ message_id: "m1", subject: "One", conversation_id: "c1" }),
      mailItem({ message_id: "m2", subject: "Two", conversation_id: "c2" }),
    ]);
    assignProjectsBatch.mockResolvedValue({
      results: [
        { id: "m1", ok: true },
        { id: "m2", ok: true },
      ],
      assigned: 2,
      failed: 0,
    });
    wrap();
    await screen.findByText("One");

    fireEvent.click(screen.getByRole("checkbox", { name: /select all in this group/i }));
    const bar = await screen.findByRole("region", { name: /bulk assignment/i });
    fireEvent.click(within(bar).getByRole("button", { name: /^not project-related$/i }));

    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    const [, batch] = assignProjectsBatch.mock.calls[0];
    expect(batch).toHaveLength(2);
    expect(batch.every((b) => b.not_relevant === true && b.project_id === null)).toBe(true);
  });

  it("marks the focused row from the keyboard", async () => {
    listUnassigned.mockResolvedValue([mailItem({ message_id: "m1", subject: "Spam" })]);
    wrap();
    await screen.findByText("Spam");

    fireEvent.keyDown(window, { key: "x" });
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch.mock.calls[0][1][0]).toMatchObject({ not_relevant: true });
  });

  it("opens the dismissed list from the chip and restores from it", async () => {
    getUnassignedSummary.mockResolvedValue({ unassigned: 0, provisional: 0, not_relevant: 2 });
    listUnassigned.mockImplementation(async (_t, opts) => {
      if (opts?.status === "not_relevant") {
        return [
          mailItem({
            message_id: "m9",
            subject: "Vendor blast",
            not_relevant_at: "2026-09-20T10:00:00Z",
          }),
        ];
      }
      return [];
    });
    wrap();

    const chip = await screen.findByRole("button", { name: /not project-related \(2\)/i });
    fireEvent.click(chip);

    expect(await screen.findByText("Vendor blast")).toBeInTheDocument();
    expect(screen.getByText(/Marked not project-related/i)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /restore to queue/i }));
    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    expect(assignProjectsBatch.mock.calls[0][1]).toEqual([
      { kind: "message", id: "m9", project_id: null, not_relevant: false, scope: "thread" },
    ]);
  });

  it("assigns a project directly from the dismissed list", async () => {
    getUnassignedSummary.mockResolvedValue({ unassigned: 0, provisional: 0, not_relevant: 1 });
    listUnassigned.mockImplementation(async (_t, opts) =>
      opts?.status === "not_relevant"
        ? [mailItem({ message_id: "m9", subject: "Turns out relevant", conversation_id: "c9" })]
        : [],
    );
    assignProjectsBatch.mockResolvedValue({
      results: [{ id: "m9", ok: true }],
      assigned: 1,
      failed: 0,
    });
    wrap();

    fireEvent.click(await screen.findByRole("button", { name: /not project-related \(1\)/i }));
    await screen.findByText("Turns out relevant");

    fireEvent.click(screen.getByLabelText(/project for Turns out relevant/i));
    fireEvent.click(await screen.findByRole("option", { name: /DC01 — Cooling/i }));
    fireEvent.click(screen.getByRole("button", { name: /^assign thread$/i }));

    await waitFor(() => expect(assignProjectsBatch).toHaveBeenCalledTimes(1));
    // Assigning does not carry the dismissal flag; the backend clears it.
    expect(assignProjectsBatch.mock.calls[0][1][0]).toMatchObject({
      id: "m9",
      project_id: "p1",
    });
  });

  it("hides the chip when nothing has been dismissed", async () => {
    getUnassignedSummary.mockResolvedValue({ unassigned: 1, provisional: 0, not_relevant: 0 });
    listUnassigned.mockResolvedValue([mailItem()]);
    wrap();
    await screen.findByText("Needs a home");
    expect(screen.queryByRole("button", { name: /not project-related \(/i })).not.toBeInTheDocument();
  });
});
