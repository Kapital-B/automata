import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SettingsPage from "@/pages/Settings";
import * as auth from "@/lib/auth";
import { slugFor } from "@/lib/categories";
import type { UiAccount } from "@/lib/accounts";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/hooks/useAccountsData", () => ({
  useAccountsData: () => ({
    accounts: [{ id: "acc1", label: "Work", primaryEmail: "w@example.com", provider: "m365", kind: "work", status: "connected", colorVar: "acct-1" } as UiAccount],
    isLoading: false,
    isError: false,
  }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listCategories: vi.fn(),
    createCategory: vi.fn(),
    updateCategory: vi.fn(),
    deleteCategory: vi.fn(),
    getSummarySettings: vi.fn(),
    updateSummarySettings: vi.fn(),
    getScheduleSettings: vi.fn(),
    updateScheduleSettings: vi.fn(),
  };
});

const m = {
  listCategories: vi.mocked(auth.listCategories),
  createCategory: vi.mocked(auth.createCategory),
  updateCategory: vi.mocked(auth.updateCategory),
  deleteCategory: vi.mocked(auth.deleteCategory),
  getSummarySettings: vi.mocked(auth.getSummarySettings),
  updateSummarySettings: vi.mocked(auth.updateSummarySettings),
  getScheduleSettings: vi.mocked(auth.getScheduleSettings),
  updateScheduleSettings: vi.mocked(auth.updateScheduleSettings),
};

const categories: auth.CategoryDefinition[] = [
  { id: "c1", slug: "finance", display_name: "Finance", definition: "Invoices", sort_order: 10 },
  { id: "c2", slug: "newsletter", display_name: "Newsletter", definition: "Digests", sort_order: 20 },
  { id: "c3", slug: "other", display_name: "Other", definition: "", sort_order: 30 },
];

function renderPage(tab?: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[tab ? `/settings?tab=${tab}` : "/settings"]}>
        <SettingsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  m.listCategories.mockResolvedValue(categories);
  m.createCategory.mockResolvedValue(categories[0]);
  m.updateCategory.mockResolvedValue(categories[0]);
  m.deleteCategory.mockResolvedValue({ status: "success" });
  m.getSummarySettings.mockResolvedValue({ include_category_slugs: [], exclude_category_slugs: [], chunk_size: 12 });
  m.updateSummarySettings.mockResolvedValue({ status: "success" });
  m.getScheduleSettings.mockResolvedValue({ chains: [], available_jobs: ["sync", "resolve_contacts", "categorize", "assign_projects", "summarize", "forward_rules"] });
  m.updateScheduleSettings.mockResolvedValue({ status: "success" });
});

describe("slugFor", () => {
  it("makes identifiers the server accepts", () => {
    expect(slugFor("Travel & Expenses")).toBe("travel-expenses");
    expect(slugFor("  Café ")).toBe("cafe");
    expect(slugFor("HR")).toBe("hr");
  });
});

describe("Categories", () => {
  it("derives the identifier from the name and refuses a duplicate", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /add category/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Finance" } });
    expect(within(dialog).getByText(/already exists/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: /add category/i })).toBeDisabled();
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Travel & Expenses" } });
    fireEvent.change(within(dialog).getByLabelText("Description"), { target: { value: "Flights and hotels" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /add category/i }));
    await waitFor(() =>
      expect(m.createCategory).toHaveBeenCalledWith("token", {
        slug: "travel-expenses",
        display_name: "Travel & Expenses",
        definition: "Flights and hotels",
        sort_order: 40,
      }),
    );
  });

  // Rules and summary filters refer to the slug; renaming must not change it.
  it("keeps the identifier when a category is renamed", async () => {
    renderPage();
    const row = (await screen.findByText("Newsletter")).closest("li")!;
    fireEvent.click(within(row).getByRole("button", { name: /edit/i }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Bulletins" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(m.updateCategory).toHaveBeenCalledWith("token", "c2", expect.objectContaining({ slug: "newsletter", display_name: "Bulletins" })),
    );
  });

  it("asks where a deleted category's mail goes", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Delete Finance" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByLabelText("Move its messages to")).toHaveValue("c3");
    fireEvent.change(within(dialog).getByLabelText("Move its messages to"), { target: { value: "c2" } });
    fireEvent.click(within(dialog).getByRole("button", { name: /delete category/i }));
    await waitFor(() => expect(m.deleteCategory).toHaveBeenCalledWith("token", "c1", "c2"));
  });

  it("reorders with move buttons", async () => {
    renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Move Finance down" }));
    await waitFor(() => expect(m.updateCategory).toHaveBeenCalledTimes(2));
    expect(m.updateCategory).toHaveBeenCalledWith("token", "c2", expect.objectContaining({ sort_order: 10 }));
    expect(m.updateCategory).toHaveBeenCalledWith("token", "c1", expect.objectContaining({ sort_order: 20 }));
    expect(screen.getByRole("button", { name: "Move Finance up" })).toBeDisabled();
  });
});

describe("Summaries", () => {
  it("saves 'all except' as an exclude list, only once something changed", async () => {
    renderPage("summaries");
    const box = await screen.findByLabelText(/Newsletter/);
    expect(screen.queryByRole("region", { name: "Unsaved changes" })).not.toBeInTheDocument();
    fireEvent.click(box);
    fireEvent.click(await screen.findByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(m.updateSummarySettings).toHaveBeenCalledWith("token", {
        include_category_slugs: [],
        exclude_category_slugs: ["newsletter"],
        chunk_size: 12,
      }),
    );
  });

  // Include and exclude used to be separate toggles that could contradict;
  // a stored contradiction is shown for what it is: nothing summarised.
  it("shows contradictory stored filters as the single choice they amount to", async () => {
    m.getSummarySettings.mockResolvedValue({ include_category_slugs: ["finance"], exclude_category_slugs: ["finance"], chunk_size: 12 });
    renderPage("summaries");
    expect(await screen.findByRole("radio", { name: /only the categories i pick/i })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("alert")).toHaveTextContent(/nothing will be summarised/i);
  });

  it("will not save a choice that summarises nothing", async () => {
    m.getSummarySettings.mockResolvedValue({ include_category_slugs: ["finance"], exclude_category_slugs: [], chunk_size: 12 });
    renderPage("summaries");
    fireEvent.click(await screen.findByLabelText(/Finance/));
    expect(screen.getByRole("alert")).toHaveTextContent(/nothing will be summarised/i);
    expect(screen.getByRole("button", { name: /save changes/i })).toBeDisabled();
  });
});

describe("Schedules", () => {
  it("opens from a deep link and builds steps from a list, in pipeline order", async () => {
    renderPage("schedules");
    fireEvent.click((await screen.findAllByRole("button", { name: /add schedule/i }))[0]);
    fireEvent.click(await screen.findByLabelText(/Run forwarding rules/));
    fireEvent.click(screen.getByLabelText(/^Sync mail/));
    fireEvent.click(screen.getByLabelText(/^Sync mail/));
    fireEvent.change(screen.getByLabelText("How often"), { target: { value: "360" } });
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(m.updateScheduleSettings).toHaveBeenCalled());
    const [chain] = m.updateScheduleSettings.mock.calls[0][1];
    expect(chain.jobs).toEqual(["sync", "resolve_contacts", "categorize", "summarize", "forward_rules"]);
    expect(chain.interval_minutes).toBe(360);
    expect(chain.enabled).toBe(true);
  });

  // "auto-draft" was suggested by this page and could never run.
  it("flags a step a schedule cannot run and drops it on save", async () => {
    m.getScheduleSettings.mockResolvedValue({
      chains: [{ id: "s1", name: "Nightly", jobs: ["sync", "auto-draft"], interval_minutes: 1440, enabled: true, next_run_at: new Date(Date.now() + 3600e3).toISOString() }],
      available_jobs: ["sync", "resolve_contacts", "categorize", "assign_projects", "summarize", "forward_rules"],
    });
    renderPage("schedules");
    expect(await screen.findByText(/which a schedule cannot run/)).toHaveTextContent("auto-draft");
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() => expect(m.updateScheduleSettings).toHaveBeenCalled());
    expect(m.updateScheduleSettings.mock.calls[0][1][0].jobs).toEqual(["sync"]);
  });

  it("will not save a schedule with no steps", async () => {
    m.getScheduleSettings.mockResolvedValue({
      chains: [{ id: "s1", name: "Nightly", jobs: ["sync"], interval_minutes: 60, enabled: true }],
      available_jobs: ["sync", "categorize"],
    });
    renderPage("schedules");
    fireEvent.click(await screen.findByLabelText(/^Sync mail/));
    expect(await screen.findByText(/needs at least one step/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save changes/i })).toBeDisabled();
  });

  it("asks before removing a schedule", async () => {
    m.getScheduleSettings.mockResolvedValue({
      chains: [{ id: "s1", name: "Nightly", jobs: ["sync"], interval_minutes: 60, enabled: true }],
      available_jobs: ["sync"],
    });
    renderPage("schedules");
    fireEvent.click(await screen.findByRole("button", { name: /remove schedule/i }));
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(screen.getByDisplayValue("Nightly")).toBeInTheDocument();
  });
});
