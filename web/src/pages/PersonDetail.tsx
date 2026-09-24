import { ContactAvatar } from "@/components/ContactAvatar";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime } from "@/lib/accounts";
import { contactName } from "@/lib/contacts";
import {
  ApiError,
  getContact,
  mergeContacts,
  type ContactIdentity,
  type ContactRecentMessage,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, GitMerge, Mail, Phone, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";

export default function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [pendingMerge, setPendingMerge] = useState<{ id: string; name: string } | null>(null);

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

  const back = (
    <Link
      to="/people"
      className="inline-flex items-center gap-1.5 rounded-sm text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ArrowLeft aria-hidden="true" className="h-4 w-4" /> People
    </Link>
  );

  if (query.isLoading) {
    return (
      <div className="space-y-6" aria-label="Loading contact">
        {back}
        <div className="flex items-center gap-4">
          <Skeleton className="h-14 w-14 rounded-full" />
          <div className="space-y-2">
            <Skeleton className="h-7 w-56" />
            <Skeleton className="h-4 w-40" />
          </div>
        </div>
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (query.isError || !query.data) {
    return (
      <div className="space-y-4">
        {back}
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "This contact could not be found."}
        </div>
      </div>
    );
  }

  const contact = query.data;
  const emails = contact.identities.filter((i) => i.kind === "email");
  const phones = contact.identities.filter((i) => i.kind === "phone");
  const aliases = contact.identities.filter((i) => i.kind !== "email" && i.kind !== "phone");
  const name = contactName(contact.display_name, emails[0]?.value_raw);

  return (
    <div className="space-y-8">
      <div className="space-y-4 border-b border-border/70 pb-6">
        {back}
        <header className="flex items-center gap-4">
          <ContactAvatar name={contact.display_name} email={emails[0]?.value_raw} size="lg" />
          <div className="min-w-0">
            <h1 className="truncate font-display text-3xl font-medium leading-tight md:text-4xl">{name}</h1>
            {(contact.company || emails[0]) && (
              <p className="mt-1 truncate text-muted-foreground">
                {[contact.company, emails[0]?.value_raw].filter(Boolean).join(" · ")}
              </p>
            )}
          </div>
        </header>
      </div>

      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <section aria-labelledby="recent-heading" className="space-y-3">
          <SectionHeading id="recent-heading">Recent messages</SectionHeading>
          {contact.recent_messages.length === 0 ? (
            <p className="surface-card p-5 text-sm text-muted-foreground">
              No messages linked yet. They appear here once mail with this person syncs.
            </p>
          ) : (
            <ul className="surface-card divide-y divide-border/70 overflow-hidden">
              {contact.recent_messages.map((m) => (
                <RecentMessageRow key={m.message_id} message={m} />
              ))}
            </ul>
          )}
        </section>

        <aside className="space-y-8">
          <section aria-labelledby="details-heading" className="space-y-3">
            <SectionHeading id="details-heading">Contact details</SectionHeading>
            {contact.identities.length === 0 ? (
              <p className="text-sm text-muted-foreground">No addresses recorded.</p>
            ) : (
              <dl className="space-y-4 text-sm">
                <IdentityGroup label="Email" icon={Mail} items={emails} href={(v) => `mailto:${v}`} />
                <IdentityGroup label="Phone" icon={Phone} items={phones} href={(v) => `tel:${v}`} />
                <IdentityGroup label="Also known as" items={aliases} />
              </dl>
            )}
          </section>

          {contact.suggested_merges.length > 0 && (
            <section aria-labelledby="merge-heading" className="space-y-3">
              <SectionHeading id="merge-heading">Possible duplicates</SectionHeading>
              <p className="text-sm text-muted-foreground">
                Same name, different address. Merging moves their addresses and messages onto this contact.
              </p>
              <ul className="space-y-2">
                {contact.suggested_merges.map((s) => {
                  const other = contactName(s.display_name);
                  return (
                    <li key={s.id} className="surface-card flex items-center justify-between gap-3 px-3 py-2">
                      <Link
                        to={`/people/${s.id}`}
                        className="min-w-0 truncate text-sm font-medium hover:underline focus-visible:underline focus-visible:outline-none"
                      >
                        {other}
                      </Link>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={mergeMutation.isPending}
                        onClick={() => setPendingMerge({ id: s.id, name: other })}
                      >
                        <GitMerge aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Merge
                      </Button>
                    </li>
                  );
                })}
              </ul>
            </section>
          )}
        </aside>
      </div>

      <AlertDialog open={pendingMerge !== null} onOpenChange={(open) => !open && setPendingMerge(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Merge into {name}?</AlertDialogTitle>
            <AlertDialogDescription>
              {pendingMerge?.name}'s addresses and messages move onto {name}, and the separate contact goes away.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (pendingMerge) mergeMutation.mutate(pendingMerge.id);
                setPendingMerge(null);
              }}
            >
              Confirm merge
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function SectionHeading({ id, children }: { id: string; children: React.ReactNode }) {
  return (
    <h2 id={id} className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
      {children}
    </h2>
  );
}

function IdentityGroup({
  label,
  icon: Icon,
  items,
  href,
}: {
  label: string;
  icon?: LucideIcon;
  items: ContactIdentity[];
  href?: (value: string) => string;
}) {
  if (items.length === 0) return null;
  return (
    <div className="space-y-1">
      <dt className="flex items-center gap-1.5 text-muted-foreground">
        {Icon && <Icon aria-hidden="true" className="h-3.5 w-3.5" />} {label}
      </dt>
      {items.map((i) => (
        <dd key={i.id} className="break-all">
          {href ? (
            <a href={href(i.value_raw)} className="text-foreground hover:underline focus-visible:underline focus-visible:outline-none">
              {i.value_raw}
            </a>
          ) : (
            i.value_raw
          )}
        </dd>
      ))}
    </div>
  );
}

function RecentMessageRow({ message: m }: { message: ContactRecentMessage }) {
  const sender = m.from_name?.trim() || m.from_address || "Unknown sender";
  return (
    <li>
      <Link
        to={`/inbox?message_id=${encodeURIComponent(m.message_id)}&account_id=${encodeURIComponent(m.account_id)}`}
        className="flex min-h-14 items-center gap-4 px-4 py-2.5 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <div className="min-w-0 flex-1">
          <p className="truncate font-medium">{m.subject?.trim() || "(no subject)"}</p>
          <p className="truncate text-sm text-muted-foreground">{sender}</p>
        </div>
        {m.received_at && (
          <time dateTime={m.received_at} className="shrink-0 text-xs text-muted-foreground" title={new Date(m.received_at).toLocaleString()}>
            {relativeTime(m.received_at)}
          </time>
        )}
      </Link>
    </li>
  );
}
