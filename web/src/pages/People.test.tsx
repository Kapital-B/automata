import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import PeoplePage from "@/pages/People";
import PersonDetailPage from "@/pages/PersonDetail";
import * as auth from "@/lib/auth";

vi.mock("@/components/auth/AuthProvider", () => ({
  useAuth: () => ({ accessToken: "token" }),
}));

vi.mock("@/lib/auth", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth")>("@/lib/auth");
  return {
    ...actual,
    listContacts: vi.fn(),
    getContact: vi.fn(),
    mergeContacts: vi.fn(),
  };
});

const listContacts = vi.mocked(auth.listContacts);
const getContact = vi.mocked(auth.getContact);
const mergeContacts = vi.mocked(auth.mergeContacts);

function renderPeople() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/people"]}>
        <Routes>
          <Route path="/people" element={<PeoplePage />} />
          <Route path="/people/:id" element={<PersonDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderPerson(id: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/people/${id}`]}>
        <Routes>
          <Route path="/people/:id" element={<PersonDetailPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("People UI", () => {
  beforeEach(() => {
    listContacts.mockReset();
    getContact.mockReset();
    mergeContacts.mockReset();
  });

  it("lists people with their address, falling back to it for a name", async () => {
    listContacts.mockResolvedValue([
      {
        id: "c1",
        organisation_id: "o1",
        display_name: "Sarah Chen",
        company: "Acme",
        primary_email: "sarah@acme.com",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      {
        id: "c2",
        organisation_id: "o1",
        display_name: "",
        primary_email: "noreply@vendor.com",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]);
    renderPeople();
    expect(await screen.findByText("Sarah Chen")).toBeInTheDocument();
    expect(screen.getByText("sarah@acme.com · Acme")).toBeInTheDocument();
    // No "Unnamed contact": the address is the name.
    expect(screen.getByText("noreply@vendor.com")).toBeInTheDocument();
    expect(screen.queryByText("Unnamed contact")).not.toBeInTheDocument();
    expect(screen.getByText("2 people")).toBeInTheDocument();
  });

  it("offers to clear a search that matches no one", async () => {
    listContacts.mockResolvedValue([]);
    renderPeople();
    fireEvent.change(screen.getByLabelText("Search people"), { target: { value: "zed" } });
    expect(await screen.findByText(/No one matches/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show everyone" }));
    await waitFor(() => expect(screen.getByLabelText("Search people")).toHaveValue(""));
  });

  it("shows what each recent message is, not a column of identical links", async () => {
    getContact.mockResolvedValue({
      id: "c1",
      organisation_id: "o1",
      display_name: "Sarah Chen",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      identities: [
        { id: "i1", kind: "email", value_normalized: "sarah@acme.com", value_raw: "sarah@acme.com", created_at: "" },
      ],
      recent_messages: [
        {
          message_id: "m1",
          account_id: "a1",
          subject: "Pump sizing",
          from_name: "Sarah Chen",
          from_address: "sarah@acme.com",
          received_at: "2026-09-20T09:30:00Z",
        },
      ],
      suggested_merges: [],
    });
    renderPerson("c1");
    const link = await screen.findByRole("link", { name: /Pump sizing/ });
    expect(link).toHaveAttribute("href", "/inbox?message_id=m1&account_id=a1");
    expect(screen.getByRole("link", { name: "sarah@acme.com" })).toHaveAttribute("href", "mailto:sarah@acme.com");
  });

  // Merging folds one contact into another; it should not happen on one stray click.
  it("merges only after confirmation", async () => {
    getContact.mockResolvedValue({
      id: "c1",
      organisation_id: "o1",
      display_name: "Alex",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
      identities: [],
      recent_messages: [],
      suggested_merges: [{ id: "c2", display_name: "Alex" }],
    });
    mergeContacts.mockResolvedValue({ ok: true });
    renderPerson("c1");

    fireEvent.click(await screen.findByRole("button", { name: "Merge" }));
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    expect(mergeContacts).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Merge" }));
    fireEvent.click(await screen.findByRole("button", { name: "Confirm merge" }));
    await waitFor(() => {
      expect(mergeContacts).toHaveBeenCalledWith("token", "c1", "c2");
    });
  });
});
