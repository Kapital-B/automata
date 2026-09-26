import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import InboxPage from "@/pages/Inbox";
import * as auth from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";

const mobile = vi.hoisted(() => ({ below: false }));

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/hooks/use-mobile", () => ({
  useIsBelowLg: () => mobile.below,
  useIsMobile: () => mobile.below,
}));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({
    accounts: [
      { id: "a1", label: "Work", primaryEmail: "w@example.com", provider: "m365", kind: "work", status: "connected", colorVar: "acct-1" } as UiAccount,
    ],
    isLoading: false,
    isError: false,
  }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listCategories: vi.fn(),
    listMessages: vi.fn(),
    getMessage: vi.fn(),
    listProjects: vi.fn(),
    listDraftSuggestions: vi.fn(),
    getForwardAllowlist: vi.fn(),
    syncAccount: vi.fn(),
    generateDraftSuggestions: vi.fn(),
  };
});

const listMessages = vi.mocked(auth.listMessages);

function message(overrides: Partial<auth.MessageItem> = {}): auth.MessageItem {
  return {
    id: "m1",
    account_id: "a1",
    provider_message_id: "p1",
    subject: "Pump P-03 duty",
    received_at: new Date().toISOString(),
    has_attachments: false,
    from_json: { name: "Jan de Vries", address: "jan@example.com" },
    preview: "Can we confirm 90 kW?",
    category_slug: "finance",
    conversation_id: "c1",
    project_id: "p1",
    ...overrides,
  };
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <InboxPage accountFilter="all" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mobile.below = false;
  vi.mocked(auth.listCategories).mockResolvedValue([
    { id: "c1", slug: "finance", display_name: "Finance & invoices", definition: "", sort_order: 1 },
  ]);
  listMessages.mockResolvedValue([
    message(),
    message({ id: "m2", subject: "Site visit", from_json: { address: "site@example.com" }, category_slug: undefined, project_id: undefined }),
  ]);
  vi.mocked(auth.getMessage).mockResolvedValue({ ...message(), body_text: "Hello, can we confirm the duty?" });
  vi.mocked(auth.listProjects).mockResolvedValue([
    { id: "p1", code: "DC01", name: "Cooling" } as auth.ProjectListItem,
  ]);
  vi.mocked(auth.listDraftSuggestions).mockResolvedValue([]);
});

describe("InboxPage", () => {
  it("lists messages as buttons with category names, and opens the first", async () => {
    renderPage();
    const list = await screen.findByRole("list", { name: "Messages" });
    const rows = within(list).getAllByRole("button");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Jan de Vries");
    expect(rows[0]).toHaveTextContent("DC01");
    // Categories read by name, not slug.
    expect(rows[0]).toHaveTextContent("Finance & invoices");
    expect(screen.getByRole("radio", { name: "Finance & invoices" })).toBeInTheDocument();

    const detail = await screen.findByRole("article", { name: "Pump P-03 duty" });
    expect(await within(detail).findByText("Hello, can we confirm the duty?")).toBeInTheDocument();
    expect(within(detail).getByRole("button", { name: /draft reply/i })).toBeInTheDocument();
    expect(within(detail).getByRole("button", { name: /forward/i })).toBeInTheDocument();
    expect(rows[0]).toHaveAttribute("aria-current", "true");

    fireEvent.click(rows[1]);
    expect(await screen.findByRole("article", { name: "Site visit" })).toBeInTheDocument();
  });

  it("filters by category", async () => {
    renderPage();
    await screen.findByRole("list", { name: "Messages" });
    fireEvent.click(screen.getByRole("radio", { name: "Finance & invoices" }));
    await waitFor(() =>
      expect(listMessages).toHaveBeenLastCalledWith("token", expect.objectContaining({ category: "finance" })),
    );
  });

  it("offers to clear filters when nothing matches", async () => {
    renderPage();
    await screen.findByRole("list", { name: "Messages" });
    listMessages.mockResolvedValue([]);
    fireEvent.click(screen.getByRole("radio", { name: "Finance & invoices" }));
    fireEvent.click(await screen.findByRole("button", { name: "Clear filters" }));
    await waitFor(() => expect(screen.getByRole("radio", { name: "All" })).toHaveAttribute("aria-checked", "true"));
  });

  it("shows one pane at a time on a narrow screen, with a way back", async () => {
    mobile.below = true;
    renderPage();
    const list = await screen.findByRole("list", { name: "Messages" });
    expect(screen.queryByRole("article")).not.toBeInTheDocument();

    fireEvent.click(within(list).getAllByRole("button")[1]);
    expect(await screen.findByRole("article", { name: "Site visit" })).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Messages" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Messages" }));
    expect(await screen.findByRole("list", { name: "Messages" })).toBeInTheDocument();
    expect(screen.queryByRole("article")).not.toBeInTheDocument();
  });

  it("wraps a plain-text body instead of letting it widen the page", async () => {
    vi.mocked(auth.getMessage).mockResolvedValue({ ...message(), body_text: "https://example.com/" + "x".repeat(300) });
    renderPage();
    const body = await screen.findByText(/https:\/\/example\.com\/x+/);
    expect(body.className).toContain("[overflow-wrap:anywhere]");
  });
});
