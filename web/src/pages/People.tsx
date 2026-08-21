import { PageHeader } from "@/components/PageHeader";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, listContacts, type ContactListItem } from "@/lib/auth";
import { useQuery } from "@tanstack/react-query";
import { Loader2, Users } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

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
    queryFn: () => listContacts(accessToken!, { q: debounced || undefined, limit: 100 }),
    enabled: Boolean(accessToken),
  });

  const contacts = useMemo(() => query.data ?? [], [query.data]);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Address book"
        title="People"
        description="Contacts inferred from your mail, scoped to your organisation."
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search by name or email"
          className="max-w-md"
          aria-label="Search people"
        />
      </div>

      {query.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading people…
        </div>
      ) : query.isError ? (
        <p className="text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Could not load people."}
        </p>
      ) : contacts.length === 0 ? (
        <EmptyState hasQuery={Boolean(debounced)} />
      ) : (
        <ul className="divide-y divide-border/70 border-y border-border/70">
          {contacts.map((c) => (
            <PersonRow key={c.id} contact={c} />
          ))}
        </ul>
      )}
    </div>
  );
}

function EmptyState({ hasQuery }: { hasQuery: boolean }) {
  return (
    <div className="flex flex-col items-start gap-3 py-10 text-muted-foreground">
      <Users className="h-8 w-8 opacity-60" />
      <p className="text-sm">
        {hasQuery
          ? "No people match that search."
          : "Sync a mailbox to infer people from From, To, and Cc."}
      </p>
      {!hasQuery && (
        <Link to="/accounts" className="text-sm text-primary underline underline-offset-4">
          Open Accounts
        </Link>
      )}
    </div>
  );
}

function PersonRow({ contact }: { contact: ContactListItem }) {
  return (
    <li>
      <Link
        to={`/people/${contact.id}`}
        className="flex items-center justify-between gap-4 py-3 transition-colors hover:bg-muted/40"
      >
        <div>
          <p className="font-medium text-foreground">
            {contact.display_name || "Unnamed contact"}
          </p>
          {contact.company ? (
            <p className="text-xs text-muted-foreground">{contact.company}</p>
          ) : null}
        </div>
        <span className="text-xs text-muted-foreground">View</span>
      </Link>
    </li>
  );
}
