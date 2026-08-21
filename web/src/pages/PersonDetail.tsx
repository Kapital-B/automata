import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, getContact, mergeContacts } from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router-dom";

export default function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const query = useQuery({
    queryKey: ["contact", accessToken, id],
    queryFn: () => getContact(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });

  const mergeMutation = useMutation({
    mutationFn: async (sourceID: string) => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return mergeContacts(accessToken, id, sourceID);
    },
    onSuccess: async () => {
      toast({ title: "Contacts merged" });
      await queryClient.invalidateQueries({ queryKey: ["contacts"] });
      await queryClient.invalidateQueries({ queryKey: ["contact", accessToken, id] });
    },
    onError: (err) => {
      toast({
        title: "Merge failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  if (query.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading contact…
      </div>
    );
  }

  if (query.isError || !query.data) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Contact not found."}
        </p>
        <Button variant="outline" onClick={() => navigate("/people")}>
          Back to People
        </Button>
      </div>
    );
  }

  const contact = query.data;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="People"
        title={contact.display_name || "Unnamed contact"}
        description={contact.company || undefined}
        actions={
          <Button variant="outline" asChild>
            <Link to="/people">All people</Link>
          </Button>
        }
      />

      <section className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Identities
        </h2>
        {contact.identities.length === 0 ? (
          <p className="text-sm text-muted-foreground">No identities yet.</p>
        ) : (
          <ul className="divide-y divide-border/70 border-y border-border/70">
            {contact.identities.map((ident) => (
              <li key={ident.id} className="flex items-baseline justify-between gap-4 py-2.5">
                <span className="text-xs uppercase tracking-wide text-muted-foreground">
                  {ident.kind}
                </span>
                <span className="font-mono text-sm">{ident.value_raw}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Recent messages
        </h2>
        {contact.recent_messages.length === 0 ? (
          <p className="text-sm text-muted-foreground">No linked messages yet.</p>
        ) : (
          <ul className="space-y-2">
            {contact.recent_messages.map((m) => (
              <li key={m.message_id}>
                <Link
                  to={`/inbox?message_id=${encodeURIComponent(m.message_id)}&account_id=${encodeURIComponent(m.account_id)}`}
                  className="text-sm text-primary underline underline-offset-4"
                >
                  Open in Inbox
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      {contact.suggested_merges.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
            Suggested merges
          </h2>
          <p className="text-sm text-muted-foreground">
            Same display name, different emails. Confirm to merge into this contact.
          </p>
          <ul className="space-y-3">
            {contact.suggested_merges.map((s) => (
              <li
                key={s.id}
                className="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 py-2"
              >
                <Link to={`/people/${s.id}`} className="text-sm font-medium hover:underline">
                  {s.display_name || "Unnamed contact"}
                </Link>
                <Button
                  size="sm"
                  disabled={mergeMutation.isPending}
                  onClick={() => mergeMutation.mutate(s.id)}
                >
                  Confirm merge
                </Button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
