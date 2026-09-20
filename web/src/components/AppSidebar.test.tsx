import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AppSidebar } from "@/components/AppSidebar";
import { SidebarProvider } from "@/components/ui/sidebar";
import * as auth from "@/lib/auth";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({
    accessToken: "token",
    user: { email: "op@example.com" },
    signOut: vi.fn(),
  }),
}));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({ accounts: [] }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    getUnassignedSummary: vi.fn(),
    listDraftSuggestions: vi.fn(),
    getAttention: vi.fn(),
  };
});

const getUnassignedSummary = vi.mocked(auth.getUnassignedSummary);
const listDraftSuggestions = vi.mocked(auth.listDraftSuggestions);
const getAttention = vi.mocked(auth.getAttention);

function renderSidebar() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SidebarProvider>
          <AppSidebar />
        </SidebarProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("AppSidebar U1 IA", () => {
  beforeEach(() => {
    getUnassignedSummary.mockResolvedValue({ unassigned: 0, provisional: 0 });
    listDraftSuggestions.mockResolvedValue([]);
    getAttention.mockResolvedValue({ items: [], counts: { total: 0 } } as never);
  });

  it("groups destinations by domain rather than by our ranking", async () => {
    renderSidebar();

    // Home leads, ungrouped: it is the landing, not a category.
    expect(await screen.findByRole("link", { name: /^Home/i })).toHaveAttribute("href", "/");

    // Group labels say what the destinations are about.
    expect(screen.getByText("Project memory")).toBeInTheDocument();
    expect(screen.getByText("Correspondence")).toBeInTheDocument();
    expect(screen.getByText("System")).toBeInTheDocument();

    // The old ranking labels are gone.
    expect(screen.queryByText("Primary")).not.toBeInTheDocument();
    expect(screen.queryByText("More")).not.toBeInTheDocument();
  });

  it("keeps every destination reachable", async () => {
    renderSidebar();
    await screen.findByRole("link", { name: /^Home/i });

    const expected: [RegExp, string][] = [
      [/^Projects/i, "/projects"],
      [/^People/i, "/people"],
      [/^Triage/i, "/triage"],
      [/^Inbox/i, "/inbox"],
      [/^Drafts/i, "/drafts"],
      [/^Rules/i, "/rules"],
      [/^Connectors/i, "/accounts"],
      [/^Runs/i, "/runs"],
      [/^Settings/i, "/settings"],
    ];
    for (const [name, href] of expected) {
      expect(screen.getByRole("link", { name })).toHaveAttribute("href", href);
    }

    expect(screen.queryByRole("link", { name: /^Today$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Assistant$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Unassigned$/i })).not.toBeInTheDocument();
  });

  it("badges the queues that have work waiting", async () => {
    getUnassignedSummary.mockResolvedValue({ unassigned: 3, provisional: 2, not_relevant: 9 });
    listDraftSuggestions.mockResolvedValue([{ id: "d1" }] as never);
    getAttention.mockResolvedValue({ items: [], counts: { total: 4 } } as never);
    renderSidebar();

    // Triage counts work to do; dismissed items are not work.
    expect(await screen.findByTitle("Triage queue")).toHaveTextContent("5");
    expect(screen.getByTitle("Needs my input")).toHaveTextContent("4");
    // Drafts stay unbadged with no connected account, since the query that
    // feeds them is gated on one.
    expect(screen.queryByTitle("Drafts ready")).not.toBeInTheDocument();
  });
});
