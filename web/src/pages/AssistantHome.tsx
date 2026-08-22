import { Link } from "react-router-dom";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, FolderKanban, Inbox, Loader2, PenLine, Plug } from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime } from "@/lib/accounts";
import {
  getAttention,
  getCurrentPosition,
  getUnassignedSummary,
  listProjects,
  markActionItemDone,
  type ProjectListItem,
} from "@/lib/auth";
import { mergeNeedsMeRows } from "@/lib/needsMe";
import { toast } from "@/hooks/use-toast";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";

type Props = {
  accountFilter: AccountFilter;
};

export default function AssistantHomePage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const {
    connectedAccounts,
    actionItems,
    fyi,
    draftsReady,
    isLoading: mailLoading,
    accountsError,
  } = useAssistantHomeData(accountFilter);

  const attentionQuery = useQuery({
    queryKey: ["attention", accessToken],
    queryFn: () => getAttention(accessToken!),
    enabled: Boolean(accessToken),
  });
  const projectsQuery = useQuery({
    queryKey: ["projects", accessToken],
    queryFn: () => listProjects(accessToken!),
    enabled: Boolean(accessToken),
  });
  const triageQuery = useQuery({
    queryKey: ["unassigned-summary", accessToken],
    queryFn: () => getUnassignedSummary(accessToken!),
    enabled: Boolean(accessToken),
  });

  const recentProjects = (projectsQuery.data ?? [])
    .slice()
    .sort((a, b) => b.updated_at.localeCompare(a.updated_at))
    .slice(0, 5);

  const positionQueries = useQueries({
    queries: recentProjects.map((p) => ({
      queryKey: ["current-position", accessToken, p.id],
      queryFn: () => getCurrentPosition(accessToken!, p.id),
      enabled: Boolean(accessToken) && recentProjects.length > 0,
      staleTime: 60_000,
    })),
  });

  const needsMe = mergeNeedsMeRows(attentionQuery.data?.items ?? [], actionItems);
  const triageCount =
    (triageQuery.data?.unassigned ?? 0) + (triageQuery.data?.provisional ?? 0);

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

  const loading =
    Boolean(accessToken) &&
    (attentionQuery.isLoading || projectsQuery.isLoading || mailLoading);

  if (loading) {
    return (
      <div className="space-y-4" role="status" aria-label="Loading home">
        <div className="h-16 animate-pulse rounded-md bg-muted/60" />
        <div className="h-48 animate-pulse rounded-md bg-muted/60" />
      </div>
    );
  }

  if (accountsError) {
    return (
      <div className="space-y-3 rounded-md border border-destructive/30 px-4 py-5 text-sm text-destructive">
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

  return (
    <div className="space-y-12">
      <section className="space-y-6" aria-labelledby="home-needs-heading">
        <div className="space-y-2">
          <p className="font-display text-sm tracking-wide text-muted-foreground">Automata</p>
          <h1 id="home-needs-heading" className="font-display text-3xl md:text-4xl font-medium leading-tight">
            Needs my input
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            What you should decide, confirm, or clear — across projects and mail.
          </p>
        </div>

        {needsMe.length === 0 ? (
          <div className="space-y-4 border-y border-border/70 py-8">
            <div className="flex items-start gap-3 text-sm text-muted-foreground">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" />
              <p>Nothing waiting on you right now.</p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button asChild variant="outline" size="sm">
                <Link to="/projects">
                  <FolderKanban className="mr-1.5 h-3.5 w-3.5" />
                  Projects
                </Link>
              </Button>
              <Button asChild variant="outline" size="sm">
                <Link to="/triage">
                  <Inbox className="mr-1.5 h-3.5 w-3.5" />
                  Triage
                </Link>
              </Button>
              {connectedAccounts.length === 0 && (
                <Button asChild size="sm">
                  <Link to="/accounts">
                    <Plug className="mr-1.5 h-3.5 w-3.5" />
                    Connect email
                  </Link>
                </Button>
              )}
            </div>
          </div>
        ) : (
          <ul className="divide-y divide-border/70 border-y border-border/70">
            {needsMe.map((row) => (
              <li key={row.id} className="flex items-start gap-3 py-3.5">
                <div className="min-w-0 flex-1">
                  <p className="text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
                    {row.whyMeLabel}
                    {row.projectLabel ? ` · ${row.projectLabel}` : ""}
                  </p>
                  <Link to={row.href} className="mt-1 block text-sm font-medium hover:underline">
                    {row.title}
                  </Link>
                </div>
                {row.kind === "mail" && row.mailActionId ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => doneMutation.mutate(row.mailActionId!)}
                    disabled={doneMutation.isPending}
                  >
                    Done
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="space-y-4" aria-labelledby="home-recent-heading">
        <div className="flex items-end justify-between gap-3">
          <h2 id="home-recent-heading" className="font-display text-2xl">
            Recent projects
          </h2>
          <Button asChild variant="ghost" size="sm">
            <Link to="/projects">All projects</Link>
          </Button>
        </div>
        {projectsQuery.isError ? (
          <p className="text-sm text-destructive">Could not load projects.</p>
        ) : recentProjects.length === 0 ? (
          <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
            <span>No projects yet.</span>
            <Button asChild size="sm">
              <Link to="/projects">Create a project</Link>
            </Button>
          </div>
        ) : (
          <ul className="space-y-3">
            {recentProjects.map((project, index) => (
              <RecentProjectRow
                key={project.id}
                project={project}
                teaser={positionTeaser(positionQueries[index]?.data)}
                loading={positionQueries[index]?.isLoading}
              />
            ))}
          </ul>
        )}
      </section>

      <section className="grid gap-6 sm:grid-cols-2" aria-label="Queues and channel pulse">
        <div className="space-y-2">
          <h2 className="font-display text-xl">Triage</h2>
          <p className="text-sm text-muted-foreground">
            {triageCount === 0
              ? "Filing queue is clear."
              : `${triageCount} item${triageCount === 1 ? "" : "s"} waiting to be assigned.`}
          </p>
          <Button asChild variant="outline" size="sm">
            <Link to="/triage">Open triage</Link>
          </Button>
        </div>
        <div className="space-y-2">
          <h2 className="font-display text-xl">Channel pulse</h2>
          <ul className="space-y-1.5 text-sm text-muted-foreground">
            <li>
              {typeof draftsReady === "number" && draftsReady > 0 ? (
                <Link to="/drafts" className="inline-flex items-center gap-1.5 text-foreground hover:underline">
                  <PenLine className="h-3.5 w-3.5" />
                  {draftsReady} draft{draftsReady === 1 ? "" : "s"} ready
                </Link>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <PenLine className="h-3.5 w-3.5" />
                  No drafts waiting
                </span>
              )}
            </li>
            {fyi.length > 0 && (
              <li>
                {fyi.length} FYI item{fyi.length === 1 ? "" : "s"} from mail summaries
              </li>
            )}
            {connectedAccounts.length === 0 && (
              <li>
                <Link to="/accounts" className="text-foreground hover:underline">
                  Connect email under Connectors
                </Link>
              </li>
            )}
          </ul>
        </div>
      </section>
    </div>
  );
}

function positionTeaser(
  position: { facts: { label: string; value_text: string }[]; decisions: { statement: string }[] } | undefined,
): string | undefined {
  if (!position) return undefined;
  const fact = position.facts[0];
  if (fact) {
    const value = fact.value_text?.trim();
    return value ? `${fact.label}: ${value}` : fact.label;
  }
  const decision = position.decisions[0];
  if (decision?.statement) {
    const s = decision.statement.trim();
    return s.length > 90 ? `${s.slice(0, 87)}…` : s;
  }
  return undefined;
}

function RecentProjectRow({
  project,
  teaser,
  loading,
}: {
  project: ProjectListItem;
  teaser?: string;
  loading?: boolean;
}) {
  return (
    <li>
      <Link
        to={`/projects/${project.id}`}
        className="block space-y-1 transition hover:text-foreground"
      >
        <div className="flex flex-wrap items-baseline gap-2">
          <span className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
            {project.code}
          </span>
          <span className="text-sm font-medium">{project.name}</span>
          <span className="text-xs text-muted-foreground">
            Updated {relativeTime(project.updated_at)}
          </span>
        </div>
        {loading ? (
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" />
            Loading position…
          </p>
        ) : teaser ? (
          <p className="text-xs text-muted-foreground line-clamp-2">{teaser}</p>
        ) : (
          <p className="text-xs text-muted-foreground">No current position yet.</p>
        )}
      </Link>
    </li>
  );
}
