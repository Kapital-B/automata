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

  it("shows primary Home Projects Triage People and Inbox under More", async () => {
    renderSidebar();

    expect(await screen.findByRole("link", { name: /^Home/i })).toHaveAttribute("href", "/");
    expect(screen.getByRole("link", { name: /^Projects/i })).toHaveAttribute("href", "/projects");
    expect(screen.getByRole("link", { name: /^Triage/i })).toHaveAttribute("href", "/triage");
    expect(screen.getByRole("link", { name: /^People/i })).toHaveAttribute("href", "/people");

    expect(screen.getByText("More")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /^Inbox/i })).toHaveAttribute("href", "/inbox");
    expect(screen.getByRole("link", { name: /^Drafts/i })).toHaveAttribute("href", "/drafts");
    expect(screen.getByRole("link", { name: /^Connectors/i })).toHaveAttribute(
      "href",
      "/accounts",
    );

    expect(screen.queryByRole("link", { name: /^Today$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Assistant$/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /^Unassigned$/i })).not.toBeInTheDocument();
  });
});
