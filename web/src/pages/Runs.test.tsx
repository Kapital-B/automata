import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
      <RunsPage accountFilter={accountFilter} />
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
        ],
        nextCursor: "cursor-2",
      };
    });
    cancelRun.mockResolvedValue(undefined);

    renderPage();

    expect(await screen.findByRole("heading", { name: "Job runs" })).toBeInTheDocument();
    expect(await screen.findByText("1/2 processed")).toBeInTheDocument();
    expect(screen.getByText("4 drafts generated · 7 seen")).toBeInTheDocument();
    expect(screen.getByText("—")).toBeInTheDocument();

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
    expect(await screen.findByText("summarize")).toBeInTheDocument();

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
});
