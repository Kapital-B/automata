import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RunsPage from "@/pages/Runs";
import * as auth from "@/lib/auth";

const toastMock = vi.fn();

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({
    accounts: [
      {
        id: "acc-1",
        label: "Work",
        primaryEmail: "work@example.com",
        kind: "work" as const,
        status: "connected" as const,
        colorVar: "acct-1" as const,
      },
    ],
  }),
}));

vi.mock("@/hooks/use-toast", () => ({
  toast: (...args: unknown[]) => toastMock(...args),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listRuns: vi.fn(),
    cancelRun: vi.fn(),
  };
});

const listRuns = vi.mocked(auth.listRuns);
const cancelRun = vi.mocked(auth.cancelRun);

function renderPage(accountFilter: "all" | string = "all") {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <RunsPage accountFilter={accountFilter} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("RunsPage", () => {
  beforeEach(() => {
    toastMock.mockReset();
    listRuns.mockReset();
    cancelRun.mockReset();
  });

  it("loads more runs with a cursor and cancels active runs", async () => {
    listRuns.mockImplementation(async (_token, filter = {}) => {
      if (filter.cursor === "cursor-2") {
        return {
          runs: [
            {
              id: "run-3",
              account_id: "acc-1",
              job_type: "summarize",
              trigger: "schedule",
              status: "success",
              started_at: "2026-08-29T20:59:00Z",
              finished_at: "2026-08-29T21:00:00Z",
              meta_json: { drafts_generated: 2 },
            },
          ],
        };
      }

      return {
        runs: [
          {
            id: "run-1",
            account_id: "acc-1",
            job_type: "sync",
            trigger: "api",
            status: "running",
            started_at: null,
            finished_at: null,
            meta_json: { processed_messages: 1, total_messages: 2 },
          },
          {
            id: "run-2",
            account_id: "acc-1",
            job_type: "categorize",
            trigger: "schedule",
            status: "success",
            started_at: "2026-08-29T20:50:00Z",
            finished_at: "2026-08-29T20:55:00Z",
            meta_json: { drafts_generated: 4, action_items_seen: 7 },
          },
          {
            id: "run-4",
            account_id: "acc-1",
            job_type: "forward_rules",
            trigger: "api",
            status: "success",
            started_at: "2026-08-29T20:40:00Z",
            finished_at: "2026-08-29T20:40:05Z",
            meta_json: {},
          },
        ],
        nextCursor: "cursor-2",
      };
    });
    cancelRun.mockResolvedValue(undefined);

    renderPage();

    expect(await screen.findByRole("heading", { name: "Job runs" })).toBeInTheDocument();
    expect(await screen.findByText("1/2 processed")).toBeInTheDocument();
    expect(screen.getByText("4 drafts generated · 7 seen")).toBeInTheDocument();
    // A run that recorded nothing says so, instead of a dash.
    expect(screen.getByText("Nothing to do")).toBeInTheDocument();
    // Jobs read as words, not internal identifiers.
    expect(within(screen.getByRole("list", { name: "Runs" })).getByText("Run forwarding rules")).toBeInTheDocument();
    expect(screen.getByText("took 5s")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));

    await waitFor(() =>
      expect(listRuns).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          cursor: "cursor-2",
          limit: 50,
        }),
      ),
    );
    expect(await within(screen.getByRole("list", { name: "Runs" })).findByText("Summarise")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(cancelRun).toHaveBeenCalledWith("token", "run-1"));
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith(expect.objectContaining({ title: "Cancel requested" })),
    );
  });

  it("passes the selected account filter to the runs query", async () => {
    listRuns.mockResolvedValue({
      runs: [],
    });

    renderPage("acc-1");

    await waitFor(() =>
      expect(listRuns).toHaveBeenCalledWith(
        "token",
        expect.objectContaining({
          accountId: "acc-1",
          limit: 50,
        }),
      ),
    );
  });

  // A failed run used to show only the word "failed"; the reason was never rendered.
  it("shows why a run failed, and its details on request", async () => {
    listRuns.mockResolvedValue({
      runs: [
        {
          id: "run-9",
          account_id: "acc-1",
          job_type: "sync",
          trigger: "schedule",
          status: "failed",
          started_at: "2026-08-29T20:00:00Z",
          finished_at: "2026-08-29T20:00:30Z",
          error_message: "mailbox credentials rejected: reconnect the account",
          meta_json: { forwarded: 3, skipped: 10, failed: 0 },
        },
      ],
    });
    renderPage();
    expect(await screen.findByText(/mailbox credentials rejected/)).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("Scheduled")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /details/i }));
    const details = document.getElementById("run-details-run-9")!;
    expect(within(details).getByText("Forwarded")).toBeInTheDocument();
    expect(within(details).getByText("run-9")).toBeInTheDocument();
  });

  it("filters by job type on the server", async () => {
    listRuns.mockResolvedValue({ runs: [] });
    renderPage();
    fireEvent.change(await screen.findByLabelText("Job type"), { target: { value: "summarize" } });
    await waitFor(() =>
      expect(listRuns).toHaveBeenCalledWith("token", expect.objectContaining({ jobType: "summarize" })),
    );
    expect(await screen.findByText("No summarise runs yet")).toBeInTheDocument();
  });
});
