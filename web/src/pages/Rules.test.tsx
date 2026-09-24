import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import RulesPage from "@/pages/Rules";
import * as auth from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

const state = vi.hoisted(() => ({ accounts: [] as UiAccount[] }));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({ accounts: state.accounts, isLoading: false, isError: false }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listForwardRules: vi.fn(),
    getForwardAllowlist: vi.fn(),
    createForwardRule: vi.fn(),
  };
});

const createForwardRule = vi.mocked(auth.createForwardRule);

function account(serverSideForward: boolean): UiAccount {
  return {
    id: "acc1",
    label: "Personal",
    primaryEmail: "me@fastmail.com",
    provider: serverSideForward ? "m365" : "imap",
    kind: "work",
    status: "connected",
    colorVar: "acct-1",
    capabilities: {
      incremental_sync: true,
      server_side_forward: serverSideForward,
      server_side_reply: serverSideForward,
      reports_removals: serverSideForward,
    },
  };
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <RulesPage accountFilter="acc1" />
    </QueryClientProvider>,
  );
}

describe("Forward rules on accounts that re-send", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(auth.listForwardRules).mockResolvedValue([]);
    vi.mocked(auth.getForwardAllowlist).mockResolvedValue({ emails: [] });
    createForwardRule.mockResolvedValue(undefined as never);
  });

  it("warns before saving, and saves nothing when declined", async () => {
    state.accounts = [account(false)];
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderPage();
    expect(screen.getByRole("note")).toHaveTextContent(/cannot forward server-side/);
    fireEvent.click(screen.getByRole("button", { name: /new rule/i }));
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringMatching(/original attached/));
    expect(createForwardRule).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("saves once the warning is accepted", async () => {
    state.accounts = [account(false)];
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /new rule/i }));
    await waitFor(() => expect(createForwardRule).toHaveBeenCalled());
    confirmSpy.mockRestore();
  });

  it("does not warn for a mailbox that forwards server-side", async () => {
    state.accounts = [account(true)];
    const confirmSpy = vi.spyOn(window, "confirm");
    renderPage();
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /new rule/i }));
    await waitFor(() => expect(createForwardRule).toHaveBeenCalled());
    expect(confirmSpy).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });
});
