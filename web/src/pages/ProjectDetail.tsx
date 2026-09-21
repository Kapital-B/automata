import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import {
  ApiError,
  addIssueItem,
  askProject,
  confirmFactVersion,
  confirmDecision,
  createProjectDecision,
  createProjectFact,
  createProjectIssue,
  discardIssue,
  getProject,
  rejectFactVersion,
  resolveContradiction,
  updateProject,
  updateProjectMember,
  withdrawDecision,
  type TimelineItem,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useProjectDetailData } from "@/hooks/useProjectDetailData";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ExtractionStatus } from "./project/ExtractionStatus";
import { NeedsConfirmation } from "./project/NeedsConfirmation";
import { OpenMode } from "./project/OpenMode";
import { PasteDialog } from "./project/PasteDialog";
import { PositionMode } from "./project/PositionMode";
import { TrailMode, type TimelineFilterState } from "./project/TrailMode";
import { DEFINITIONS } from "./project/definitions";

const PROJECT_MODES = [
  { id: "trail" as const, label: "Trail" },
  { id: "position" as const, label: "Position" },
  { id: "open" as const, label: "Open" },
];

type ProjectMode = (typeof PROJECT_MODES)[number]["id"];

function parseProjectMode(raw: string | null): ProjectMode {
  if (raw === "position" || raw === "open") return raw;
  return "trail";
}

function itemRef(item: TimelineItem) {
  return item.message_id
    ? { message_id: item.message_id }
    : { manual_item_id: item.manual_item_id };
}

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const mode = parseProjectMode(searchParams.get("mode"));
  const setMode = (next: ProjectMode) => {
    setSearchParams(
      (prev) => {
        const nextParams = new URLSearchParams(prev);
        if (next === "trail") nextParams.delete("mode");
        else nextParams.set("mode", next);
        return nextParams;
      },
      { replace: true },
    );
  };
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { accounts } = useAccountsData();

  const [filters, setFilters] = useState<TimelineFilterState>({
    source: "all",
    unassignedToIssue: false,
  });
  const [pasteOpen, setPasteOpen] = useState(false);
  const [createIssueOpen, setCreateIssueOpen] = useState(false);
  const [newIssueTitle, setNewIssueTitle] = useState("");
  const [newIssueNote, setNewIssueNote] = useState("");
  const [pendingItemRefs, setPendingItemRefs] = useState<
    { message_id?: string; manual_item_id?: string }[]
  >([]);
  const [issueDialogHint, setIssueDialogHint] = useState<string | null>(null);
  const [createFactOpen, setCreateFactOpen] = useState(false);
  const [factSubjectKey, setFactSubjectKey] = useState("pump.p03.duty_kw");
  const [factLabel, setFactLabel] = useState("");
  const [factValue, setFactValue] = useState("");
  const [factUnit, setFactUnit] = useState("");
  const [factConfirmNow, setFactConfirmNow] = useState(true);
  const [factEvidence, setFactEvidence] = useState<
    { message_id?: string; manual_item_id?: string }[]
  >([]);
  const [createDecisionOpen, setCreateDecisionOpen] = useState(false);
  const [decisionStatement, setDecisionStatement] = useState("");
  const [decisionConfirmNow, setDecisionConfirmNow] = useState(true);
  const [askQuestion, setAskQuestion] = useState("");
  const [askAnswer, setAskAnswer] = useState<{
    answer: string;
    citations: { type: string; id: string }[];
    confidence: number;
  } | null>(null);

  const data = useProjectDetailData(id, {
    source: filters.source,
    unassignedToIssue: filters.unassignedToIssue,
  });

  const invalidateFacts = async () => {
    await queryClient.invalidateQueries({ queryKey: ["project-facts"] });
    await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
  };

  const failed = (title: string) => (err: unknown) => {
    toast({
      title,
      description: err instanceof ApiError ? err.message : "Please try again.",
      variant: "destructive",
    });
  };

  const createIssueMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return createProjectIssue(accessToken, id, {
        title: newIssueTitle.trim(),
        current_position_note: newIssueNote.trim() || undefined,
        item_refs: pendingItemRefs.length > 0 ? pendingItemRefs : undefined,
      });
    },
    onSuccess: async () => {
      toast({ title: "Issue created" });
      setCreateIssueOpen(false);
      setNewIssueTitle("");
      setNewIssueNote("");
      setPendingItemRefs([]);
      setIssueDialogHint(null);
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
      await queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
    },
    onError: failed("Could not create issue"),
  });

  const discardIssueMutation = useMutation({
    mutationFn: async (issueID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return discardIssue(accessToken, issueID);
    },
    onSuccess: async () => {
      toast({ title: "Issue discarded" });
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
      await queryClient.invalidateQueries({ queryKey: ["project-attention"] });
    },
    onError: failed("Discard failed"),
  });

  const attachMutation = useMutation({
    mutationFn: async (args: { issueID: string; messageID?: string; manualItemID?: string }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return addIssueItem(accessToken, args.issueID, {
        message_id: args.messageID,
        manual_item_id: args.manualItemID,
      });
    },
    onSuccess: async () => {
      toast({ title: "Attached to issue" });
      await queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
      await queryClient.invalidateQueries({ queryKey: ["issue"] });
    },
    onError: failed("Attach failed"),
  });

  const createFactMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      const trimmed = factValue.trim();
      const asNumber = Number(trimmed);
      const value =
        trimmed !== "" && !Number.isNaN(asNumber) && /^-?\d+(\.\d+)?$/.test(trimmed)
          ? asNumber
          : trimmed;
      const existing = data.facts.find((f) => f.subject_key === factSubjectKey.trim());
      const active = existing?.versions.find((v) => v.status === "active");
      return createProjectFact(accessToken, id, {
        subject_key: factSubjectKey.trim(),
        label: factLabel.trim(),
        value,
        unit: factUnit.trim() || undefined,
        confirm: factConfirmNow,
        supersedes_version_id: factConfirmNow && active ? active.id : undefined,
        evidence: factEvidence.length > 0 ? factEvidence : undefined,
      });
    },
    onSuccess: async () => {
      toast({ title: factConfirmNow ? "Fact confirmed" : "Fact proposed" });
      setCreateFactOpen(false);
      setFactLabel("");
      setFactValue("");
      setFactUnit("");
      setFactEvidence([]);
      setFactConfirmNow(true);
      await invalidateFacts();
    },
    onError: failed("Could not save fact"),
  });

  const confirmFactMutation = useMutation({
    mutationFn: async (args: { versionID: string; supersedesVersionID?: string }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return confirmFactVersion(accessToken, args.versionID, {
        supersedes_version_id: args.supersedesVersionID,
      });
    },
    onSuccess: async () => {
      toast({ title: "Fact confirmed" });
      await invalidateFacts();
    },
    onError: failed("Confirm failed"),
  });

  const rejectFactMutation = useMutation({
    mutationFn: async (versionID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return rejectFactVersion(accessToken, versionID);
    },
    onSuccess: async () => {
      toast({ title: "Proposal rejected" });
      await invalidateFacts();
    },
    onError: failed("Reject failed"),
  });

  const resolveContradictionMutation = useMutation({
    mutationFn: async (args: {
      id: string;
      resolution: "supersede" | "reject_a" | "reject_b" | "note";
      keep_fact_version_id?: string;
    }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return resolveContradiction(accessToken, args.id, {
        resolution: args.resolution,
        keep_fact_version_id: args.keep_fact_version_id,
      });
    },
    onSuccess: async () => {
      toast({ title: "Contradiction resolved" });
      await queryClient.invalidateQueries({ queryKey: ["project-contradictions"] });
      await invalidateFacts();
    },
    onError: failed("Resolve failed"),
  });

  const createDecisionMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return createProjectDecision(accessToken, id, {
        statement: decisionStatement.trim(),
        confirm: decisionConfirmNow,
      });
    },
    onSuccess: async () => {
      toast({ title: decisionConfirmNow ? "Decision accepted" : "Decision proposed" });
      setCreateDecisionOpen(false);
      setDecisionStatement("");
      await queryClient.invalidateQueries({ queryKey: ["project-decisions"] });
      await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
      await queryClient.invalidateQueries({ queryKey: ["attention"] });
    },
    onError: failed("Create decision failed"),
  });

  const confirmDecisionMutation = useMutation({
    mutationFn: async (decisionID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return confirmDecision(accessToken, decisionID);
    },
    onSuccess: async () => {
      toast({ title: "Decision confirmed" });
      await queryClient.invalidateQueries({ queryKey: ["project-decisions"] });
      await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
      await queryClient.invalidateQueries({ queryKey: ["attention"] });
    },
    onError: failed("Confirm failed"),
  });

  const withdrawDecisionMutation = useMutation({
    mutationFn: async (decisionID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return withdrawDecision(accessToken, decisionID);
    },
    onSuccess: async () => {
      toast({ title: "Decision withdrawn" });
      await queryClient.invalidateQueries({ queryKey: ["project-decisions"] });
      await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
      await queryClient.invalidateQueries({ queryKey: ["attention"] });
    },
    onError: failed("Withdraw failed"),
  });

  const askMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return askProject(accessToken, id, askQuestion.trim());
    },
    onSuccess: (res) => setAskAnswer(res),
    onError: failed("Ask failed"),
  });

  const [name, setName] = useState("");
  const [keywords, setKeywords] = useState("");
  const [role, setRole] = useState("");
  const [discipline, setDiscipline] = useState("");
  const [scope, setScope] = useState("");

  useEffect(() => {
    if (!data.project) return;
    setName(data.project.name);
    setKeywords((data.project.keywords ?? []).join(", "));
    setRole(data.project.member?.role ?? "");
    setDiscipline(data.project.member?.discipline ?? "");
    setScope(data.project.member?.current_scope ?? "");
  }, [data.project]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      await updateProject(accessToken, id, {
        name: name.trim(),
        keywords: keywords
          .split(/[,;\n]+/)
          .map((k) => k.trim())
          .filter(Boolean),
      });
      await updateProjectMember(accessToken, id, {
        role: role.trim(),
        discipline: discipline.trim() || null,
        current_scope: scope.trim() || null,
      });
    },
    onSuccess: async () => {
      toast({ title: "Project saved" });
      await queryClient.invalidateQueries({ queryKey: ["project", accessToken, id] });
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
    onError: failed("Save failed"),
  });

  const archiveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return updateProject(accessToken, id, { archived: true });
    },
    onSuccess: async () => {
      toast({ title: "Project archived" });
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate("/projects");
    },
  });

  const accountFor = (accountID?: string) =>
    accountID ? accounts.find((x) => x.id === accountID) : undefined;

  if (data.projectQuery.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading project…
      </div>
    );
  }

  if (data.projectQuery.isError || !data.project) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-destructive">
          {data.projectQuery.error instanceof ApiError
            ? data.projectQuery.error.message
            : "Project not found."}
        </p>
        <Button variant="outline" onClick={() => navigate("/projects")}>
          Back to Projects
        </Button>
      </div>
    );
  }

  const project = data.project;
  const positionFacts = data.currentPosition?.facts ?? [];
  const positionDecisions = data.currentPosition?.decisions ?? [];

  return (
    <div className="space-y-6">
      <Dialog open={createFactOpen} onOpenChange={setCreateFactOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add fact</DialogTitle>
            <DialogDescription>
              Creates a versioned assertion. Same subject key appends a new version —
              never overwrites in place.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Input
              value={factSubjectKey}
              onChange={(e) => setFactSubjectKey(e.target.value)}
              placeholder="pump.p03.duty_kw"
            />
            <Input
              value={factLabel}
              onChange={(e) => setFactLabel(e.target.value)}
              placeholder="Pump P-03 duty"
            />
            <div className="flex gap-2">
              <Input
                value={factValue}
                onChange={(e) => setFactValue(e.target.value)}
                placeholder="90"
                className="flex-1"
              />
              <Input
                value={factUnit}
                onChange={(e) => setFactUnit(e.target.value)}
                placeholder="kW"
                className="w-24"
              />
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={factConfirmNow}
                onChange={(e) => setFactConfirmNow(e.target.checked)}
              />
              Confirm as active now
            </label>
            {factEvidence.length > 0 ? (
              <p className="text-xs text-muted-foreground">
                {factEvidence.length} evidence item(s) attached
              </p>
            ) : null}
            <Button
              className="w-full"
              disabled={
                !factSubjectKey.trim() ||
                !factLabel.trim() ||
                !factValue.trim() ||
                createFactMutation.isPending
              }
              onClick={() => createFactMutation.mutate()}
            >
              {createFactMutation.isPending ? "Saving…" : "Save fact"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={createDecisionOpen} onOpenChange={setCreateDecisionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add decision</DialogTitle>
            <DialogDescription>
              Record an approval or go/no-go. Evidence is attached as correspondence
              supporting it arrives.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <Textarea
              value={decisionStatement}
              onChange={(e) => setDecisionStatement(e.target.value)}
              placeholder="Proceed with 90 kW duty for Pump P-03"
              rows={3}
            />
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={decisionConfirmNow}
                onChange={(e) => setDecisionConfirmNow(e.target.checked)}
              />
              Accept now
            </label>
            <Button
              className="w-full"
              disabled={!decisionStatement.trim() || createDecisionMutation.isPending}
              onClick={() => createDecisionMutation.mutate()}
            >
              {createDecisionMutation.isPending ? "Saving…" : "Save decision"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={createIssueOpen}
        onOpenChange={(open) => {
          setCreateIssueOpen(open);
          if (!open) {
            setIssueDialogHint(null);
            setPendingItemRefs([]);
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create issue</DialogTitle>
            <DialogDescription>
              Default assignee is you. Confirm to create — suggestions are never auto-saved.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {issueDialogHint ? (
              <p className="text-xs text-muted-foreground">{issueDialogHint}</p>
            ) : null}
            <Input
              value={newIssueTitle}
              onChange={(e) => setNewIssueTitle(e.target.value)}
              placeholder="Pump P-03 Sizing"
            />
            <Textarea
              value={newIssueNote}
              onChange={(e) => setNewIssueNote(e.target.value)}
              placeholder="Optional current position note"
              rows={2}
            />
            <Button
              className="w-full"
              disabled={!newIssueTitle.trim() || createIssueMutation.isPending}
              onClick={() => createIssueMutation.mutate()}
            >
              {createIssueMutation.isPending ? "Creating…" : "Create"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={pasteOpen} onOpenChange={setPasteOpen}>
        <PasteDialog
          projectID={id!}
          accessToken={accessToken!}
          onDone={async () => {
            setPasteOpen(false);
            await queryClient.invalidateQueries({
              queryKey: ["project-timeline", accessToken, id],
            });
            await queryClient.invalidateQueries({ queryKey: ["unassigned"] });
          }}
        />
      </Dialog>

      <PageHeader
        eyebrow={project.code}
        title={project.name}
        description="Where this project stands, and what needs you."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button onClick={() => setPasteOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Paste correspondence
            </Button>
            <Button variant="outline" asChild>
              <Link to="/projects">All projects</Link>
            </Button>
            <Button
              variant="outline"
              disabled={archiveMutation.isPending}
              onClick={() => archiveMutation.mutate()}
            >
              Archive
            </Button>
          </div>
        }
      />

      <ExtractionStatus
        lastExtractedAt={data.extraction.lastExtractedAt}
        reviewing={data.extraction.reviewing}
        stalled={data.extraction.stalled}
        pending={data.extraction.pending}
        onCheckNow={data.extraction.checkNow}
      />

      <details className="max-w-xl text-sm">
        <summary className="cursor-pointer text-muted-foreground">Edit header &amp; role</summary>
        <div className="mt-3 space-y-3">
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground" htmlFor="proj-name">
              Name
            </label>
            <Input id="proj-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground" htmlFor="proj-keywords">
              Keywords
            </label>
            <Input
              id="proj-keywords"
              value={keywords}
              onChange={(e) => setKeywords(e.target.value)}
              placeholder="cooling, chiller, P-03"
            />
            <p className="text-[11px] text-muted-foreground">Comma-separated; used for auto-assign.</p>
          </div>
          <p className="font-mono text-xs text-muted-foreground">Code: {project.code}</p>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground" htmlFor="member-role">
              Your role
            </label>
            <Input
              id="member-role"
              value={role}
              onChange={(e) => setRole(e.target.value)}
              placeholder="Mechanical Engineer"
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground" htmlFor="member-discipline">
              Discipline
            </label>
            <Input
              id="member-discipline"
              value={discipline}
              onChange={(e) => setDiscipline(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <label className="text-xs text-muted-foreground" htmlFor="member-scope">
              Current scope
            </label>
            <Input id="member-scope" value={scope} onChange={(e) => setScope(e.target.value)} />
          </div>
          <Button disabled={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
            {saveMutation.isPending ? "Saving…" : "Save"}
          </Button>
        </div>
      </details>

      <section
        aria-label="Current position"
        className="sticky top-14 z-20 border-y border-border/70 bg-background/95 py-3 backdrop-blur"
      >
        <h2 className="mb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Current position
        </h2>
        <p className="mb-2 max-w-prose text-xs text-muted-foreground">{DEFINITIONS.position}</p>
        {data.currentPositionQuery.isLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : positionFacts.length === 0 && positionDecisions.length === 0 ? (
          <p className="text-sm text-muted-foreground">No active facts or decisions yet.</p>
        ) : (
          <div className="space-y-2">
            {positionFacts.length > 0 ? (
              <ul className="flex flex-wrap gap-x-6 gap-y-2">
                {positionFacts.map((f) => (
                  <li key={f.version_id} className="text-sm">
                    <span className="text-muted-foreground">{f.label}</span>
                    <span className="mx-1.5 text-muted-foreground/60">·</span>
                    <span className="font-medium">
                      {f.value_text}
                      {f.unit ? ` ${f.unit}` : ""}
                    </span>
                    <span className="ml-1.5 text-xs text-muted-foreground">
                      ({f.evidence_count} evidence)
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
            {positionDecisions.length > 0 ? (
              <ul className="space-y-1">
                {positionDecisions.map((d) => (
                  <li key={d.decision_id} className="text-sm">
                    <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
                      Decision
                    </span>{" "}
                    <span className="font-medium">{d.statement}</span>
                    <span className="ml-1.5 text-xs text-muted-foreground">
                      ({d.evidence_count} evidence)
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        )}
      </section>

      <NeedsConfirmation
        rows={data.confirmationRows}
        loading={
          data.factsQuery.isLoading ||
          data.decisionsQuery.isLoading ||
          data.contradictionsQuery.isLoading
        }
        actions={{
          confirmFact: (versionID, supersedesVersionID) =>
            confirmFactMutation.mutate({ versionID, supersedesVersionID }),
          rejectFact: (versionID) => rejectFactMutation.mutate(versionID),
          confirmDecision: (decisionID) => confirmDecisionMutation.mutate(decisionID),
          withdrawDecision: (decisionID) => withdrawDecisionMutation.mutate(decisionID),
          resolveContradiction: (contradictionID, resolution, keepFactVersionID) =>
            resolveContradictionMutation.mutate({
              id: contradictionID,
              resolution,
              keep_fact_version_id: keepFactVersionID,
            }),
          busy:
            confirmFactMutation.isPending ||
            rejectFactMutation.isPending ||
            confirmDecisionMutation.isPending ||
            withdrawDecisionMutation.isPending ||
            resolveContradictionMutation.isPending,
        }}
      />

      <section aria-label="Ask Project AI" className="space-y-2 border-b border-border/70 pb-4">
        <h2 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Ask Project AI
        </h2>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input
            value={askQuestion}
            onChange={(e) => setAskQuestion(e.target.value)}
            placeholder="Ask a grounded question about this project"
            disabled={!data.llmEnabled || askMutation.isPending}
            onKeyDown={(e) => {
              if (e.key === "Enter" && askQuestion.trim()) askMutation.mutate();
            }}
          />
          <Button
            variant="outline"
            disabled={!data.llmEnabled || !askQuestion.trim() || askMutation.isPending}
            title={
              data.llmEnabled
                ? "Answer from project facts, decisions, and correspondence"
                : "Configure LLM_BASE_URL and LLM_MODEL on the API"
            }
            onClick={() => askMutation.mutate()}
          >
            {askMutation.isPending ? "Asking…" : data.llmEnabled ? "Ask" : "Ask (LLM off)"}
          </Button>
        </div>
        {askAnswer ? (
          <div className="space-y-1 text-sm">
            <p>{askAnswer.answer}</p>
            {askAnswer.citations.length > 0 ? (
              <p className="text-xs text-muted-foreground">
                Citations:{" "}
                {askAnswer.citations.map((c) => `${c.type}:${c.id.slice(0, 8)}`).join(", ")}
              </p>
            ) : null}
          </div>
        ) : null}
      </section>

      <div
        role="tablist"
        aria-label="Project workspace mode"
        className="flex flex-wrap gap-2 border-b border-border/70 pb-3"
      >
        {PROJECT_MODES.map((item) => (
          <Button
            key={item.id}
            role="tab"
            aria-selected={mode === item.id}
            size="sm"
            variant={mode === item.id ? "default" : "outline"}
            onClick={() => setMode(item.id)}
          >
            {item.label}
          </Button>
        ))}
      </div>

      {mode === "trail" ? (
        <TrailMode
          items={data.timeline}
          loading={data.timelineQuery.isLoading}
          filters={filters}
          onFiltersChange={setFilters}
          projectID={id!}
          issues={data.openIssues}
          accountFor={accountFor}
          attaching={attachMutation.isPending}
          onAttach={(issueID, item) =>
            attachMutation.mutate({
              issueID,
              messageID: item.message_id,
              manualItemID: item.manual_item_id,
            })
          }
          onCreateIssue={(item) => {
            setPendingItemRefs([itemRef(item)].filter((r) => r.message_id || r.manual_item_id));
            setNewIssueTitle(item.title?.trim() || "");
            setIssueDialogHint("Pre-attached from timeline");
            setCreateIssueOpen(true);
          }}
          onAddFactEvidence={(item) => {
            setFactEvidence([itemRef(item)].filter((r) => r.message_id || r.manual_item_id));
            setFactLabel(item.title?.trim() || "");
            setCreateFactOpen(true);
          }}
        />
      ) : null}

      {mode === "position" ? (
        <PositionMode
          facts={data.facts}
          factsLoading={data.factsQuery.isLoading}
          decisions={data.decisions}
          decisionsLoading={data.decisionsQuery.isLoading}
          onAddFact={() => setCreateFactOpen(true)}
          onAddDecision={() => setCreateDecisionOpen(true)}
        />
      ) : null}

      {mode === "open" ? (
        <OpenMode
          attention={data.attention}
          attentionLoading={data.attentionQuery.isLoading}
          issues={data.openIssues}
          issuesLoading={data.issuesQuery.isLoading}
          projectID={id!}
          onNewIssue={() => setCreateIssueOpen(true)}
          onDiscard={(issueID) => discardIssueMutation.mutate(issueID)}
          discarding={discardIssueMutation.isPending}
        />
      ) : null}
    </div>
  );
}
