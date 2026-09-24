import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccountsPage from "@/pages/Accounts";
import * as auth from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

const workAccount: UiAccount = {
  id: "acc1",
  label: "Work",
  primaryEmail: "work@ex.com",
  colorVar: "acct-1",
  status: "connected",
  provider: "m365",
  kind: "work",
  lastSyncedAt: "2026-09-21T10:00:00Z",
};

const state = vi.hoisted(() => ({ accounts: [] as UiAccount[] }));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({ accounts: state.accounts, isLoading: false, isError: false }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    syncAccount: vi.fn(),
    listConnectors: vi.fn(),
    listProjects: vi.fn(),
    listMailProviders: vi.fn(),
    connectImapAccount: vi.fn(),
    startMailboxConnect: vi.fn(),
    listConnectorBindings: vi.fn(),
    createConnectorBinding: vi.fn(),
  };
});

const syncAccount = vi.mocked(auth.syncAccount);
const listConnectors = vi.mocked(auth.listConnectors);
const listProjects = vi.mocked(auth.listProjects);
const listMailProviders = vi.mocked(auth.listMailProviders);
const connectImapAccount = vi.mocked(auth.connectImapAccount);
const startMailboxConnect = vi.mocked(auth.startMailboxConnect);

const noServerForward: auth.MailCapabilities = {
  incremental_sync: true,
  server_side_forward: false,
  server_side_reply: false,
  reports_removals: false,
};

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
    state.accounts = [workAccount];
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

describe("Connecting mailboxes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.accounts = [];
    listConnectors.mockResolvedValue([]);
    listProjects.mockResolvedValue([]);
    listMailProviders.mockResolvedValue([
      { provider: "m365", connect: "oauth", capabilities: { ...noServerForward, server_side_forward: true } },
      { provider: "imap", connect: "password", capabilities: noServerForward },
    ]);
  });

  it("offers only the providers the server can connect", async () => {
    renderPage();
    fireEvent.click(screen.getAllByRole("button", { name: /add account/i })[0]);
    expect(await screen.findByText("Other (IMAP)")).toBeInTheDocument();
    expect(screen.getByText("Microsoft")).toBeInTheDocument();
    expect(screen.queryByText("Google")).not.toBeInTheDocument();
  });

  // A wrong password has to fail at connect, in the dialog, naming the cause.
  it("fills servers from a preset and shows the server's rejection", async () => {
    connectImapAccount.mockRejectedValue(
      new auth.ApiError("the IMAP server at imap.fastmail.com refused the username or password", 422),
    );
    renderPage();
    fireEvent.click(screen.getAllByRole("button", { name: /add account/i })[0]);
    fireEvent.click(await screen.findByText("Other (IMAP)"));
    fireEvent.change(screen.getByLabelText(/^email address/i), { target: { value: "me@fastmail.com" } });
    fireEvent.change(screen.getByLabelText(/^password/i), { target: { value: "wrong" } });
    fireEvent.click(screen.getByRole("button", { name: /connect mailbox/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/refused the username or password/);
    expect(connectImapAccount).toHaveBeenCalledWith("token", {
      email: "me@fastmail.com",
      username: undefined,
      password: "wrong",
      imap: { host: "imap.fastmail.com", port: 993, security: "tls" },
      smtp: { host: "smtp.fastmail.com", port: 465, security: "tls" },
    });
  });

  it("says what a provider without server-side forward does instead", () => {
    state.accounts = [{ ...workAccount, provider: "imap", capabilities: noServerForward }];
    renderPage();
    expect(screen.getByText(/sent as a new message with the original attached/i)).toBeInTheDocument();
    expect(screen.getByText(/deleted or moved out of the inbox/i)).toBeInTheDocument();
  });

  // Reconnect used to just queue another sync, which fails the same way.
  it("reconnects an expired Microsoft account through sign-in, keeping its kind", async () => {
    startMailboxConnect.mockReturnValue(new Promise(() => {}));
    state.accounts = [{ ...workAccount, status: "expired", kind: "personal" }];
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /reconnect/i }));
    fireEvent.click(await screen.findByRole("button", { name: /continue to microsoft sign-in/i }));
    await waitFor(() => expect(startMailboxConnect).toHaveBeenCalledWith("token", "m365", "personal"));
  });

  it("reconnects an expired IMAP account with its address filled in", async () => {
    state.accounts = [{ ...workAccount, status: "expired", provider: "imap", primaryEmail: "me@icloud.com" }];
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /reconnect/i }));
    expect(await screen.findByLabelText(/^email address/i)).toHaveValue("me@icloud.com");
    expect(screen.getByLabelText(/mail provider/i)).toHaveValue("icloud");
  });
});

describe("Slack connectors", () => {
  const listConnectorBindings = vi.mocked(auth.listConnectorBindings);
  const createConnectorBinding = vi.mocked(auth.createConnectorBinding);
  const workspace = (tenant: string): auth.ConnectorAccount => ({
    id: "s1",
    provider: "slack",
    label: "Kapital B",
    connection_status: "connected",
    external_tenant_id: tenant,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  });

  beforeEach(() => {
    vi.clearAllMocks();
    state.accounts = [workAccount];
    listMailProviders.mockResolvedValue([]);
    listProjects.mockResolvedValue([
      { id: "p1", name: "Cooling", code: "DC01" } as auth.ProjectListItem,
    ]);
    listConnectorBindings.mockResolvedValue([
      {
        id: "b1",
        connector_account_id: "s1",
        organisation_id: "o1",
        external_channel_id: "C123",
        project_id: "p1",
        label: "#dc01-site",
        created_at: "",
        updated_at: "",
      },
    ]);
  });

  it("lists channels against their project and keeps dev hints out of real workspaces", async () => {
    listConnectors.mockResolvedValue([workspace("T0REAL")]);
    renderPage();
    expect(await screen.findByText("dc01-site")).toBeInTheDocument();
    expect(await screen.findByText("DC01 · Cooling")).toBeInTheDocument();
    expect(screen.queryByText(/C_FAKE_DC01/)).not.toBeInTheDocument();
    expect(screen.queryByText(/test workspace/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /add channel/i }));
    expect(screen.getByLabelText("Channel ID")).toHaveValue("");
  });

  it("adds a channel from a form with labelled fields", async () => {
    listConnectors.mockResolvedValue([workspace("T0REAL")]);
    createConnectorBinding.mockResolvedValue({} as auth.ConnectorBinding);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /add channel/i }));
    fireEvent.change(screen.getByLabelText("Channel ID"), { target: { value: " C999 " } });
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "#ops" } });
    fireEvent.change(screen.getByLabelText("Project"), { target: { value: "p1" } });
    fireEvent.click(screen.getByRole("button", { name: /^add channel$/i }));
    await waitFor(() =>
      expect(createConnectorBinding).toHaveBeenCalledWith("token", "s1", {
        external_channel_id: "C999",
        project_id: "p1",
        label: "#ops",
      }),
    );
  });

  it("prefills the only channel a fake workspace has", async () => {
    listConnectors.mockResolvedValue([workspace("T_FAKE")]);
    renderPage();
    expect(await screen.findByText(/test workspace/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /add channel/i }));
    expect(screen.getByLabelText("Channel ID")).toHaveValue("C_FAKE_DC01");
  });
});
