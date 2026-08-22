import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle2, Mail, Plug, RefreshCw, Server, Slack, Zap } from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime } from "@/lib/accounts";
import { dismissFYI, getAttention, markActionItemDone, refreshSummary } from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";

type Props = {
  accountFilter: AccountFilter;
};

const CONNECTOR_DEFS = [
  { name: "Email", icon: Mail },
  { name: "Slack", icon: Slack },
  { name: "Linear", icon: Zap },
  { name: "MCP servers", icon: Server },
];

export default function AssistantHomePage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const {
    accounts,
    connectedAccounts,
    erroredAccounts,
    summary,
    actionItems,
    fyi,
    draftsReady,
    suggestions,
    isLoading,
    accountsError,
    summaryError,
    draftsError,
    runsError,
  } = useAssistantHomeData(accountFilter);

  const attentionQuery = useQuery({
    queryKey: ["attention", accessToken],
    queryFn: () => getAttention(accessToken!),
    enabled: Boolean(accessToken),
  });
  const needsMe = attentionQuery.data?.counts?.total ?? 0;

  const refreshMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) return { queued: 0, failed: 0, total: 0 };
      const targets =
        accountFilter !== "all"
          ? [accountFilter]
          : connectedAccounts.map((a) => a.id);
      if (targets.length === 0) return { queued: 0, failed: 0, total: 0 };
      const results = await Promise.allSettled(targets.map((id) => refreshSummary(accessToken, id)));
      const queued = results.filter((r) => r.status === "fulfilled").length;
      return { queued, failed: results.length - queued, total: results.length };
    },
    onSuccess: (res) => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
      void queryClient.invalidateQueries({ queryKey: ["assistant-home", "runs"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
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
    onError: (err) => {
      toast({
        title: "Could not refresh summary",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const doneMutation = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return markActionItemDone(accessToken, id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions"] });
    },
    onError: (err) => {
      toast({
        title: "Could not mark action item done",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const dismissMutation = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return dismissFYI(accessToken, id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
    },
    onError: (err) => {
      toast({
        title: "Could not dismiss FYI",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="surface-card h-20 animate-pulse" />
        <div className="surface-card h-64 animate-pulse" />
      </div>
    );
  }

  if (accountsError) {
    return (
      <div className="surface-card space-y-3 px-4 py-5 text-sm text-destructive">
        <p>Could not load accounts.</p>
        <Button
          size="sm"
          variant="outline"
          onClick={() => {
            void queryClient.invalidateQueries({ queryKey: ["accounts"] });
          }}
        >
          Retry
        </Button>
      </div>
    );
  }

  if (accounts.length === 0 || connectedAccounts.length === 0) {
    return (
      <div className="mx-auto flex max-w-3xl flex-col items-center text-center">
        <div className="mb-4 grid h-12 w-12 place-items-center rounded-xl bg-secondary text-foreground">
          <Plug className="h-5 w-5" />
        </div>
        <h1 className="font-display text-3xl">Connect an email account to start</h1>
        <p className="mt-3 max-w-xl text-sm text-muted-foreground">
          The assistant needs at least one Microsoft mailbox. Connect Work or Personal Outlook in
          Accounts to generate summaries, action items, and drafts.
        </p>
        <div className="mt-6">
          <Button asChild>
            <Link to="/accounts">Open Accounts</Link>
          </Button>
        </div>
      </div>
    );
  }

  const actionPreview = actionItems.slice(0, 3);
  const fyiPreview = fyi.slice(0, 2);

  return (
    <div className="space-y-8">
      <section className="surface-card px-4 py-4">
        <div className="grid grid-cols-2 gap-3 md:grid-cols-6">
          <Metric label="Needs my input" value={String(needsMe)} />
          <Metric label="Action items" value={String(actionItems.length)} />
          <Metric label="FYI" value={String(fyi.length)} />
          <Metric label="Accounts" value={`${connectedAccounts.length}/${accounts.length}`} />
          <Metric
            label="Latest summary"
            value={summary?.snapshot ? relativeTime(summary.snapshot.created_at) : "never"}
          />
          {typeof draftsReady === "number" && <Metric label="Drafts ready" value={String(draftsReady)} />}
        </div>
        {needsMe > 0 && attentionQuery.data ? (
          <ul className="mt-4 space-y-2 border-t border-border/60 pt-3">
            {attentionQuery.data.items.slice(0, 5).map((item) => (
              <li key={item.id} className="text-sm">
                <Link
                  to={`/projects/${item.project_id}`}
                  className="hover:underline"
                >
                  <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
                    {item.why_me.replaceAll("_", " ")}
                  </span>
                  <span className="mt-0.5 block">
                    {item.title}
                    {item.project_name ? ` · ${item.project_name}` : ""}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : null}
      </section>

      {summaryError && (
        <div className="surface-card flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-sm text-destructive">
          <span className="inline-flex items-center gap-2">
            <AlertCircle className="h-4 w-4" />
            Could not load today's intelligence.
          </span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              void queryClient.invalidateQueries({ queryKey: ["summary"] });
            }}
          >
            Retry
          </Button>
        </div>
      )}

      {!summaryError && !summary?.snapshot && (
        <div className="surface-card flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-sm">
          <span className="text-muted-foreground">
            No summary generated yet for this scope.
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refreshMutation.mutate()}
            disabled={refreshMutation.isPending}
          >
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            Refresh summary
          </Button>
        </div>
      )}

      {(draftsError || runsError) && (
        <div className="surface-card flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground">
          <AlertCircle className="h-3.5 w-3.5" />
          Some secondary assistant signals are unavailable right now.
        </div>
      )}

      {actionItems.length > 0 && (
        <section className="surface-card border border-primary/20 px-4 py-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="text-sm font-medium">
                You have {actionItems.length} open action item{actionItems.length === 1 ? "" : "s"} across{" "}
                {new Set(actionItems.map((i) => i.account_id)).size} account
                {new Set(actionItems.map((i) => i.account_id)).size === 1 ? "" : "s"}.
              </p>
              <div className="mt-2 flex flex-wrap gap-2">
                {connectedAccounts.map((a) => {
                  const count = actionItems.filter((i) => i.account_id === a.id).length;
                  if (count === 0) return null;
                  return <AccountBadge key={a.id} account={a} className="rounded-full border px-2 py-1" />;
                })}
              </div>
            </div>
            <div className="flex gap-2">
              <Button asChild variant="outline" size="sm">
                <Link to="/today">Review in Today</Link>
              </Button>
              <Button asChild size="sm">
                <Link to="/inbox">Show source messages</Link>
              </Button>
            </div>
          </div>
        </section>
      )}

      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-display text-2xl">Suggestions</h2>
          <Button variant="outline" size="sm" onClick={() => refreshMutation.mutate()} disabled={refreshMutation.isPending}>
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            Refresh summary
          </Button>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          {suggestions.map((s) => (
            <Link key={s.id} to={s.href ?? "/today"} className="surface-card px-4 py-3 transition hover:border-foreground/30">
              <p className="text-sm font-medium">{s.title}</p>
              <p className="mt-1 text-xs text-muted-foreground">{s.description}</p>
            </Link>
          ))}
        </div>
      </section>

      <section className="space-y-3">
        <h2 className="font-display text-2xl">Action items</h2>
        {actionPreview.length === 0 ? (
          <div className="surface-card flex items-center gap-2 px-4 py-4 text-sm text-muted-foreground">
            <CheckCircle2 className="h-4 w-4 text-success" />
            Nothing pending right now.
          </div>
        ) : (
          <div className="space-y-2">
            {actionPreview.map((item) => {
              const account = accounts.find((a) => a.id === item.account_id);
              return (
                <div key={item.id} className="surface-card px-4 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm">{item.text}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <AccountBadge account={account} />
                        <Link
                          to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
                          className="text-xs text-primary underline underline-offset-2"
                        >
                          Message {item.message_id.slice(0, 8)}
                        </Link>
                        <span className="text-xs text-muted-foreground">Created {relativeTime(item.created_at)}</span>
                      </div>
                    </div>
                    <Button size="sm" variant="outline" onClick={() => doneMutation.mutate(item.id)}>
                      Done
                    </Button>
                  </div>
                </div>
              );
            })}
            <Button asChild variant="ghost" size="sm">
              <Link to="/today">View all action items</Link>
            </Button>
          </div>
        )}
      </section>

      <section className="space-y-3">
        <h2 className="font-display text-2xl">For your awareness</h2>
        {fyiPreview.length === 0 ? (
          <div className="surface-card px-4 py-4 text-sm text-muted-foreground">No FYI items right now.</div>
        ) : (
          <div className="space-y-2">
            {fyiPreview.map((item) => {
              const account = accounts.find((a) => a.id === item.account_id);
              return (
                <div key={item.id} className="surface-card px-4 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm">{item.text}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-2">
                        <AccountBadge account={account} />
                        <Link
                          to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
                          className="text-xs text-primary underline underline-offset-2"
                        >
                          Message {item.message_id.slice(0, 8)}
                        </Link>
                        <span className="text-xs text-muted-foreground">Created {relativeTime(item.created_at)}</span>
                      </div>
                    </div>
                    <Button size="sm" variant="outline" onClick={() => dismissMutation.mutate(item.id)}>
                      Dismiss
                    </Button>
                  </div>
                </div>
              );
            })}
            <Button asChild variant="ghost" size="sm">
              <Link to="/today">View all FYI</Link>
            </Button>
          </div>
        )}
      </section>

      <section>
        <h2 className="mb-2 text-xs uppercase tracking-widest text-muted-foreground">Connectors</h2>
        <div className="flex flex-wrap gap-2">
          {CONNECTOR_DEFS.map((connector) => {
            const isEmail = connector.name === "Email";
            const emailConnected = connectedAccounts.length > 0;
            const emailAttention = erroredAccounts.length > 0;
            const label = isEmail
              ? emailConnected
                ? emailAttention
                  ? "attention"
                  : "connected"
                : "needs connection"
              : "soon";
            return (
              <span
                key={connector.name}
                className="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px]"
              >
                <connector.icon className="h-3 w-3" />
                {connector.name}
                <span
                  className={
                    label === "connected"
                      ? "uppercase tracking-wider text-success"
                      : label === "attention"
                        ? "uppercase tracking-wider text-destructive"
                        : "uppercase tracking-wider text-muted-foreground"
                  }
                >
                  {label === "soon" ? "coming soon" : label}
                </span>
              </span>
            );
          })}
        </div>
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-[10px] uppercase tracking-widest text-muted-foreground">{label}</p>
      <p className="mt-1 font-display text-2xl">{value}</p>
    </div>
  );
}
