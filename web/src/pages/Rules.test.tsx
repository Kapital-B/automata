import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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
    putForwardAllowlist: vi.fn(),
    createForwardRule: vi.fn(),
    updateForwardRule: vi.fn(),
    deleteForwardRule: vi.fn(),
    runForwardRules: vi.fn(),
    previewForwardRule: vi.fn(),
    listForwardRuleActivity: vi.fn(),
    listCategories: vi.fn(),
    getApiHealth: vi.fn(),
  };
});

const m = {
  listForwardRules: vi.mocked(auth.listForwardRules),
  getForwardAllowlist: vi.mocked(auth.getForwardAllowlist),
  putForwardAllowlist: vi.mocked(auth.putForwardAllowlist),
  createForwardRule: vi.mocked(auth.createForwardRule),
  updateForwardRule: vi.mocked(auth.updateForwardRule),
  deleteForwardRule: vi.mocked(auth.deleteForwardRule),
  runForwardRules: vi.mocked(auth.runForwardRules),
  previewForwardRule: vi.mocked(auth.previewForwardRule),
  listForwardRuleActivity: vi.mocked(auth.listForwardRuleActivity),
  listCategories: vi.mocked(auth.listCategories),
  getApiHealth: vi.mocked(auth.getApiHealth),
};

function account(id: string, label: string, serverSideForward = true): UiAccount {
  return {
    id,
    label,
    primaryEmail: `${label.toLowerCase()}@example.com`,
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

function rule(over: Partial<auth.ForwardRule> = {}): auth.ForwardRule {
  return {
    id: "r1",
    account_id: "acc1",
    name: "Invoices",
    mode: "logic",
    condition_json: { all: [{ field: "category_slug", op: "equals", value: "finance" }] },
    forward_to: "bills@example.com",
    enabled: false,
    applies_from: "2026-09-20T00:00:00Z",
    created_at: "2026-09-20T00:00:00Z",
    stats: { forwarded: 3, failed: 1, pending: 0 },
    ...over,
  };
}

function renderPage(filter = "acc1") {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <RulesPage accountFilter={filter} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  state.accounts = [account("acc1", "Work")];
  m.getForwardAllowlist.mockResolvedValue({ emails: ["bills@example.com"] });
  m.listForwardRules.mockResolvedValue([]);
  m.listCategories.mockResolvedValue([
    { id: "c1", slug: "finance", display_name: "Finance", definition: "", sort_order: 20 },
    { id: "c2", slug: "newsletter", display_name: "Newsletter", definition: "", sort_order: 40 },
  ]);
  m.getApiHealth.mockResolvedValue({ status: "ok", llm: true });
  m.createForwardRule.mockResolvedValue({ id: "new" });
  m.updateForwardRule.mockResolvedValue({ status: "ok" });
  m.deleteForwardRule.mockResolvedValue(undefined);
  m.runForwardRules.mockResolvedValue({ job_run_id: "run", status: "queued" });
  m.putForwardAllowlist.mockResolvedValue(undefined);
  m.listForwardRuleActivity.mockResolvedValue([]);
});

describe("Creating rules", () => {
  // Conditions used to be typed as JSON, with a default that named a
  // category that does not exist.
  it("builds a condition from fields and creates the rule paused", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /new rule/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Supplier invoices" } });
    await waitFor(() => expect(within(dialog).getByLabelText("Condition 1 value")).toHaveValue("finance"));
    fireEvent.click(within(dialog).getByRole("button", { name: /add condition/i }));
    const save = within(dialog).getByRole("button", { name: /create rule/i });
    expect(save).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Condition 2 value"), { target: { value: "Invoice" } });
    fireEvent.click(save);
    await waitFor(() =>
      expect(m.createForwardRule).toHaveBeenCalledWith("token", "acc1", {
        name: "Supplier invoices",
        mode: "logic",
        condition_json: {
          all: [
            { field: "category_slug", op: "equals", value: "finance" },
            { field: "subject", op: "contains", value: "Invoice" },
          ],
        },
        forward_to: "bills@example.com",
        enabled: false,
      }),
    );
  });

  it("warns before saving a rule on a mailbox that re-sends copies", async () => {
    state.accounts = [account("acc1", "Personal", false)];
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /new rule/i }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("note")).toHaveTextContent(/cannot forward server-side/);
  });

  it("shows the server's reason when a rule is refused", async () => {
    m.createForwardRule.mockRejectedValue(new auth.ApiError("the destination must be on your forwarding allowlist", 422));
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /new rule/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "x" } });
    await waitFor(() => expect(within(dialog).getByLabelText("Condition 1 value")).toHaveValue("finance"));
    fireEvent.click(within(dialog).getByRole("button", { name: /create rule/i }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(/allowlist/);
  });
});

describe("Switching rules on and off", () => {
  // The first run after switching on used to forward every matching message
  // ever synced, without asking.
  it("asks whether existing mail is included, with a count, before switching on", async () => {
    m.listForwardRules.mockResolvedValue([rule()]);
    m.previewForwardRule.mockResolvedValue({
      in_scope: 120,
      matched: 14,
      capped: false,
      samples: [{ subject: "Invoice 7", from_name: "Acme", from_address: "a@acme.com", received_at: "2026-09-01T00:00:00Z" }],
    });
    renderPage();
    fireEvent.click(await screen.findByRole("switch", { name: /switch on invoices/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("radio", { name: /existing mail too/i }));
    expect(await within(dialog).findByText(/of 120 existing messages match/)).toBeInTheDocument();
    expect(within(dialog).getByText(/14/)).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: /^switch on$/i }));
    await waitFor(() =>
      expect(m.updateForwardRule).toHaveBeenCalledWith("token", "r1", expect.objectContaining({ enabled: true, start: "existing" })),
    );
  });

  it("switches on for new mail only by default", async () => {
    m.listForwardRules.mockResolvedValue([rule()]);
    renderPage();
    fireEvent.click(await screen.findByRole("switch", { name: /switch on invoices/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: /^switch on$/i }));
    await waitFor(() =>
      expect(m.updateForwardRule).toHaveBeenCalledWith("token", "r1", expect.objectContaining({ enabled: true, start: "new" })),
    );
    expect(m.previewForwardRule).not.toHaveBeenCalled();
  });

  it("switches off without re-scoping", async () => {
    m.listForwardRules.mockResolvedValue([rule({ enabled: true })]);
    renderPage();
    fireEvent.click(await screen.findByRole("switch", { name: /switch off invoices/i }));
    await waitFor(() => expect(m.updateForwardRule).toHaveBeenCalled());
    expect(m.updateForwardRule.mock.calls[0][2]).toMatchObject({ enabled: false });
    expect(m.updateForwardRule.mock.calls[0][2]).not.toHaveProperty("start");
  });
});

describe("Reading rules", () => {
  it("describes the condition in words and shows what it has done", async () => {
    m.listForwardRules.mockResolvedValue([rule({ enabled: true })]);
    renderPage();
    expect(await screen.findByText("Category is Finance")).toBeInTheDocument();
    expect(screen.getByText(/3 forwarded/)).toHaveTextContent(/1 failed/);
    expect(screen.getByText(/Mail from/)).toBeInTheDocument();
  });

  it("explains a blocked rule", async () => {
    m.listForwardRules.mockResolvedValue([rule({ enabled: true, blocked_reason: "its destination is no longer on the allowlist" })]);
    renderPage();
    expect(await screen.findByRole("alert")).toHaveTextContent(/no longer on the allowlist/);
    expect(screen.getByText("Blocked")).toBeInTheDocument();
  });

  it("shows recent activity on request", async () => {
    m.listForwardRules.mockResolvedValue([rule({ enabled: true })]);
    m.listForwardRuleActivity.mockResolvedValue([
      {
        message_id: "m1",
        account_id: "acc1",
        subject: "Invoice 7",
        from_name: "Acme",
        from_address: "a@acme.com",
        received_at: "2026-09-01T00:00:00Z",
        status: "failed",
        pending: true,
        attempts: 1,
        reason: "could not send; will retry",
        at: "2026-09-01T00:00:00Z",
      },
    ]);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /activity/i }));
    expect(await screen.findByText("Invoice 7")).toBeInTheDocument();
    expect(screen.getByLabelText("Waiting")).toBeInTheDocument();
  });

  it("asks before deleting", async () => {
    m.listForwardRules.mockResolvedValue([rule()]);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /^delete$/i }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(m.deleteForwardRule).not.toHaveBeenCalled();
  });
});

// "All accounts" used to mean the first account, silently.
describe("All accounts", () => {
  it("shows every account's rules and runs every account with a rule on", async () => {
    state.accounts = [account("acc1", "Work"), account("acc2", "Home")];
    m.listForwardRules.mockImplementation(async (_t, id) =>
      id === "acc1" ? [rule({ enabled: true })] : [rule({ id: "r2", account_id: "acc2", name: "Receipts", enabled: false })],
    );
    renderPage("all");
    expect(await screen.findByText("Invoices")).toBeInTheDocument();
    expect(screen.getByText("Receipts")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /run now/i }));
    await waitFor(() => expect(m.runForwardRules).toHaveBeenCalledTimes(1));
    expect(m.runForwardRules).toHaveBeenCalledWith("token", "acc1");
  });
});

describe("Allowlist", () => {
  it("checks an address before sending it", async () => {
    renderPage();
    const input = await screen.findByLabelText("Add an address");
    fireEvent.change(input, { target: { value: "not an address" } });
    fireEvent.click(screen.getByRole("button", { name: /add address/i }));
    expect(await screen.findByText(/is not an email address/)).toBeInTheDocument();
    expect(m.putForwardAllowlist).not.toHaveBeenCalled();
  });

  it("asks before removing an address a rule uses, naming the rule", async () => {
    m.listForwardRules.mockResolvedValue([rule({ enabled: true })]);
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Remove bills@example.com" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent(/Invoices forwards here/);
    fireEvent.click(within(dialog).getByRole("button", { name: /remove address/i }));
    await waitFor(() => expect(m.putForwardAllowlist).toHaveBeenCalledWith("token", []));
  });
});
