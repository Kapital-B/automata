import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { dismissFYI, generateDraftSuggestions, getSummary, listDraftSuggestions, markActionItemDone, refreshSummary } from "@/lib/auth";
import { RefreshCw, CheckCircle2, Info } from "lucide-react";
import { Link } from "react-router-dom";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/hooks/use-toast";
import { relativeTime } from "@/lib/accounts";
import { useEffect, useMemo, useState } from "react";

interface Props {
  accountFilter: AccountFilter;
}

export default function TodayPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const activeAccountID = accountFilter === "all" ? undefined : accountFilter;

  const summaryQuery = useQuery({
    queryKey: ["summary", accessToken, activeAccountID],
    queryFn: () => getSummary(accessToken!, activeAccountID),
    enabled: Boolean(accessToken),
  });
  const actionItems = summaryQuery.data?.action_items ?? [];
  const fyi = summaryQuery.data?.fyi ?? [];
  const snapshot = summaryQuery.data?.snapshot;
  const [pendingDraftMessageKeys, setPendingDraftMessageKeys] = useState<Set<string>>(() => new Set());

  const refreshMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) return { queued: 0, failed: 0, total: 0 };
      const targetAccountIDs =
        activeAccountID != null
          ? [activeAccountID]
          : accounts.filter((a) => a.status === "connected").map((a) => a.id);
      if (targetAccountIDs.length === 0) {
        return { queued: 0, failed: 0, total: 0 };
      }
      const results = await Promise.allSettled(targetAccountIDs.map((id) => refreshSummary(accessToken, id)));
      const queued = results.filter((r) => r.status === "fulfilled").length;
      return { queued, failed: results.length - queued, total: results.length };
    },
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
      if (res.total === 0) {
        toast({ title: "No connected accounts available" });
        return;
      }
      toast({
        title: "Summary refresh queued",
        description:
          res.failed > 0
            ? `${res.queued}/${res.total} accounts queued (${res.failed} failed).`
            : `${res.queued} account${res.queued === 1 ? "" : "s"} queued.`,
      });
    },
  });
  const doneMutation = useMutation({
    mutationFn: async (itemID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return markActionItemDone(accessToken, itemID);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
    },
  });
  const dismissFYIMutation = useMutation({
    mutationFn: async (fyiID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return dismissFYI(accessToken, fyiID);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
    },
  });
  const draftsScope = activeAccountID ?? "all";
  const draftsQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, draftsScope],
    queryFn: () => listDraftSuggestions(accessToken!, activeAccountID),
    enabled: Boolean(accessToken),
  });
  const draftByMessageKey = useMemo(() => {
    const m = new Map<string, string>();
    for (const d of draftsQuery.data ?? []) {
      const key = `${d.account_id}:${d.message_id}`;
      if (!m.has(key)) m.set(key, d.id);
    }
    return m;
  }, [draftsQuery.data]);
  const createDraftMutation = useMutation({
    mutationFn: async ({ accountID, messageID }: { accountID: string; messageID: string }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return generateDraftSuggestions(accessToken, accountID, { messageId: messageID });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
    },
    onError: (error) => {
      toast({
        title: "Could not queue draft generation",
        description: error instanceof Error ? error.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const makeMessageKey = (accountID: string, messageID: string) => `${accountID}:${messageID}`;
  useEffect(() => {
    setPendingDraftMessageKeys((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set(prev);
      for (const key of prev) {
        if (draftByMessageKey.has(key)) {
          next.delete(key);
        }
      }
      return next;
    });
  }, [draftByMessageKey]);

  // Group action items by account for clear provenance
  const grouped = accounts
    .map((a) => ({
      account: a,
      items: actionItems.filter((i) => i.account_id === a.id),
    }))
    .filter((g) => g.items.length > 0);

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow={`Daily summary · ${new Date(snapshot?.created_at ?? Date.now()).toLocaleDateString(undefined, {
          weekday: "long",
          month: "long",
          day: "numeric",
        })}`}
        title="What needs your attention today."
        description={
          snapshot
            ? `Generated ${relativeTime(snapshot.created_at)} from ${
                accountFilter === "all" ? `${accounts.length} accounts` : accounts.find((a) => a.id === accountFilter)?.label
              }`
            : "No summary generated yet."
        }
        actions={
          <>
            <Button
              variant="outline"
              size="sm"
              disabled
              title="Bulk “mark all reviewed” needs a backend endpoint; use Done on each item for now."
            >
              <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" />
              <span>Mark all reviewed</span>
              <span className="ml-1.5 text-[10px] font-normal normal-case text-muted-foreground">(coming later)</span>
            </Button>
            <Button
              size="sm"
              className="bg-foreground text-background hover:bg-foreground/90"
              disabled={refreshMutation.isPending}
              onClick={() => refreshMutation.mutate()}
            >
              <RefreshCw className="mr-1.5 h-3.5 w-3.5" /> Refresh summary
            </Button>
          </>
        }
      />

      {/* Stats strip */}
      <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {[
          { label: "Action items", value: actionItems.length, tone: "text-foreground" },
          { label: "FYI", value: fyi.length, tone: "text-foreground" },
          { label: "Drafts ready", value: "-", tone: "text-foreground" },
          { label: "Run id", value: snapshot?.run_id.slice(0, 8) ?? "-", tone: "font-mono text-sm text-muted-foreground" },
        ].map((s) => (
          <div key={s.label} className="surface-card px-4 py-3">
            <p className="text-[10px] uppercase tracking-widest text-muted-foreground">
              {s.label}
            </p>
            <p className={`mt-1 font-display text-2xl font-medium ${s.tone}`}>{s.value}</p>
          </div>
        ))}
      </section>

      {/* Action items, grouped by account so provenance is unambiguous */}
      <section className="space-y-6">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">Action items</h2>
          <p className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            {actionItems.length} open
          </p>
        </div>

        {grouped.length === 0 ? (
          <div className="surface-card flex items-center gap-3 px-4 py-6 text-muted-foreground">
            <CheckCircle2 className="h-5 w-5 text-success" />
            Nothing pending. Inbox zero, summary-zero.
          </div>
        ) : (
          grouped.map(({ account, items }) => (
            <div key={account.id} className="space-y-2">
              <AccountBadge account={account} showEmail size="sm" />
              <ul className="surface-card divide-y divide-border/70 overflow-hidden">
                {items.map((item) => {
                  const key = makeMessageKey(item.account_id, item.message_id);
                  const draftID = draftByMessageKey.get(key);
                  const isPendingDraft = pendingDraftMessageKeys.has(key);
                  return (
                    <li key={item.id} className="group flex items-start gap-4 px-5 py-4 transition hover:bg-secondary/40">
                      <div className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-foreground/70" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm leading-snug text-foreground">{item.text}</p>
                        <Link
                          to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
                          className="mt-1 inline-block truncate text-xs text-primary underline underline-offset-2 hover:text-primary/80"
                        >
                          Message {item.message_id.slice(0, 8)}
                        </Link>
                        <p className="mt-1 text-xs text-muted-foreground">Created {relativeTime(item.created_at)}</p>
                      </div>
                      {item.due_at && (
                        <span className="rounded-full border border-border bg-background px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-foreground/80">
                          {item.is_overdue ? "Overdue" : new Date(item.due_at).toLocaleDateString()}
                        </span>
                      )}
                      <div className="flex items-center gap-2">
                        {draftID ? (
                          <Button asChild size="sm" variant="outline">
                            <Link to={`/drafts?draft_id=${encodeURIComponent(draftID)}`}>Open draft</Link>
                          </Button>
                        ) : (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={isPendingDraft || createDraftMutation.isPending}
                            onClick={() => {
                              setPendingDraftMessageKeys((prev) => new Set(prev).add(key));
                              createDraftMutation.mutate({ accountID: item.account_id, messageID: item.message_id }, {
                                onSuccess: () => {
                                  toast({ title: "Draft generation queued" });
                                },
                              });
                            }}
                          >
                            {isPendingDraft ? "Draft queued..." : "Create draft"}
                          </Button>
                        )}
                        <Button size="sm" variant="outline" onClick={() => doneMutation.mutate(item.id)}>Done</Button>
                      </div>
                    </li>
                  );
                })}
              </ul>
            </div>
          ))
        )}
      </section>

      {/* FYI */}
      <section className="space-y-4">
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-2xl">For your awareness</h2>
          <p className="text-xs font-mono uppercase tracking-widest text-muted-foreground">
            {fyi.length} items
          </p>
        </div>
        <ul className="grid gap-3 md:grid-cols-2">
          {fyi.map((item) => (
            <li key={item.id} className="surface-card flex items-start gap-3 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="text-sm leading-snug">{item.text}</p>
                <Link
                  to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
                  className="mt-1 inline-block truncate text-xs text-primary underline underline-offset-2 hover:text-primary/80"
                >
                  Message {item.message_id.slice(0, 8)}
                </Link>
                <p className="mt-1 text-xs text-muted-foreground">Created {relativeTime(item.created_at)}</p>
                <div className="mt-2">
                  <AccountBadge account={accounts.find((a) => a.id === item.account_id)} />
                </div>
              </div>
              <Button size="sm" variant="outline" onClick={() => dismissFYIMutation.mutate(item.id)}>
                Dismiss
              </Button>
            </li>
          ))}
        </ul>
      </section>

      <footer className="hairline pt-6 text-xs text-muted-foreground">
        Window: {snapshot ? new Date(snapshot.window_start).toLocaleString() : "-"} → {snapshot ? new Date(snapshot.window_end).toLocaleString() : "-"}
        <span className="mx-2">·</span>
        Run <span className="font-mono">{snapshot?.run_id ?? "-"}</span>
      </footer>
    </div>
  );
}
