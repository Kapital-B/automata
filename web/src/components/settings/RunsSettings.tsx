import { useState } from "react";
import { Link } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Ban, CheckCircle2, ChevronDown, Clock, History, Loader2, XCircle } from "lucide-react";
import { AccountBadge } from "@/components/AccountBadge";
import { ListSkeleton } from "@/components/ListSkeleton";
import { Button } from "@/components/ui/button";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { toast } from "@/hooks/use-toast";
import { relativeTime, type UiAccount } from "@/lib/accounts";
import { cn } from "@/lib/utils";
import { ApiError, cancelRun, listRuns, type JobRun } from "@/lib/auth";
import { isActiveRun, jobTypeLabel, jobTypeOptions, runDetails, runDuration, runSummary } from "@/lib/runs";

interface Props {
  accountFilter: AccountFilter;
}

const statusStyle: Record<JobRun["status"], { label: string; icon: typeof Clock; className: string }> = {
  success: { label: "Succeeded", icon: CheckCircle2, className: "bg-success/10 text-success" },
  failed: { label: "Failed", icon: XCircle, className: "bg-destructive/10 text-destructive" },
  running: { label: "Running", icon: Loader2, className: "bg-accent/10 text-accent" },
  pending: { label: "Queued", icon: Clock, className: "bg-muted text-muted-foreground" },
  cancelled: { label: "Cancelled", icon: Ban, className: "bg-muted text-muted-foreground" },
};

function StatusPill({ status }: { status: JobRun["status"] }) {
  const s = statusStyle[status] ?? statusStyle.pending;
  const Icon = s.icon;
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium", s.className)}>
      <Icon
        aria-hidden="true"
        className={cn("h-3 w-3", status === "running" && "animate-spin motion-reduce:animate-none")}
      />
      {s.label}
    </span>
  );
}

const selectClass =
  "h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-56";

/** The Runs tab in Settings: the audit trail of background jobs. */
export function RunsSettings({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const [jobType, setJobType] = useState("");

  const runsQuery = useInfiniteQuery({
    queryKey: ["runs", accessToken, accountFilter, jobType],
    queryFn: ({ pageParam }) =>
      listRuns(accessToken!, {
        accountId: accountFilter === "all" ? undefined : accountFilter,
        jobType: jobType || undefined,
        cursor: pageParam,
        limit: 50,
      }),
    initialPageParam: undefined as string | undefined,
    enabled: Boolean(accessToken),
    getNextPageParam: (lastPage) => lastPage.nextCursor ?? undefined,
    // Poll only while something is in flight.
    refetchInterval: (query) =>
      (query.state.data?.pages ?? []).some((p) => p.runs.some(isActiveRun)) ? 2000 : false,
  });

  const cancel = useMutation({
    mutationFn: async (runID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      await cancelRun(accessToken, runID);
    },
    onSuccess: async () => {
      toast({ title: "Cancel requested", description: "The run stops after the step it is on." });
      await queryClient.invalidateQueries({ queryKey: ["runs"] });
    },
    onError: (error) =>
      toast({
        title: "Could not cancel the run",
        description: error instanceof ApiError ? error.message : "Please try again.",
        variant: "destructive",
      }),
  });

  const runs =
    runsQuery.data?.pages
      .flatMap((page) => page.runs)
      .filter((r) => accountFilter === "all" || r.account_id === accountFilter || !r.account_id) ?? [];
  const getAccount = (id?: string) => accounts.find((a) => a.id === id);

  return (
    <section aria-labelledby="runs-heading" className="space-y-4">
      <div className="space-y-1">
        <h2 id="runs-heading" className="font-display text-2xl font-medium">
          Job runs
        </h2>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Everything Automata did in the background: syncs, categorising, summaries, forwarding and drafts, with what
          each run found.
        </p>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <label htmlFor="runs-job-type" className="sr-only">
            Job type
          </label>
          <select id="runs-job-type" className={selectClass} value={jobType} onChange={(e) => setJobType(e.target.value)}>
            <option value="">All jobs</option>
            {jobTypeOptions().map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <p className="text-sm text-muted-foreground">
          When jobs run on their own is set under{" "}
          <Link to="/settings?tab=schedules" className="font-medium text-primary underline-offset-4 hover:underline">
            Schedules
          </Link>
          .
        </p>
      </div>

      {runsQuery.isLoading ? (
        <ListSkeleton label="Loading runs" />
      ) : runsQuery.isError ? (
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          Could not load runs: {runsQuery.error instanceof Error ? runsQuery.error.message : "unknown error"}
        </div>
      ) : runs.length === 0 ? (
        <div className="surface-card flex flex-col items-center gap-3 px-6 py-12 text-center">
          <History aria-hidden="true" className="h-8 w-8 text-muted-foreground" />
          <p className="font-medium">{jobType ? `No ${jobTypeLabel(jobType).toLowerCase()} runs yet` : "No runs yet"}</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            Runs appear here when you sync a mailbox, run rules, or a schedule fires.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          <ul aria-label="Runs" className="surface-card divide-y divide-border/70 overflow-hidden">
            {runs.map((r) => (
              <RunRow
                key={r.id}
                run={r}
                account={getAccount(r.account_id)}
                cancelling={cancel.isPending && cancel.variables === r.id}
                onCancel={() => cancel.mutate(r.id)}
              />
            ))}
          </ul>
          {(runsQuery.hasNextPage || runsQuery.isFetchingNextPage || runsQuery.isFetchNextPageError) && (
            <div className="flex flex-col items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={!runsQuery.hasNextPage || runsQuery.isFetchingNextPage}
                onClick={() => void runsQuery.fetchNextPage()}
              >
                {runsQuery.isFetchingNextPage && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
                {runsQuery.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
              {runsQuery.isFetchNextPageError && (
                <p role="alert" className="text-xs text-destructive">
                  Could not load more runs. Try again.
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function RunRow({
  run,
  account,
  cancelling,
  onCancel,
}: {
  run: JobRun;
  account: UiAccount | undefined;
  cancelling: boolean;
  onCancel: () => void;
}) {
  const [open, setOpen] = useState(false);
  const summary = runSummary(run);
  const duration = runDuration(run);
  const details = runDetails(run);
  const detailsID = `run-details-${run.id}`;

  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <p className="font-medium">{jobTypeLabel(run.job_type)}</p>
            {run.account_id && <AccountBadge account={account} />}
            <span className="text-xs text-muted-foreground">{run.trigger === "schedule" ? "Scheduled" : "Manual"}</span>
          </div>
          {run.status === "failed" && run.error_message ? (
            <p className="line-clamp-2 text-sm text-destructive">{run.error_message}</p>
          ) : summary ? (
            <p className="text-sm text-muted-foreground">{summary}</p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-3">
          <div className="text-right text-xs text-muted-foreground">
            <p>
              {run.started_at ? (
                <time dateTime={run.started_at} title={new Date(run.started_at).toLocaleString()}>
                  {relativeTime(run.started_at)}
                </time>
              ) : (
                "Not started"
              )}
            </p>
            {duration && <p>{isActiveRun(run) ? `for ${duration}` : `took ${duration}`}</p>}
          </div>
          <StatusPill status={run.status} />
        </div>
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-2">
        {(details.length > 0 || run.error_message) && (
          <Button
            size="sm"
            variant="ghost"
            className="h-8 px-2 text-muted-foreground"
            aria-expanded={open}
            aria-controls={detailsID}
            onClick={() => setOpen((v) => !v)}
          >
            <ChevronDown
              aria-hidden="true"
              className={cn("mr-1 h-3.5 w-3.5 transition-transform motion-reduce:transition-none", open && "rotate-180")}
            />
            Details
          </Button>
        )}
        {isActiveRun(run) && (
          <Button size="sm" variant="outline" className="h-8" disabled={cancelling} onClick={onCancel}>
            {cancelling && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {cancelling ? "Cancelling…" : "Cancel"}
          </Button>
        )}
      </div>

      {open && (
        <div id={detailsID} className="mt-2 space-y-3 rounded-md bg-muted/40 px-3 py-3 text-sm">
          {run.error_message && (
            <p className="whitespace-pre-wrap break-words text-destructive">{run.error_message}</p>
          )}
          <dl className="grid gap-x-6 gap-y-1 sm:grid-cols-2">
            {run.started_at && <Detail label="Started" value={new Date(run.started_at).toLocaleString()} />}
            {run.finished_at && <Detail label="Finished" value={new Date(run.finished_at).toLocaleString()} />}
            {details.map((d) => (
              <Detail key={d.label} label={d.label} value={d.value} />
            ))}
            <Detail label="Run ID" value={run.id} mono />
          </dl>
        </div>
      )}
    </li>
  );
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex min-w-0 gap-2">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 truncate", mono && "font-mono text-xs leading-5")}>{value}</dd>
    </div>
  );
}
