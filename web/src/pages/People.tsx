import { PageHeader } from "@/components/PageHeader";
import { ContactAvatar } from "@/components/ContactAvatar";
import { contactName } from "@/lib/contacts";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, listContacts, type ContactListItem } from "@/lib/auth";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Search, Users, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

const PAGE_LIMIT = 100;

export default function PeoplePage() {
  const { accessToken } = useAuth();
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");

  useEffect(() => {
    const t = window.setTimeout(() => setDebounced(q.trim()), 250);
    return () => window.clearTimeout(t);
  }, [q]);

  const query = useQuery({
    queryKey: ["contacts", accessToken, debounced],
    queryFn: () => listContacts(accessToken!, { q: debounced || undefined, limit: PAGE_LIMIT }),
    enabled: Boolean(accessToken),
    placeholderData: (previous) => previous,
  });

  const contacts = useMemo(() => query.data ?? [], [query.data]);
  const searching = query.isFetching && !query.isLoading;

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Address book"
        title="People"
        description="Everyone you correspond with, gathered from the senders and recipients of synced mail."
      />

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative w-full sm:max-w-sm">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search name, email or company"
            className="pl-9 pr-9"
            aria-label="Search people"
          />
          {q && (
            <button
              type="button"
              onClick={() => setQ("")}
              aria-label="Clear search"
              className="absolute right-1 top-1/2 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <X className="h-4 w-4" />
            </button>
          )}
        </div>
        {!query.isLoading && !query.isError && contacts.length > 0 && (
          <p className="text-sm text-muted-foreground" aria-live="polite">
            {searching
              ? "Searching…"
              : contacts.length >= PAGE_LIMIT
                ? `Showing the first ${PAGE_LIMIT}. Search to narrow down.`
                : `${contacts.length} ${contacts.length === 1 ? "person" : "people"}`}
          </p>
        )}
      </div>

      {query.isLoading ? (
        <ListSkeleton />
      ) : query.isError ? (
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Could not load people."}
        </div>
      ) : contacts.length === 0 ? (
        <EmptyState query={debounced} onClear={() => setQ("")} />
      ) : (
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {contacts.map((c) => (
            <PersonRow key={c.id} contact={c} />
          ))}
        </ul>
      )}
    </div>
  );
}

function ListSkeleton() {
  return (
    <ul aria-label="Loading people" className="surface-card divide-y divide-border/70">
      {Array.from({ length: 6 }, (_, i) => (
        <li key={i} className="flex items-center gap-3 px-4 py-3">
          <Skeleton className="h-9 w-9 rounded-full" />
          <div className="space-y-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-56" />
          </div>
        </li>
      ))}
    </ul>
  );
}

function EmptyState({ query, onClear }: { query: string; onClear: () => void }) {
  return (
    <div className="surface-card flex flex-col items-center gap-3 px-6 py-12 text-center">
      <Users aria-hidden="true" className="h-8 w-8 text-muted-foreground" />
      {query ? (
        <>
          <p className="text-sm text-muted-foreground">No one matches “{query}”.</p>
          <button
            type="button"
            onClick={onClear}
            className="text-sm font-medium text-primary underline-offset-4 hover:underline"
          >
            Show everyone
          </button>
        </>
      ) : (
        <>
          <p className="font-medium">No people yet</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            People appear as mail syncs: everyone in From, To and Cc becomes a contact.
          </p>
          <Link to="/accounts" className="text-sm font-medium text-primary underline-offset-4 hover:underline">
            Connect or sync a mailbox
          </Link>
        </>
      )}
    </div>
  );
}

function PersonRow({ contact }: { contact: ContactListItem }) {
  const name = contactName(contact.display_name, contact.primary_email);
  const secondary = [contact.display_name?.trim() ? contact.primary_email : undefined, contact.company]
    .filter(Boolean)
    .join(" · ");
  return (
    <li>
      <Link
        to={`/people/${contact.id}`}
        className="group flex min-h-14 items-center gap-3 px-4 py-2.5 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <ContactAvatar name={contact.display_name} email={contact.primary_email} />
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium text-foreground">{name}</p>
          {secondary && <p className="truncate text-sm text-muted-foreground">{secondary}</p>}
        </div>
        <ChevronRight
          aria-hidden="true"
          className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none motion-reduce:group-hover:translate-x-0"
        />
      </Link>
    </li>
  );
}
