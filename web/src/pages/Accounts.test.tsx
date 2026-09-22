import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccountsPage from "@/pages/Accounts";
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
        kind: "work",
        lastSyncedAt: "2026-09-21T10:00:00Z",
      },
    ],
    isLoading: false,
    isError: false,
  }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    syncAccount: vi.fn(),
    listConnectors: vi.fn(),
    listProjects: vi.fn(),
  };
});

const syncAccount = vi.mocked(auth.syncAccount);
const listConnectors = vi.mocked(auth.listConnectors);
const listProjects = vi.mocked(auth.listProjects);

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <AccountsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("Accounts sync controls", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listConnectors.mockResolvedValue([]);
    listProjects.mockResolvedValue([]);
    syncAccount.mockResolvedValue({ job_run_id: "run-1234abcd", status: "queued" });
  });

  it("syncs normally without forcing", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /sync now/i }));
    await waitFor(() =>
      expect(syncAccount).toHaveBeenCalledWith("token", "acc1", { force: undefined }),
    );
  });

  // An ordinary sync only collects what changed, so a message whose stored
  // content was lost locally is never refetched without this.
  it("forces a full resync once confirmed", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /full resync/i }));
    expect(confirmSpy).toHaveBeenCalled();
    await waitFor(() =>
      expect(syncAccount).toHaveBeenCalledWith("token", "acc1", { force: true }),
    );
    confirmSpy.mockRestore();
  });

  it("does not resync when the confirmation is declined", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /full resync/i }));
    expect(syncAccount).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });
});
