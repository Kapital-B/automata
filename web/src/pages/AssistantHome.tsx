import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleDot,
  FileCheck2,
  FolderKanban,
  History,
  Inbox,
  Info,
  ListTodo,
  Loader2,
  Mail,
  PenLine,
  Plug,
  Scale,
  Sparkles,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";
import { PageHeader } from "@/components/PageHeader";
import { ListSkeleton } from "@/components/ListSkeleton";
import { Tag } from "@/components/ConnectionCard";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime } from "@/lib/accounts";
import {
  ApiError,
  askAcross,
  getApiHealth,
  getAttention,
  getOverview,
  listActivity,
  dismissFYI,
  markActionItemDone,
  type ActivityItem,
  type AskAnswer,
  type AskCitation,
  type OverviewProject,
  type SummaryFYI,
} from "@/lib/auth";
import { activityHref, activityLabel, groupActivityByDay, isAdverse } from "@/lib/activity";
import { mergeNeedsMeRows, type NeedsMeRow } from "@/lib/needsMe";
import { inboxHref } from "@/lib/todos";
import { TodoItem } from "@/components/TodoItem";
import { toast } from "@/hooks/use-toast";
import { useAssistantHomeData } from "@/hooks/useAssistantHomeData";
import { useState } from "react";
import { unassignedSummaryQueryOptions } from "@/lib/triage";

type Props = {
  accountFilter: AccountFilter;
};

/** How many attention rows Home shows before asking to expand. */
const NEEDS_ME_PREVIEW = 5;
/** How many FYIs Home shows before asking to expand. */
const FYI_PREVIEW = 4;

function citationHref(c: AskCitation): string | undefined {
  if (!c.project_id) return undefined;
  if (c.type === "issue") return `/projects/${c.project_id}/issues/${c.id}`;
  if (c.type === "fact_version" || c.type === "decision") {
    return `/projects/${c.project_id}#position`;
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
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
      void queryClient.invalidateQueries({ queryKey: ["project-todos"] });
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions"] });
    },
    onError: (err) => {
      toast({
        title: "Could not mark the to-do done",
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
      <div className="space-y-8" role="status" aria-label="Loading home">
        <div className="space-y-2 border-b border-border/70 pb-6">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-9 w-72" />
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {Array.from({ length: 5 }, (_, i) => (
            <Skeleton key={i} className="h-[4.5rem] rounded-lg" />
          ))}
        </div>
        <ListSkeleton label="Loading what needs you" rows={4} />
      </div>
    );
  }

  if (accountsError) {
    return (
      <div role="alert" className="surface-card space-y-3 p-5 text-sm text-destructive">
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
    <div className="space-y-10">
      <div className="space-y-6">
        <PageHeader
          eyebrow="Automata"
          title="Across your projects"
          description="What needs you, what changed, and where things stand."
        />

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
      </div>

      <AskAcrossProjects
        llmEnabled={llmEnabled}
        question={askQuestion}
        setQuestion={setAskQuestion}
        answer={askAnswer}
        pending={askMutation.isPending}
        onAsk={() => askMutation.mutate()}
      />

      <section className="space-y-3" aria-labelledby="home-actions-heading">
        <SectionHeading
          id="home-actions-heading"
          title="Needs you"
          description="Proposals to confirm, disagreements to settle, and to-dos from your mail."
        />
        {needsMe.length === 0 ? (
          <div className="surface-card flex flex-col items-center gap-3 px-6 py-10 text-center">
            <CheckCircle2 aria-hidden="true" className="h-8 w-8 text-success" />
            <p className="text-sm text-muted-foreground">Nothing waiting on you right now.</p>
            <div className="flex flex-wrap justify-center gap-2">
              <Button asChild variant="outline" size="sm">
                <Link to="/projects">
                  <FolderKanban aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                  Projects
                </Link>
              </Button>
              <Button asChild variant="outline" size="sm">
                <Link to="/triage">
                  <Inbox aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                  Triage
                </Link>
              </Button>
              {connectedAccounts.length === 0 && (
                <Button asChild size="sm">
                  <Link to="/accounts">
                    <Plug aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                    Connect email
                  </Link>
                </Button>
              )}
            </div>
          </div>
        ) : (
          <>
            <ul
              id="home-needs-list"
              aria-label="Needs you"
              className="surface-card scroll-mt-20 divide-y divide-border/70 overflow-hidden"
            >
              {visibleNeedsMe.map((row) =>
                row.kind === "mail" && row.mailActionId ? (
                  <TodoItem
                    key={row.id}
                    text={row.title}
                    href={row.href}
                    label={row.whyMeLabel}
                    projectLabel={row.projectLabel}
                    issueTitle={row.issueTitle}
                    issueHref={row.issueHref}
                    dueAt={row.dueAt}
                    pending={doneMutation.isPending}
                    onDone={() => doneMutation.mutate(row.mailActionId!)}
                  />
                ) : (
                  <NeedsMeItem key={row.id} row={row} />
                ),
              )}
            </ul>
            {needsMe.length > NEEDS_ME_PREVIEW ? (
              <Button
                variant="ghost"
                size="sm"
                className="text-muted-foreground"
                aria-expanded={showAllNeedsMe}
                aria-controls="home-needs-list"
                onClick={() => setShowAllNeedsMe((v) => !v)}
              >
                <ChevronDown
                  aria-hidden="true"
                  className={cn(
                    "mr-1.5 h-3.5 w-3.5 transition-transform motion-reduce:transition-none",
                    showAllNeedsMe && "rotate-180",
                  )}
                />
                {showAllNeedsMe ? "Show fewer" : `Show all ${needsMe.length}`}
              </Button>
            ) : null}
          </>
        )}
      </section>

      <div className="grid gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,22rem)]">
        <section className="space-y-3" aria-labelledby="home-changed-heading">
          <SectionHeading
            id="home-changed-heading"
            title="What changed"
            description="Decisions, facts and issues as they move."
          />
          {activityQuery.isLoading ? (
            <ListSkeleton label="Loading recent activity" rows={4} />
          ) : activityQuery.isError ? (
            <div role="alert" className="surface-card p-5 text-sm text-destructive">
              Could not load recent activity.
            </div>
          ) : activityGroups.length === 0 ? (
            <div className="surface-card flex flex-col items-center gap-2 px-6 py-10 text-center">
              <History aria-hidden="true" className="h-7 w-7 text-muted-foreground" />
              <p className="max-w-sm text-sm text-muted-foreground">
                No recorded changes yet. Decisions, facts and issues appear here as they move.
              </p>
            </div>
          ) : (
            <div className="space-y-5">
              {activityGroups.map((group) => (
                <div key={group.key} className="space-y-2">
                  <h3 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                    {group.label}
                  </h3>
                  <ul
                    aria-label={`Changes ${group.label.toLowerCase()}`}
                    className="surface-card divide-y divide-border/70 overflow-hidden"
                  >
                    {group.items.map((item) => (
                      <ActivityRow key={`${item.kind}:${item.ref_id}`} item={item} />
                    ))}
                  </ul>
                </div>
              ))}
            </div>
          )}
        </section>

        <div className="space-y-10">
          <section className="space-y-3" aria-labelledby="home-recent-heading">
            <div className="flex items-end justify-between gap-3">
              <SectionHeading id="home-recent-heading" title="Projects" />
              <Button asChild variant="ghost" size="sm" className="text-muted-foreground">
                <Link to="/projects">
                  All projects
                  <ChevronRight aria-hidden="true" className="ml-1 h-3.5 w-3.5" />
                </Link>
              </Button>
            </div>
            {overviewQuery.isError ? (
              <div role="alert" className="surface-card p-5 text-sm text-destructive">
                Could not load projects.
              </div>
            ) : recentProjects.length === 0 ? (
              <div className="surface-card flex flex-col items-center gap-3 px-6 py-10 text-center">
                <FolderKanban aria-hidden="true" className="h-7 w-7 text-muted-foreground" />
                <p className="text-sm text-muted-foreground">No projects yet.</p>
                <Button asChild size="sm">
                  <Link to="/projects">Create a project</Link>
                </Button>
              </div>
            ) : (
              <ul aria-label="Recent projects" className="surface-card divide-y divide-border/70 overflow-hidden">
                {recentProjects.map((project) => (
                  <RecentProjectRow key={project.id} project={project} />
                ))}
              </ul>
            )}
          </section>

          <section className="space-y-3" aria-labelledby="home-queues-heading">
            <SectionHeading id="home-queues-heading" title="Queues" />
            <ul className="surface-card divide-y divide-border/70 overflow-hidden">
              <QueueRow
                icon={Inbox}
                title="Triage"
                detail={
                  triageCount === 0
                    ? "Filing queue is clear."
                    : `${triageCount} item${triageCount === 1 ? "" : "s"} waiting to be assigned.`
                }
                to="/triage"
                linkLabel="Open triage"
              />
              <QueueRow
                icon={PenLine}
                title="Drafts"
                detail={
                  typeof draftsReady === "number" && draftsReady > 0
                    ? `${draftsReady} draft${draftsReady === 1 ? "" : "s"} ready`
                    : "No drafts waiting."
                }
                to={typeof draftsReady === "number" && draftsReady > 0 ? "/drafts" : undefined}
                linkLabel={
                  typeof draftsReady === "number" && draftsReady > 0
                    ? `${draftsReady} draft${draftsReady === 1 ? "" : "s"} ready`
                    : undefined
                }
              />
              {connectedAccounts.length === 0 && (
                <QueueRow
                  icon={Plug}
                  title="Email"
                  detail="No mailbox connected yet."
                  to="/accounts"
                  linkLabel="Connect email"
                />
              )}
            </ul>
          </section>

          {fyi.length > 0 && <FyiSection items={fyi} />}
        </div>
      </div>
    </div>
  );
}

function SectionHeading({ id, title, description }: { id: string; title: string; description?: string }) {
  return (
    <div>
      <h2 id={id} className="font-display text-xl font-medium">
        {title}
      </h2>
      {description && <p className="text-sm text-muted-foreground">{description}</p>}
    </div>
  );
}

/** A square icon in the style the project page and lists use. */
function IconTile({ icon: Icon, tone }: { icon: LucideIcon; tone?: "adverse" }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border",
        tone === "adverse" ? "border-destructive/30 bg-destructive/5" : "border-border bg-secondary",
      )}
    >
      <Icon className={cn("h-4 w-4", tone === "adverse" ? "text-destructive" : "text-muted-foreground")} />
    </span>
  );
}

const NEEDS_ME_ICONS: Record<string, LucideIcon> = {
  open_contradiction: AlertTriangle,
  provisional_decision: Scale,
  provisional_fact: FileCheck2,
  issue_assignee: CircleDot,
  member_role: Users,
  mail_action_item: ListTodo,
};

function NeedsMeItem({ row }: { row: NeedsMeRow }) {
  const adverse = row.whyMe === "open_contradiction";
  return (
    <li className="flex items-start gap-3 px-4 py-3">
      <IconTile icon={NEEDS_ME_ICONS[row.whyMe] ?? CircleDot} tone={adverse ? "adverse" : undefined} />
      <div className="min-w-0 flex-1 space-y-1">
        <Link
          to={row.href}
          className="block rounded-sm font-medium hover:underline focus-visible:underline focus-visible:outline-none"
        >
          {row.title}
        </Link>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className={cn(adverse && "font-medium text-destructive")}>{row.whyMeLabel}</span>
          {row.projectLabel && (
            <>
              <span aria-hidden="true">·</span>
              <span className="truncate">{row.projectLabel}</span>
            </>
          )}
        </div>
      </div>
    </li>
  );
}

function QueueRow({
  icon,
  title,
  detail,
  to,
  linkLabel,
}: {
  icon: LucideIcon;
  title: string;
  detail: string;
  to?: string;
  linkLabel?: string;
}) {
  const body = (
    <>
      <IconTile icon={icon} />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{detail}</p>
      </div>
      {to && (
        <ChevronRight
          aria-hidden="true"
          className="mt-2 h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none motion-reduce:group-hover:translate-x-0"
        />
      )}
    </>
  );
  return (
    <li>
      {to ? (
        <Link
          to={to}
          aria-label={linkLabel}
          className="group flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
        >
          {body}
        </Link>
      ) : (
        <div className="flex items-start gap-3 px-4 py-3">{body}</div>
      )}
    </li>
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
    <section aria-label="Ask across projects" className="surface-card overflow-hidden">
      <form
        className="flex flex-col gap-2 p-3 sm:flex-row sm:items-center"
        onSubmit={(e) => {
          e.preventDefault();
          if (question.trim() && !pending) onAsk();
        }}
      >
        <div className="relative flex-1">
          <Sparkles
            aria-hidden="true"
            className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            placeholder="Ask across projects…"
            aria-label="Ask across projects"
            className="h-10 pl-9"
            disabled={!llmEnabled || pending}
          />
        </div>
        <Button
          type="submit"
          className="h-10"
          disabled={!llmEnabled || !question.trim() || pending}
          title={
            llmEnabled
              ? "Answer from structured project state across your projects"
              : "Configure LLM_BASE_URL and LLM_MODEL on the API"
          }
        >
          {pending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
          {pending ? "Asking…" : llmEnabled ? "Ask" : "Ask (LLM off)"}
        </Button>
      </form>
      {!llmEnabled && (
        <p className="border-t border-border/70 px-4 py-2 text-xs text-muted-foreground">
          Asking needs a language model configured on the API.
        </p>
      )}
      {answer ? (
        <div aria-live="polite" className="space-y-3 border-t border-border/70 bg-muted/20 px-4 py-3 text-sm">
          <p className="max-w-prose leading-relaxed">{answer.answer}</p>
          {answer.citations.length > 0 ? (
            <ul aria-label="Sources" className="flex flex-wrap gap-1.5">
              {answer.citations.map((c) => {
                const href = citationHref(c);
                const label = [c.project_code, c.type, c.id.slice(0, 8)].filter(Boolean).join(" · ");
                const chip = "inline-flex items-center rounded-full border border-border bg-background px-2.5 py-0.5 font-mono text-[11px]";
                return (
                  <li key={`${c.type}:${c.id}`}>
                    {href ? (
                      <Link
                        to={href}
                        className={cn(
                          chip,
                          "transition-colors hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        )}
                      >
                        {label}
                      </Link>
                    ) : (
                      <span className={cn(chip, "text-muted-foreground")}>{label}</span>
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
      className={cn(
        "surface-card px-4 py-3 transition-colors hover:border-foreground/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        highlight && "border-destructive/40",
      )}
    >
      <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">{label}</p>
      <p
        className={cn(
          "mt-1 flex items-center gap-1.5 font-display text-2xl tabular-nums",
          value === 0 && "text-muted-foreground",
          highlight && "text-destructive",
        )}
      >
        {highlight ? <AlertTriangle className="h-4 w-4" aria-hidden="true" /> : null}
        {value}
      </p>
    </Link>
  );
}

const ACTIVITY_ICONS: Partial<Record<ActivityItem["kind"], LucideIcon>> = {
  decision_proposed: Scale,
  decision_accepted: Scale,
  decision_withdrawn: Scale,
  fact_recorded: FileCheck2,
  fact_superseded: FileCheck2,
  contradiction_opened: AlertTriangle,
  contradiction_resolved: AlertTriangle,
  issue_opened: CircleDot,
  issue_resolved: CircleDot,
};

function ActivityRow({ item }: { item: ActivityItem }) {
  const at = new Date(item.occurred_at);
  const adverse = isAdverse(item.kind);
  return (
    <li className="flex items-start gap-3 px-4 py-3">
      <IconTile icon={ACTIVITY_ICONS[item.kind] ?? History} tone={adverse ? "adverse" : undefined} />
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-baseline justify-between gap-x-3">
          <Link
            to={activityHref(item)}
            className="min-w-0 rounded-sm font-medium hover:underline focus-visible:underline focus-visible:outline-none"
          >
            {item.title}
          </Link>
          <time className="shrink-0 text-xs text-muted-foreground" dateTime={item.occurred_at} title={at.toLocaleString()}>
            {relativeTime(item.occurred_at)}
          </time>
        </div>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className={cn(adverse && "font-medium text-destructive")}>{activityLabel(item.kind)}</span>
          <span aria-hidden="true">·</span>
          <span className="font-mono">{item.project_code}</span>
          {item.source ? <Tag>{item.source}</Tag> : null}
        </div>
      </div>
    </li>
  );
}

function RecentProjectRow({ project }: { project: OverviewProject }) {
  return (
    <li>
      <Link
        to={`/projects/${project.id}`}
        className="group flex items-start gap-3 px-4 py-3 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <span
          aria-hidden="true"
          className="inline-flex h-9 min-w-9 shrink-0 items-center justify-center rounded-md border border-border bg-secondary px-1.5 font-mono text-[11px] font-medium"
        >
          {project.code}
        </span>
        <div className="min-w-0 flex-1 space-y-0.5">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium text-foreground">{project.name}</p>
            {project.attention_count > 0 ? (
              <span className="rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">
                {project.attention_count} needs you
              </span>
            ) : null}
          </div>
          <p className="line-clamp-2 text-sm text-muted-foreground">
            {project.teaser || "No current position yet."}
          </p>
          <p className="text-xs text-muted-foreground">
            {project.last_activity_at ? `Active ${relativeTime(project.last_activity_at)}` : "No activity yet"}
          </p>
        </div>
        <ChevronRight
          aria-hidden="true"
          className="mt-2.5 h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none motion-reduce:group-hover:translate-x-0"
        />
      </Link>
    </li>
  );
}

/**
 * Things mail told you that ask nothing of you. Each opens its message, and
 * Dismiss clears it once read; unlike a to-do there is nothing to finish.
 */
function FyiSection({ items }: { items: SummaryFYI[] }) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [showAll, setShowAll] = useState(false);
  const visible = showAll ? items : items.slice(0, FYI_PREVIEW);
  const dismiss = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return dismissFYI(accessToken, id);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary"] });
    },
    onError: (err) => {
      toast({
        title: "Could not dismiss",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  return (
    <section className="space-y-3" aria-labelledby="home-fyi-heading">
      <SectionHeading
        id="home-fyi-heading"
        title="For your information"
        description="What mail told you that needs nothing from you."
      />
      <ul id="home-fyi-list" aria-label="For your information" className="surface-card divide-y divide-border/70 overflow-hidden">
        {visible.map((item) => (
          <li key={item.id} className="flex items-start gap-3 px-4 py-3">
            <IconTile icon={Info} />
            <div className="min-w-0 flex-1 space-y-0.5">
              <Link
                to={inboxHref(item.message_id, item.account_id)}
                className="line-clamp-3 rounded-sm text-sm hover:underline focus-visible:underline focus-visible:outline-none"
              >
                {item.text}
              </Link>
              <time dateTime={item.created_at} className="block text-xs text-muted-foreground">
                {relativeTime(item.created_at)}
              </time>
            </div>
            <Button
              size="icon"
              variant="ghost"
              className="h-9 w-9 shrink-0 text-muted-foreground"
              aria-label={`Dismiss “${item.text}”`}
              title="Dismiss"
              disabled={dismiss.isPending && dismiss.variables === item.id}
              onClick={() => dismiss.mutate(item.id)}
            >
              <X aria-hidden="true" className="h-4 w-4" />
            </Button>
          </li>
        ))}
      </ul>
      {items.length > FYI_PREVIEW && (
        <Button
          variant="ghost"
          size="sm"
          className="text-muted-foreground"
          aria-expanded={showAll}
          aria-controls="home-fyi-list"
          onClick={() => setShowAll((v) => !v)}
        >
          {showAll ? "Show fewer" : `Show all ${items.length}`}
        </Button>
      )}
    </section>
  );
}
