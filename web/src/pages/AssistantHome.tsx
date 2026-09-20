import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  FolderKanban,
  Inbox,
  PenLine,
  Plug,
} from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime } from "@/lib/accounts";
import {
  ApiError,
  askAcross,
  getApiHealth,
  getAttention,
  getOverview,
  listActivity,
  markActionItemDone,
  type ActivityItem,
  type AskAnswer,
  type AskCitation,
  type OverviewProject,
} from "@/lib/auth";
import { activityHref, activityLabel, groupActivityByDay, isAdverse } from "@/lib/activity";
import { mergeNeedsMeRows } from "@/lib/needsMe";
import { toast } from "@/hooks/use-toast";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";
import { useState } from "react";
import { unassignedSummaryQueryOptions } from "@/lib/triage";

type Props = {
  accountFilter: AccountFilter;
};

/** How many attention rows Home shows before asking to expand. */
const NEEDS_ME_PREVIEW = 5;

function citationHref(c: AskCitation): string | undefined {
  if (!c.project_id) return undefined;
  if (c.type === "issue") return `/projects/${c.project_id}/issues/${c.id}`;
  if (c.type === "fact_version" || c.type === "decision") {
    return `/projects/${c.project_id}?mode=position`;
  }
  return `/projects/${c.project_id}`;
}

export default function AssistantHomePage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [askQuestion, setAskQuestion] = useState("");
  const [askAnswer, setAskAnswer] = useState<AskAnswer | null>(null);
  const {
    connectedAccounts,
    fyi,
    draftsReady,
    accountsError,
  } = useAssistantHomeData(accountFilter);

  const attentionQuery = useQuery({
    queryKey: ["attention", accessToken],
    queryFn: () => getAttention(accessToken!),
    enabled: Boolean(accessToken),
  });
  // One request for counts and projects. This page used to list projects and
  // then fan out a current-position request per project from the browser.
  const overviewQuery = useQuery({
    queryKey: ["overview", accessToken],
    queryFn: () => getOverview(accessToken!),
    enabled: Boolean(accessToken),
    ...unassignedSummaryQueryOptions,
  });
  const activityQuery = useQuery({
    queryKey: ["activity", accessToken],
    queryFn: () => listActivity(accessToken!, { limit: 25 }),
    enabled: Boolean(accessToken),
  });
  const healthQuery = useQuery({
    queryKey: ["api-health"],
    queryFn: () => getApiHealth(),
    staleTime: 60_000,
  });
  const llmEnabled = healthQuery.data?.llm === true;

  const counts = overviewQuery.data?.counts;
  const recentProjects = overviewQuery.data?.projects ?? [];
  const needsMe = mergeNeedsMeRows(attentionQuery.data?.items ?? []);
  // Long attention lists push everything else off the page; show a working set
  // and let the operator open the rest.
  const [showAllNeedsMe, setShowAllNeedsMe] = useState(false);
  const visibleNeedsMe = showAllNeedsMe ? needsMe : needsMe.slice(0, NEEDS_ME_PREVIEW);
  const activityGroups = groupActivityByDay(activityQuery.data?.items ?? []);
  const triageCount = (counts?.triage_unassigned ?? 0) + (counts?.triage_provisional ?? 0);

  const doneMutation = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return markActionItemDone(accessToken, id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["attention"] });
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

  const askMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return askAcross(accessToken, askQuestion.trim());
    },
    onSuccess: (res) => {
      setAskAnswer(res);
    },
    onError: (err) => {
      toast({
        title: "Ask failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const loading = Boolean(accessToken) && (attentionQuery.isLoading || overviewQuery.isLoading);

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
            Across your projects
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            What needs you, what changed, and where things stand.
          </p>
        </div>

        <nav aria-label="Overview" className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          <MetricCard label="Needs you" value={counts?.needs_you ?? 0} to="#home-needs-list" />
          <MetricCard label="Triage" value={triageCount} to="/triage" />
          <MetricCard
            label="Contradictions"
            value={counts?.open_contradictions ?? 0}
            to="/projects"
            adverse
          />
          <MetricCard
            label="Unconfirmed"
            value={(counts?.provisional_facts ?? 0) + (counts?.proposed_decisions ?? 0)}
            to="/projects"
          />
          <MetricCard label="Projects" value={counts?.active_projects ?? 0} to="/projects" />
        </nav>
      </section>

      <AskAcrossProjects
        llmEnabled={llmEnabled}
        question={askQuestion}
        setQuestion={setAskQuestion}
        answer={askAnswer}
        pending={askMutation.isPending}
        onAsk={() => askMutation.mutate()}
      />

      <section className="space-y-4" aria-labelledby="home-actions-heading">
        <h2 id="home-actions-heading" className="font-display text-2xl">
          Needs you
        </h2>
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
          <>
          <ul id="home-needs-list" className="divide-y divide-border/70 border-y border-border/70">
            {visibleNeedsMe.map((row) => (
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
          {needsMe.length > NEEDS_ME_PREVIEW ? (
            <Button
              variant="ghost"
              size="sm"
              aria-expanded={showAllNeedsMe}
              onClick={() => setShowAllNeedsMe((v) => !v)}
            >
              {showAllNeedsMe
                ? "Show fewer"
                : `Show all ${needsMe.length}`}
            </Button>
          ) : null}
          </>
        )}
      </section>

      <section className="space-y-4" aria-labelledby="home-changed-heading">
        <div className="flex items-end justify-between gap-3">
          <h2 id="home-changed-heading" className="font-display text-2xl">
            What changed
          </h2>
        </div>
        {activityQuery.isError ? (
          <p className="text-sm text-destructive">Could not load recent activity.</p>
        ) : activityGroups.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No recorded changes yet. Decisions, facts and issues appear here as they move.
          </p>
        ) : (
          <div className="space-y-6">
            {activityGroups.map((group) => (
              <div key={group.key} className="space-y-2">
                <h3 className="text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
                  {group.label}
                </h3>
                <ul className="divide-y divide-border/70 border-y border-border/70">
                  {group.items.map((item) => (
                    <ActivityRow key={`${item.kind}:${item.ref_id}`} item={item} />
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-4" aria-labelledby="home-recent-heading">
        <div className="flex items-end justify-between gap-3">
          <h2 id="home-recent-heading" className="font-display text-2xl">
            Projects
          </h2>
          <Button asChild variant="ghost" size="sm">
            <Link to="/projects">All projects</Link>
          </Button>
        </div>
        {overviewQuery.isError ? (
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
            {recentProjects.map((project) => (
              <RecentProjectRow key={project.id} project={project} />
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

function AskAcrossProjects({
  llmEnabled,
  question,
  setQuestion,
  answer,
  pending,
  onAsk,
}: {
  llmEnabled: boolean;
  question: string;
  setQuestion: (v: string) => void;
  answer: AskAnswer | null;
  pending: boolean;
  onAsk: () => void;
}) {
  return (
    <section aria-label="Ask across projects" className="space-y-3">
      <div className="flex flex-col gap-2 sm:flex-row">
        <Input
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          placeholder="Ask across projects…"
          aria-label="Ask across projects"
          disabled={!llmEnabled || pending}
          onKeyDown={(e) => {
            if (e.key === "Enter" && question.trim()) onAsk();
          }}
        />
        <Button
          variant="outline"
          disabled={!llmEnabled || !question.trim() || pending}
          title={
            llmEnabled
              ? "Answer from structured project state across your projects"
              : "Configure LLM_BASE_URL and LLM_MODEL on the API"
          }
          onClick={onAsk}
        >
          {pending ? "Asking…" : llmEnabled ? "Ask" : "Ask (LLM off)"}
        </Button>
      </div>
      {answer ? (
        <div className="space-y-2 border-t border-border/70 pt-3 text-sm">
          <p>{answer.answer}</p>
          {answer.citations.length > 0 ? (
            <ul className="space-y-1 text-xs text-muted-foreground">
              {answer.citations.map((c) => {
                const href = citationHref(c);
                const label = [c.project_code, c.type, c.id.slice(0, 8)]
                  .filter(Boolean)
                  .join(" · ");
                return (
                  <li key={`${c.type}:${c.id}`}>
                    {href ? (
                      <Link to={href} className="hover:underline">
                        {label}
                      </Link>
                    ) : (
                      label
                    )}
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className="text-xs text-muted-foreground">No citations returned.</p>
          )}
        </div>
      ) : null}
    </section>
  );
}

function MetricCard({
  label,
  value,
  to,
  adverse,
}: {
  label: string;
  value: number;
  to: string;
  adverse?: boolean;
}) {
  // Zero renders muted rather than hiding the card: a row that changes shape
  // between loads teaches people not to trust it.
  const highlight = adverse && value > 0;
  return (
    <Link
      to={to}
      aria-label={`${label}, ${value}`}
      className="surface-card px-4 py-3 transition hover:border-foreground/30"
    >
      <p className="text-[10px] uppercase tracking-widest text-muted-foreground">{label}</p>
      <p
        className={`mt-1 flex items-center gap-1.5 font-display text-2xl ${
          value === 0 ? "text-muted-foreground" : ""
        } ${highlight ? "text-destructive" : ""}`}
      >
        {highlight ? <AlertTriangle className="h-4 w-4" aria-hidden="true" /> : null}
        {value}
      </p>
    </Link>
  );
}

function ActivityRow({ item }: { item: ActivityItem }) {
  const at = new Date(item.occurred_at);
  return (
    <li className="flex items-start gap-3 py-3">
      <div className="min-w-0 flex-1 space-y-1">
        <p className="flex flex-wrap items-center gap-1.5 text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
          <span className={isAdverse(item.kind) ? "text-destructive" : ""}>
            {activityLabel(item.kind)}
          </span>
          <span aria-hidden="true">·</span>
          <span className="font-mono normal-case tracking-normal">{item.project_code}</span>
          {item.source ? (
            <span className="rounded bg-muted px-1.5 py-0.5 tracking-normal">{item.source}</span>
          ) : null}
        </p>
        <Link to={activityHref(item)} className="block text-sm font-medium hover:underline">
          {item.title}
        </Link>
      </div>
      <time
        className="shrink-0 text-xs text-muted-foreground"
        dateTime={item.occurred_at}
        title={at.toLocaleString()}
      >
        {relativeTime(item.occurred_at)}
      </time>
    </li>
  );
}

function RecentProjectRow({ project }: { project: OverviewProject }) {
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
          {project.attention_count > 0 ? (
            <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-foreground">
              {project.attention_count} needs you
            </span>
          ) : null}
          <span className="text-xs text-muted-foreground">
            {project.last_activity_at
              ? `Active ${relativeTime(project.last_activity_at)}`
              : "No activity yet"}
          </span>
        </div>
        {project.teaser ? (
          <p className="text-xs text-muted-foreground line-clamp-2">{project.teaser}</p>
        ) : (
          <p className="text-xs text-muted-foreground">No current position yet.</p>
        )}
      </Link>
    </li>
  );
}
