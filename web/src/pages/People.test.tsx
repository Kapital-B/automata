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

  it("renders contacts from the API", async () => {
    listContacts.mockResolvedValue([
      {
        id: "c1",
        organisation_id: "o1",
        display_name: "Sarah Chen",
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ]);
    renderPeople();
    expect(await screen.findByText("Sarah Chen")).toBeInTheDocument();
  });

  it("confirm merge hits the merge endpoint", async () => {
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
    const btn = await screen.findByRole("button", { name: "Confirm merge" });
    fireEvent.click(btn);
    await waitFor(() => {
      expect(mergeContacts).toHaveBeenCalledWith("token", "c1", "c2");
    });
  });
});
