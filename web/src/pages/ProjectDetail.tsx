import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, ClipboardPaste, Pencil, Archive } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmAction, Tag } from "@/components/ConnectionCard";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useProjectDetailData } from "@/hooks/useProjectDetailData";
import { toast } from "@/hooks/use-toast";
import {
  ApiError,
  addIssueItem,
  completeProjectTodo,
  confirmDecision,
  confirmFactVersion,
  createProjectDecision,
  createProjectFact,
  createProjectIssue,
  discardIssue,
  rejectFactVersion,
  resolveContradiction,
  updateProject,
  updateProjectMember,
  withdrawDecision,
  type TimelineItem,
} from "@/lib/auth";
import { ExtractionStatus } from "./project/ExtractionStatus";
import { NeedsYou } from "./project/NeedsYou";
import { TodosSection } from "./project/TodosSection";
import { PositionSection } from "./project/PositionSection";
import { IssuesPanel } from "./project/IssuesPanel";
import { AskPanel } from "./project/AskPanel";
import { CorrespondenceSection, type TimelineFilterState } from "./project/CorrespondenceSection";
import { PasteDialog } from "./project/PasteDialog";
import {
  DecisionDialog,
  EditProjectDialog,
  FactDialog,
  IssueDialog,
  type FactInput,
  type ProjectEdit,
} from "./project/ProjectDialogs";
import { sectionForLegacyMode, type ItemRef } from "./project/format";

function refFor(item: TimelineItem): ItemRef[] {
  const ref = item.message_id ? { message_id: item.message_id } : { manual_item_id: item.manual_item_id };
  return ref.message_id || ref.manual_item_id ? [ref] : [];
}

/** Numbers stay numbers, so facts compare and chart properly. */
function factValue(raw: string): string | number {
  return /^-?\d+(\.\d+)?$/.test(raw) ? Number(raw) : raw;
}

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const projectID = id!;
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const { accounts } = useAccountsData();

  const [filters, setFilters] = useState<TimelineFilterState>({ source: "all", unassignedToIssue: false });
  const data = useProjectDetailData(id, filters);

  const [pasteOpen, setPasteOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [decisionOpen, setDecisionOpen] = useState(false);
  const [factDialog, setFactDialog] = useState<{ open: boolean; seed?: { label?: string; evidence?: ItemRef[] } }>({ open: false });
  const [issueDialog, setIssueDialog] = useState<{ open: boolean; seed?: { title?: string; items?: ItemRef[] } }>({ open: false });

  // Deep links (#position, or the old ?mode=) land on their section once the
  // page has content to scroll to.
  const loaded = Boolean(data.project);
  useEffect(() => {
    if (!loaded) return;
    const target = location.hash.replace(/^#/, "") || sectionForLegacyMode(searchParams.get("mode"));
    if (!target) return;
    document.getElementById(target)?.scrollIntoView?.({ block: "start" });
  }, [loaded, location.hash, searchParams]);

  const invalidate = (...keys: string[]) =>
    Promise.all(keys.map((key) => queryClient.invalidateQueries({ queryKey: [key] })));
  const failed = (title: string) => (err: unknown) =>
    toast({ title, description: err instanceof ApiError ? err.message : "Please try again.", variant: "destructive" });
  const authed = () => {
    if (!accessToken) throw new Error("Not authenticated");
    return accessToken;
  };

  const createIssue = useMutation({
    mutationFn: (input: { title: string; note: string; items: ItemRef[] }) =>
      createProjectIssue(authed(), projectID, {
        title: input.title,
        current_position_note: input.note || undefined,
        item_refs: input.items.length > 0 ? input.items : undefined,
      }),
    onSuccess: async () => {
      toast({ title: "Issue created" });
      setIssueDialog({ open: false });
      await invalidate("project-issues", "project-timeline");
    },
    onError: failed("Could not create the issue"),
  });
  const completeTodo = useMutation({
    mutationFn: (todoID: string) => completeProjectTodo(authed(), projectID, todoID),
    onSuccess: async () => {
      toast({ title: "To-do done" });
      await invalidate("project-todos", "project-attention", "attention", "overview", "summary", "issue");
    },
    onError: failed("Could not mark the to-do done"),
  });
  const discard = useMutation({
    mutationFn: (issueID: string) => discardIssue(authed(), issueID),
    onSuccess: async () => {
      toast({ title: "Issue discarded" });
      await invalidate("project-issues", "project-attention");
    },
    onError: failed("Could not discard the issue"),
  });
  const attach = useMutation({
    mutationFn: (args: { issueID: string; item: TimelineItem }) =>
      addIssueItem(authed(), args.issueID, { message_id: args.item.message_id, manual_item_id: args.item.manual_item_id }),
    onSuccess: async () => {
      toast({ title: "Attached to the issue" });
      await invalidate("project-timeline", "project-issues", "issue");
    },
    onError: failed("Could not attach"),
  });
  const createFact = useMutation({
    mutationFn: (input: FactInput) => {
      const active = data.facts.find((f) => f.subject_key === input.subjectKey)?.versions.find((v) => v.status === "active");
      return createProjectFact(authed(), projectID, {
        subject_key: input.subjectKey,
        label: input.label,
        value: factValue(input.value),
        unit: input.unit || undefined,
        confirm: input.confirm,
        supersedes_version_id: input.confirm && active ? active.id : undefined,
        evidence: input.evidence.length > 0 ? input.evidence : undefined,
      });
    },
    onSuccess: async (_d, input) => {
      toast({ title: input.confirm ? "Fact recorded" : "Fact proposed", description: input.confirm ? undefined : "It waits in Needs you." });
      setFactDialog({ open: false });
      await invalidate("project-facts", "project-current-position");
    },
    onError: failed("Could not save the fact"),
  });
  const createDecision = useMutation({
    mutationFn: (input: { statement: string; accept: boolean }) =>
      createProjectDecision(authed(), projectID, { statement: input.statement, confirm: input.accept }),
    onSuccess: async (_d, input) => {
      toast({ title: input.accept ? "Decision recorded" : "Decision proposed" });
      setDecisionOpen(false);
      await invalidate("project-decisions", "project-current-position", "attention");
    },
    onError: failed("Could not save the decision"),
  });

  // Confirmation queue.
  const confirmFact = useMutation({
    mutationFn: (args: { versionID: string; supersedes?: string }) =>
      confirmFactVersion(authed(), args.versionID, { supersedes_version_id: args.supersedes }),
    onSuccess: async () => {
      toast({ title: "Fact confirmed" });
      await invalidate("project-facts", "project-current-position", "project-attention");
    },
    onError: failed("Could not confirm"),
  });
  const rejectFact = useMutation({
    mutationFn: (versionID: string) => rejectFactVersion(authed(), versionID),
    onSuccess: async () => {
      toast({ title: "Change rejected" });
      await invalidate("project-facts", "project-current-position", "project-attention");
    },
    onError: failed("Could not reject"),
  });
  const acceptDecision = useMutation({
    mutationFn: (decisionID: string) => confirmDecision(authed(), decisionID),
    onSuccess: async () => {
      toast({ title: "Decision accepted" });
      await invalidate("project-decisions", "project-current-position", "attention", "project-attention");
    },
    onError: failed("Could not accept"),
  });
  const withdraw = useMutation({
    mutationFn: (decisionID: string) => withdrawDecision(authed(), decisionID),
    onSuccess: async () => {
      toast({ title: "Decision withdrawn" });
      await invalidate("project-decisions", "project-current-position", "attention", "project-attention");
    },
    onError: failed("Could not withdraw"),
  });
  const resolve = useMutation({
    mutationFn: (args: { id: string; resolution: "supersede" | "reject_a" | "reject_b" | "note"; keep?: string }) =>
      resolveContradiction(authed(), args.id, { resolution: args.resolution, keep_fact_version_id: args.keep }),
    onSuccess: async () => {
      toast({ title: "Resolved" });
      await invalidate("project-contradictions", "project-facts", "project-current-position", "project-attention");
    },
    onError: failed("Could not resolve"),
  });

  const saveDetails = useMutation({
    mutationFn: async (input: ProjectEdit) => {
      const token = authed();
      await updateProject(token, projectID, {
        name: input.name,
        client: input.client || null,
        description: input.description || null,
        keywords: input.keywords,
      });
      await updateProjectMember(token, projectID, {
        role: input.role,
        discipline: input.discipline || null,
        current_scope: input.scope || null,
      });
    },
    onSuccess: async () => {
      toast({ title: "Project saved" });
      setEditOpen(false);
      await invalidate("project", "projects");
    },
    onError: failed("Could not save the project"),
  });
  const archive = useMutation({
    mutationFn: () => updateProject(authed(), projectID, { archived: true }),
    onSuccess: async () => {
      toast({ title: "Project archived" });
      await invalidate("projects");
      navigate("/projects");
    },
    onError: failed("Could not archive the project"),
  });

  if (data.projectQuery.isLoading) {
    return (
      <div aria-label="Loading project" className="space-y-6">
        <Skeleton className="h-4 w-24" />
        <div className="flex items-center gap-4">
          <Skeleton className="h-12 w-12 rounded-md" />
          <div className="space-y-2">
            <Skeleton className="h-8 w-64" />
            <Skeleton className="h-4 w-48" />
          </div>
        </div>
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (data.projectQuery.isError || !data.project) {
    return (
      <div className="space-y-4">
        <BackLink />
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          {data.projectQuery.error instanceof ApiError ? data.projectQuery.error.message : "This project could not be found."}
        </div>
      </div>
    );
  }

  const project = data.project;
  const member = project.member;
  const subtitle = [project.client, project.description].filter(Boolean).join(" · ");
  const busy = confirmFact.isPending || rejectFact.isPending || acceptDecision.isPending || withdraw.isPending || resolve.isPending;

  return (
    <div className="space-y-8">
      <header className="space-y-4 border-b border-border/70 pb-6">
        <BackLink />
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-4">
            <span
              aria-hidden="true"
              className="inline-flex h-12 min-w-12 shrink-0 items-center justify-center rounded-md border border-border bg-secondary px-2 font-mono text-sm font-medium"
            >
              {project.code}
            </span>
            <div className="min-w-0 space-y-1">
              <p className="sr-only">Project {project.code}</p>
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="font-display text-3xl font-medium leading-tight md:text-4xl">{project.name}</h1>
                {project.archived_at && <Tag>Archived</Tag>}
              </div>
              {subtitle && <p className="text-muted-foreground">{subtitle}</p>}
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
                {member?.role && (
                  <span>
                    You: <span className="text-foreground">{member.role}</span>
                    {member.discipline ? `, ${member.discipline}` : ""}
                  </span>
                )}
                <ExtractionStatus
                  lastExtractedAt={data.extraction.lastExtractedAt}
                  reviewing={data.extraction.reviewing}
                  stalled={data.extraction.stalled}
                  pending={data.extraction.pending}
                  onCheckNow={data.extraction.checkNow}
                />
              </div>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90" onClick={() => setPasteOpen(true)}>
              <ClipboardPaste aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Paste correspondence
            </Button>
            <Button size="sm" variant="outline" onClick={() => setEditOpen(true)}>
              <Pencil aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Edit details
            </Button>
            {!project.archived_at && (
              <ConfirmAction
                title={`Archive ${project.name}?`}
                description="It leaves the project list and stops receiving new correspondence. Everything already on it is kept, and you can find it again with Show archived."
                confirmLabel="Archive project"
                onConfirm={() => archive.mutate()}
                trigger={(open) => (
                  <Button size="sm" variant="ghost" className="text-muted-foreground" onClick={open} disabled={archive.isPending}>
                    <Archive aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Archive
                  </Button>
                )}
              />
            )}
          </div>
        </div>
      </header>

      <NeedsYou
        rows={data.confirmationRows}
        attention={data.attention?.items ?? []}
        loading={data.factsQuery.isLoading || data.decisionsQuery.isLoading || data.contradictionsQuery.isLoading}
        projectID={projectID}
        actions={{
          confirmFact: (versionID, supersedes) => confirmFact.mutate({ versionID, supersedes }),
          rejectFact: (versionID) => rejectFact.mutate(versionID),
          confirmDecision: (decisionID) => acceptDecision.mutate(decisionID),
          withdrawDecision: (decisionID) => withdraw.mutate(decisionID),
          resolveContradiction: (cid, resolution, keep) => resolve.mutate({ id: cid, resolution, keep }),
          busy,
        }}
      />

      <TodosSection
        todos={data.todos}
        loading={data.todosQuery.isLoading}
        projectID={projectID}
        busy={completeTodo.isPending}
        onDone={(id) => completeTodo.mutate(id)}
      />

      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_22rem]">
        <div className="min-w-0 space-y-8">
          <PositionSection
            position={data.currentPosition}
            facts={data.facts}
            decisions={data.decisions}
            loading={data.currentPositionQuery.isLoading}
            onAddFact={() => setFactDialog({ open: true })}
            onAddDecision={() => setDecisionOpen(true)}
          />
          <CorrespondenceSection
            items={data.timeline}
            loading={data.timelineQuery.isLoading}
            filters={filters}
            onFiltersChange={setFilters}
            projectID={projectID}
            issues={data.openIssues}
            accountFor={(accountID) => (accountID ? accounts.find((a) => a.id === accountID) : undefined)}
            attaching={attach.isPending}
            onAttach={(issueID, item) => attach.mutate({ issueID, item })}
            onCreateIssue={(item) => setIssueDialog({ open: true, seed: { title: item.title?.trim(), items: refFor(item) } })}
            onRecordFact={(item) => setFactDialog({ open: true, seed: { label: item.title?.trim(), evidence: refFor(item) } })}
          />
        </div>
        <aside className="space-y-8">
          <IssuesPanel
            issues={data.openIssues}
            loading={data.issuesQuery.isLoading}
            projectID={projectID}
            onNewIssue={() => setIssueDialog({ open: true })}
            onDiscard={(issueID) => discard.mutate(issueID)}
            discarding={discard.isPending}
          />
          <AskPanel projectID={projectID} enabled={data.llmEnabled} />
        </aside>
      </div>

      <FactDialog
        open={factDialog.open}
        onOpenChange={(open) => setFactDialog((d) => ({ ...d, open }))}
        facts={data.facts}
        seed={factDialog.seed}
        pending={createFact.isPending}
        onSubmit={(input) => createFact.mutate(input)}
      />
      <DecisionDialog
        open={decisionOpen}
        onOpenChange={setDecisionOpen}
        pending={createDecision.isPending}
        onSubmit={(input) => createDecision.mutate(input)}
      />
      <IssueDialog
        open={issueDialog.open}
        onOpenChange={(open) => setIssueDialog((d) => ({ ...d, open }))}
        seed={issueDialog.seed}
        pending={createIssue.isPending}
        onSubmit={(input) => createIssue.mutate(input)}
      />
      <EditProjectDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        project={project}
        pending={saveDetails.isPending}
        onSubmit={(input) => saveDetails.mutate(input)}
      />
      <Dialog open={pasteOpen} onOpenChange={setPasteOpen}>
        <PasteDialog
          projectID={projectID}
          accessToken={accessToken!}
          onDone={async () => {
            setPasteOpen(false);
            await invalidate("project-timeline", "unassigned");
          }}
        />
      </Dialog>
    </div>
  );
}

function BackLink() {
  return (
    <Link
      to="/projects"
      className="inline-flex items-center gap-1.5 rounded-sm text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <ArrowLeft aria-hidden="true" className="h-4 w-4" /> Projects
    </Link>
  );
}
