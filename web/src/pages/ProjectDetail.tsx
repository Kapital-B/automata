import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import {
  ApiError,
  addIssueItem,
  confirmFactVersion,
  confirmDecision,
  createManualItem,
  createProjectDecision,
  createProjectFact,
  createProjectIssue,
  dismissInterpretation,
  getCurrentPosition,
  getProject,
  getProjectTimeline,
  getApiHealth,
  interpretProject,
  listContacts,
  listProjectContradictions,
  listProjectDecisions,
  listProjectFacts,
  listProjectInterpretations,
  listProjectIssues,
  reconcileProject,
  rejectFactVersion,
  resolveContradiction,
  suggestProjectIssue,
  updateProject,
  updateProjectMember,
  withdrawDecision,
  type Contradiction,
  type Decision,
  type FactDetail,
  type Interpretation,
  type IssueListItem,
  type TimelineItem,
} from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Plus } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { cn } from "@/lib/utils";

const CHANNELS = [
  { value: "teams", label: "Teams" },
  { value: "whatsapp", label: "WhatsApp" },
  { value: "sms", label: "SMS" },
  { value: "call", label: "Call" },
  { value: "meeting", label: "Meeting" },
  { value: "note", label: "Note" },
] as const;

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { accounts } = useAccountsData();
  const [sourceFilter, setSourceFilter] = useState<"all" | "mail" | "manual">("all");
  const [unassignedToIssue, setUnassignedToIssue] = useState(false);
  const [pasteOpen, setPasteOpen] = useState(false);
  const [createIssueOpen, setCreateIssueOpen] = useState(false);
  const [newIssueTitle, setNewIssueTitle] = useState("");
  const [newIssueNote, setNewIssueNote] = useState("");
  const [pendingItemRefs, setPendingItemRefs] = useState<
    { message_id?: string; manual_item_id?: string }[]
  >([]);
  const [suggestMeta, setSuggestMeta] = useState<string | null>(null);
  const [createFactOpen, setCreateFactOpen] = useState(false);
  const [factSubjectKey, setFactSubjectKey] = useState("pump.p03.duty_kw");
  const [factLabel, setFactLabel] = useState("");
  const [factValue, setFactValue] = useState("");
  const [factUnit, setFactUnit] = useState("");
  const [factConfirmNow, setFactConfirmNow] = useState(true);
  const [factEvidence, setFactEvidence] = useState<
    { message_id?: string; manual_item_id?: string }[]
  >([]);
  const [expandedFactID, setExpandedFactID] = useState<string | null>(null);
  const [createDecisionOpen, setCreateDecisionOpen] = useState(false);
  const [decisionStatement, setDecisionStatement] = useState("");
  const [decisionConfirmNow, setDecisionConfirmNow] = useState(true);

  const projectQuery = useQuery({
    queryKey: ["project", accessToken, id],
    queryFn: () => getProject(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });
  const timelineQuery = useQuery({
    queryKey: ["project-timeline", accessToken, id, sourceFilter, unassignedToIssue],
    queryFn: () =>
      getProjectTimeline(accessToken!, id!, {
        source: sourceFilter,
        unassigned_to_issue: unassignedToIssue,
        limit: 100,
      }),
    enabled: Boolean(accessToken && id),
  });
  const issuesQuery = useQuery({
    queryKey: ["project-issues", accessToken, id],
    queryFn: () => listProjectIssues(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });
  const currentPositionQuery = useQuery({
    queryKey: ["project-current-position", accessToken, id],
    queryFn: () => getCurrentPosition(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });
  const factsQuery = useQuery({
    queryKey: ["project-facts", accessToken, id],
    queryFn: () =>
      listProjectFacts(accessToken!, id!, { include: ["proposed", "history"] }),
    enabled: Boolean(accessToken && id),
  });
  const interpretationsQuery = useQuery({
    queryKey: ["project-interpretations", accessToken, id],
    queryFn: () => listProjectInterpretations(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });
  const contradictionsQuery = useQuery({
    queryKey: ["project-contradictions", accessToken, id],
    queryFn: () => listProjectContradictions(accessToken!, id!, "open"),
    enabled: Boolean(accessToken && id),
  });
  const decisionsQuery = useQuery({
    queryKey: ["project-decisions", accessToken, id],
    queryFn: () => listProjectDecisions(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });
  const healthQuery = useQuery({
    queryKey: ["api-health"],
    queryFn: () => getApiHealth(),
    staleTime: 60_000,
  });
  const llmEnabled = healthQuery.data?.llm === true;
  const openIssues = useMemo(
    () => (issuesQuery.data ?? []).filter((iss) => iss.status !== "resolved"),
    [issuesQuery.data],
  );
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
      setSuggestMeta(null);
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
      await queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
    },
    onError: (err) => {
      toast({
        title: "Could not create issue",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const suggestIssueMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return suggestProjectIssue(accessToken, id);
    },
    onSuccess: (res) => {
      setNewIssueTitle(res.title);
      setPendingItemRefs(res.item_refs ?? []);
      const conf = typeof res.confidence === "number" ? Math.round(res.confidence * 100) : null;
      setSuggestMeta(
        [
          conf != null ? `${conf}% confidence` : null,
          res.reason?.trim() || null,
          res.item_refs?.length ? `${res.item_refs.length} item(s) pre-selected` : null,
        ]
          .filter(Boolean)
          .join(" · ") || null,
      );
      setCreateIssueOpen(true);
      toast({ title: "Suggestion ready", description: "Review and create to confirm." });
    },
    onError: (err) => {
      toast({
        title: "Suggest failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });
  const attachMutation = useMutation({
    mutationFn: async (args: {
      issueID: string;
      messageID?: string;
      manualItemID?: string;
    }) => {
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
    onError: (err) => {
      toast({
        title: "Attach failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const invalidateFacts = async () => {
    await queryClient.invalidateQueries({ queryKey: ["project-facts"] });
    await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
  };

  const createFactMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      const trimmed = factValue.trim();
      const asNumber = Number(trimmed);
      const value =
        trimmed !== "" && !Number.isNaN(asNumber) && /^-?\d+(\.\d+)?$/.test(trimmed)
          ? asNumber
          : trimmed;
      const existing = (factsQuery.data ?? []).find((f) => f.subject_key === factSubjectKey.trim());
      const active = existing?.versions.find((v) => v.status === "active");
      return createProjectFact(accessToken, id, {
        subject_key: factSubjectKey.trim(),
        label: factLabel.trim(),
        value,
        unit: factUnit.trim() || undefined,
        confirm: factConfirmNow,
        supersedes_version_id:
          factConfirmNow && active ? active.id : undefined,
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
    onError: (err) => {
      toast({
        title: "Could not save fact",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
    onError: (err) => {
      toast({
        title: "Confirm failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
    onError: (err) => {
      toast({
        title: "Reject failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const interpretMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return interpretProject(accessToken, id);
    },
    onSuccess: async (res) => {
      const n = res.candidates?.length ?? 0;
      toast({
        title: n > 0 ? "Interpretation ready" : "No durable candidates",
        description: n > 0 ? `${n} candidate(s) pending review.` : res.reason || undefined,
      });
      await queryClient.invalidateQueries({ queryKey: ["project-interpretations"] });
    },
    onError: (err) => {
      toast({
        title: "Interpret failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const dismissInterpMutation = useMutation({
    mutationFn: async (interpretationID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return dismissInterpretation(accessToken, interpretationID);
    },
    onSuccess: async () => {
      toast({ title: "Interpretation dismissed" });
      await queryClient.invalidateQueries({ queryKey: ["project-interpretations"] });
    },
    onError: (err) => {
      toast({
        title: "Dismiss failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const reconcileMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return reconcileProject(accessToken, id);
    },
    onSuccess: async (res) => {
      toast({
        title: "Reconcile complete",
        description: `${res.processed_interpretations} interpretation(s); ${res.contradictions_opened} contradiction(s) opened.`,
      });
      await queryClient.invalidateQueries({ queryKey: ["project-interpretations"] });
      await queryClient.invalidateQueries({ queryKey: ["project-contradictions"] });
      await invalidateFacts();
      await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
    },
    onError: (err) => {
      toast({
        title: "Reconcile failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
      await queryClient.invalidateQueries({ queryKey: ["project-current-position"] });
    },
    onError: (err) => {
      toast({
        title: "Resolve failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
    onError: (err) => {
      toast({
        title: "Create decision failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
    onError: (err) => {
      toast({
        title: "Confirm failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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
    onError: (err) => {
      toast({
        title: "Withdraw failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const [name, setName] = useState("");
  const [keywords, setKeywords] = useState("");
  const [role, setRole] = useState("");
  const [discipline, setDiscipline] = useState("");
  const [scope, setScope] = useState("");

  useEffect(() => {
    if (!projectQuery.data) return;
    setName(projectQuery.data.name);
    setKeywords((projectQuery.data.keywords ?? []).join(", "));
    setRole(projectQuery.data.member?.role ?? "");
    setDiscipline(projectQuery.data.member?.discipline ?? "");
    setScope(projectQuery.data.member?.current_scope ?? "");
  }, [projectQuery.data]);

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
    onError: (err) => {
      toast({
        title: "Save failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
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

  if (projectQuery.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading project…
      </div>
    );
  }

  if (projectQuery.isError || !projectQuery.data) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-destructive">
          {projectQuery.error instanceof ApiError
            ? projectQuery.error.message
            : "Project not found."}
        </p>
        <Button variant="outline" onClick={() => navigate("/projects")}>
          Back to Projects
        </Button>
      </div>
    );
  }

  const project = projectQuery.data;
  const items = timelineQuery.data ?? [];

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow={project.code}
        title={project.name}
        description="Correspondence timeline with current position, facts, and an issues rail."
        actions={
          <div className="flex flex-wrap gap-2">
            <Dialog open={createFactOpen} onOpenChange={setCreateFactOpen}>
              <DialogTrigger asChild>
                <Button variant="outline">Add fact</Button>
              </DialogTrigger>
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
              <DialogTrigger asChild>
                <Button variant="outline">Add decision</Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Add decision</DialogTitle>
                  <DialogDescription>
                    Record an approval or go/no-go. Evidence can be attached later via reconcile.
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
                  setSuggestMeta(null);
                  setPendingItemRefs([]);
                }
              }}
            >
              <DialogTrigger asChild>
                <Button variant="outline">New issue</Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Create issue</DialogTitle>
                  <DialogDescription>
                    Default assignee is you. Confirm to create — suggestions are never auto-saved.
                  </DialogDescription>
                </DialogHeader>
                <div className="space-y-3">
                  {suggestMeta ? (
                    <p className="text-xs text-muted-foreground">{suggestMeta}</p>
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
            <Button
              variant="outline"
              disabled={!llmEnabled || suggestIssueMutation.isPending}
              title={
                llmEnabled
                  ? "Propose an issue from unassigned correspondence"
                  : "Configure LLM_BASE_URL and LLM_MODEL on the API to enable suggestions"
              }
              onClick={() => suggestIssueMutation.mutate()}
            >
              {suggestIssueMutation.isPending
                ? "Suggesting…"
                : llmEnabled
                  ? "Suggest issue"
                  : "Suggest (LLM off)"}
            </Button>
            <Button
              variant="outline"
              disabled={!llmEnabled || interpretMutation.isPending}
              title={
                llmEnabled
                  ? "Extract fact/decision candidates from project correspondence"
                  : "Configure LLM_BASE_URL and LLM_MODEL on the API to enable interpret"
              }
              onClick={() => interpretMutation.mutate()}
            >
              {interpretMutation.isPending
                ? "Interpreting…"
                : llmEnabled
                  ? "Interpret"
                  : "Interpret (LLM off)"}
            </Button>
            <Button
              variant="outline"
              disabled={
                reconcileMutation.isPending ||
                (interpretationsQuery.data ?? []).length === 0
              }
              title="Apply pending interpretations (Stage B)"
              onClick={() => reconcileMutation.mutate()}
            >
              {reconcileMutation.isPending ? "Reconciling…" : "Reconcile"}
            </Button>
            <Dialog open={pasteOpen} onOpenChange={setPasteOpen}>
              <DialogTrigger asChild>
                <Button>
                  <Plus className="mr-2 h-4 w-4" />
                  Paste correspondence
                </Button>
              </DialogTrigger>
              <PasteDialog
                projectID={id!}
                accessToken={accessToken!}
                onDone={async () => {
                  setPasteOpen(false);
                  await queryClient.invalidateQueries({
                    queryKey: ["project-timeline", accessToken, id],
                  });
                  await queryClient.invalidateQueries({ queryKey: ["unassigned"] });
                  // Backend may auto-run interpret; refresh shortly.
                  window.setTimeout(() => {
                    void queryClient.invalidateQueries({
                      queryKey: ["project-interpretations"],
                    });
                  }, 800);
                }}
              />
            </Dialog>
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
        className="border-y border-border/70 py-3"
      >
        <h2 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Current position
        </h2>
        {currentPositionQuery.isLoading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : (currentPositionQuery.data?.facts ?? []).length === 0 &&
          (currentPositionQuery.data?.decisions ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">No active facts or decisions yet.</p>
        ) : (
          <div className="space-y-2">
            {(currentPositionQuery.data?.facts ?? []).length > 0 ? (
              <ul className="flex flex-wrap gap-x-6 gap-y-2">
                {(currentPositionQuery.data?.facts ?? []).map((f) => (
                  <li key={f.version_id} className="text-sm">
                    <span className="text-muted-foreground">{f.label}</span>
                    <span className="mx-1.5 text-muted-foreground/60">·</span>
                    <span className="font-medium">
                      {f.value_text}
                      {f.unit ? ` ${f.unit}` : ""}
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
            {(currentPositionQuery.data?.decisions ?? []).length > 0 ? (
              <ul className="space-y-1">
                {(currentPositionQuery.data?.decisions ?? []).map((d) => (
                  <li key={d.decision_id} className="text-sm">
                    <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
                      Decision
                    </span>{" "}
                    <span className="font-medium">{d.statement}</span>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        )}
      </section>

      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_240px]">
        <div className="space-y-4">
      <div className="flex flex-wrap gap-2">
        {(["all", "mail", "manual"] as const).map((s) => (
          <Button
            key={s}
            size="sm"
            variant={sourceFilter === s ? "default" : "outline"}
            onClick={() => setSourceFilter(s)}
          >
            {s === "all" ? "All" : s === "mail" ? "Mail" : "Manual"}
          </Button>
        ))}
        <Button
          size="sm"
          variant={unassignedToIssue ? "default" : "outline"}
          onClick={() => setUnassignedToIssue((v) => !v)}
        >
          Unassigned to issue
        </Button>
      </div>

      {timelineQuery.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading timeline…
        </div>
      ) : items.length === 0 ? (
        <p className="py-8 text-sm text-muted-foreground">
          No correspondence yet. Assign mail to this project or paste a Teams/WhatsApp note.
        </p>
      ) : (
        <ol className="divide-y divide-border/70 border-y border-border/70">
          {items.map((item) => (
            <TimelineRow
              key={
                item.message_id ??
                item.manual_item_id ??
                `${item.source}-${item.occurred_at}-${item.title}`
              }
              item={item}
              account={accountFor(item.account_id)}
              projectID={id!}
              issues={openIssues}
              attaching={attachMutation.isPending}
              onAttach={(issueID) =>
                attachMutation.mutate({
                  issueID,
                  messageID: item.message_id,
                  manualItemID: item.manual_item_id,
                })
              }
              onCreateIssue={() => {
                setPendingItemRefs([
                  item.message_id
                    ? { message_id: item.message_id }
                    : { manual_item_id: item.manual_item_id },
                ].filter((r) => r.message_id || r.manual_item_id));
                setNewIssueTitle(item.title?.trim() || "");
                setSuggestMeta("Pre-attached from timeline");
                setCreateIssueOpen(true);
              }}
              onAddFactEvidence={() => {
                setFactEvidence([
                  item.message_id
                    ? { message_id: item.message_id }
                    : { manual_item_id: item.manual_item_id },
                ].filter((r) => r.message_id || r.manual_item_id));
                setFactLabel(item.title?.trim() || "");
                setCreateFactOpen(true);
              }}
            />
          ))}
        </ol>
      )}
        </div>

        <aside className="space-y-6 lg:border-l lg:border-border/70 lg:pl-4">
          <div className="space-y-3">
            <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
              Interpretations
            </h2>
            {interpretationsQuery.isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : (interpretationsQuery.data ?? []).length === 0 ? (
              <p className="text-xs text-muted-foreground">No pending interpretations.</p>
            ) : (
              <ul className="space-y-3">
                {(interpretationsQuery.data ?? []).map((interp) => (
                  <InterpretationRailItem
                    key={interp.id}
                    interpretation={interp}
                    dismissing={dismissInterpMutation.isPending}
                    onDismiss={() => dismissInterpMutation.mutate(interp.id)}
                  />
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-3">
            <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
              Contradictions
            </h2>
            {contradictionsQuery.isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : (contradictionsQuery.data ?? []).length === 0 ? (
              <p className="text-xs text-muted-foreground">No open contradictions.</p>
            ) : (
              <ul className="space-y-3">
                {(contradictionsQuery.data ?? []).map((c) => (
                  <ContradictionRailItem
                    key={c.id}
                    contradiction={c}
                    resolving={resolveContradictionMutation.isPending}
                    onResolve={(resolution, keep) =>
                      resolveContradictionMutation.mutate({
                        id: c.id,
                        resolution,
                        keep_fact_version_id: keep,
                      })
                    }
                  />
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-3">
            <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
              Facts
            </h2>
            {factsQuery.isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : (factsQuery.data ?? []).length === 0 ? (
              <p className="text-xs text-muted-foreground">No facts yet.</p>
            ) : (
              <ul className="space-y-3">
                {(factsQuery.data ?? []).map((fact) => (
                  <FactRailItem
                    key={fact.id}
                    fact={fact}
                    expanded={expandedFactID === fact.id}
                    onToggle={() =>
                      setExpandedFactID((cur) => (cur === fact.id ? null : fact.id))
                    }
                    confirming={confirmFactMutation.isPending}
                    rejecting={rejectFactMutation.isPending}
                    onConfirm={(versionID, supersedesVersionID) =>
                      confirmFactMutation.mutate({ versionID, supersedesVersionID })
                    }
                    onReject={(versionID) => rejectFactMutation.mutate(versionID)}
                  />
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-3">
            <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
              Decisions
            </h2>
            {decisionsQuery.isLoading ? (
              <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
            ) : (decisionsQuery.data ?? []).length === 0 ? (
              <p className="text-xs text-muted-foreground">No decisions yet.</p>
            ) : (
              <ul className="space-y-3">
                {(decisionsQuery.data ?? []).map((d) => (
                  <DecisionRailItem
                    key={d.id}
                    decision={d}
                    confirming={confirmDecisionMutation.isPending}
                    withdrawing={withdrawDecisionMutation.isPending}
                    onConfirm={() => confirmDecisionMutation.mutate(d.id)}
                    onWithdraw={() => withdrawDecisionMutation.mutate(d.id)}
                  />
                ))}
              </ul>
            )}
          </div>

          <div className="space-y-3">
          <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
            Issues
          </h2>
          {issuesQuery.isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          ) : openIssues.length === 0 ? (
            <p className="text-xs text-muted-foreground">No open issues yet.</p>
          ) : (
            <ul className="space-y-2">
              {openIssues.map((iss) => (
                <li key={iss.id}>
                  <Link
                    to={`/projects/${id}/issues/${iss.id}`}
                    className="block text-sm hover:underline"
                  >
                    <span className="font-medium">{iss.title}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {iss.assignee_label ?? "Unassigned"} · {iss.status}
                      {iss.awaiting_me ? " · awaiting you" : ""}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function DecisionRailItem({
  decision,
  confirming,
  withdrawing,
  onConfirm,
  onWithdraw,
}: {
  decision: Decision;
  confirming: boolean;
  withdrawing: boolean;
  onConfirm: () => void;
  onWithdraw: () => void;
}) {
  return (
    <li className="space-y-1.5 text-sm">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">{decision.status}</p>
      <p>{decision.statement}</p>
      {decision.status === "proposed" ? (
        <div className="flex flex-wrap gap-1.5">
          <Button
            size="sm"
            variant="outline"
            className="h-7 text-xs"
            disabled={confirming}
            onClick={onConfirm}
          >
            Confirm
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 text-xs"
            disabled={withdrawing}
            onClick={onWithdraw}
          >
            Withdraw
          </Button>
        </div>
      ) : null}
    </li>
  );
}

function ContradictionRailItem({
  contradiction,
  resolving,
  onResolve,
}: {
  contradiction: Contradiction;
  resolving: boolean;
  onResolve: (
    resolution: "supersede" | "reject_a" | "reject_b" | "note",
    keepFactVersionID?: string,
  ) => void;
}) {
  const sides = contradiction.sides ?? [];
  const proposed = sides.length >= 2 ? sides[1]?.fact_version_id : sides[0]?.fact_version_id;
  return (
    <li className="space-y-2 text-sm">
      <p className="text-xs">{contradiction.summary}</p>
      <div className="flex flex-wrap gap-1.5">
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={resolving || !proposed}
          onClick={() => onResolve("supersede", proposed)}
        >
          Keep proposed
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          disabled={resolving}
          onClick={() => onResolve("reject_b")}
        >
          Reject proposed
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="h-7 text-xs"
          disabled={resolving}
          onClick={() => onResolve("note")}
        >
          Note only
        </Button>
      </div>
    </li>
  );
}

function InterpretationRailItem({
  interpretation,
  dismissing,
  onDismiss,
}: {
  interpretation: Interpretation;
  dismissing: boolean;
  onDismiss: () => void;
}) {
  const candidates = interpretation.candidates ?? [];
  return (
    <li className="space-y-2 text-sm">
      <p className="text-xs text-muted-foreground">
        {candidates.length} candidate(s)
        {typeof interpretation.confidence === "number"
          ? ` · ${Math.round(interpretation.confidence * 100)}%`
          : ""}
        {interpretation.sources?.length
          ? ` · ${interpretation.sources.length} source(s)`
          : ""}
      </p>
      <ul className="space-y-1.5 border-l border-border/60 pl-2">
        {candidates.length === 0 ? (
          <li className="text-xs text-muted-foreground">Empty payload</li>
        ) : (
          candidates.map((c, idx) => (
            <li key={`${interpretation.id}-${idx}`} className="text-xs">
              <span className="font-medium uppercase tracking-wider text-muted-foreground">
                {c.kind}
              </span>
              {c.kind === "fact" ? (
                <span className="mt-0.5 block">
                  {c.label ?? c.subject_key}
                  {c.value != null
                    ? `: ${typeof c.value === "object" ? JSON.stringify(c.value) : String(c.value)}`
                    : ""}
                  {c.unit ? ` ${c.unit}` : ""}
                </span>
              ) : (
                <span className="mt-0.5 block">{c.statement || "(decision)"}</span>
              )}
              {c.reason ? (
                <span className="mt-0.5 block text-muted-foreground">{c.reason}</span>
              ) : null}
            </li>
          ))
        )}
      </ul>
      <Button
        size="sm"
        variant="outline"
        className="h-7 text-xs"
        disabled={dismissing}
        onClick={onDismiss}
      >
        Dismiss
      </Button>
    </li>
  );
}

function FactRailItem({
  fact,
  expanded,
  onToggle,
  confirming,
  rejecting,
  onConfirm,
  onReject,
}: {
  fact: FactDetail;
  expanded: boolean;
  onToggle: () => void;
  confirming: boolean;
  rejecting: boolean;
  onConfirm: (versionID: string, supersedesVersionID?: string) => void;
  onReject: (versionID: string) => void;
}) {
  const active = fact.versions.find((v) => v.status === "active");
  const proposed = fact.versions.filter((v) => v.status === "proposed");
  const history = fact.versions.filter(
    (v) => v.status === "superseded" || v.status === "rejected",
  );
  const display = active ?? proposed[0];
  return (
    <li className="text-sm">
      <button type="button" className="w-full text-left hover:underline" onClick={onToggle}>
        <span className="font-medium">{fact.label}</span>
        <span className="mt-0.5 block text-xs text-muted-foreground">
          {display
            ? `${display.value_text}${display.unit ? ` ${display.unit}` : ""} · ${display.status}`
            : fact.subject_key}
        </span>
      </button>
      {expanded ? (
        <div className="mt-2 space-y-2 border-l border-border/60 pl-2">
          {proposed.map((v) => (
            <div key={v.id} className="space-y-1">
              <p className="text-xs text-muted-foreground">
                Proposed: {v.value_text}
                {v.unit ? ` ${v.unit}` : ""}
              </p>
              <div className="flex flex-wrap gap-1">
                <Button
                  size="sm"
                  className="h-7 text-xs"
                  disabled={confirming}
                  onClick={() => onConfirm(v.id, active?.id)}
                >
                  Confirm
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="h-7 text-xs"
                  disabled={rejecting}
                  onClick={() => onReject(v.id)}
                >
                  Reject
                </Button>
              </div>
            </div>
          ))}
          {history.length > 0 ? (
            <ul className="space-y-1 text-xs text-muted-foreground">
              {history.map((v) => (
                <li key={v.id}>
                  {v.status}: {v.value_text}
                  {v.unit ? ` ${v.unit}` : ""}
                  {v.evidence?.length
                    ? ` · ${v.evidence.length} evidence`
                    : ""}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}

function TimelineRow({
  item,
  account,
  projectID,
  issues,
  attaching,
  onAttach,
  onCreateIssue,
  onAddFactEvidence,
}: {
  item: TimelineItem;
  account: UiAccount | undefined;
  projectID: string;
  issues: IssueListItem[];
  attaching: boolean;
  onAttach: (issueID: string) => void;
  onCreateIssue: () => void;
  onAddFactEvidence: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [attachIssueID, setAttachIssueID] = useState("");
  const when = useMemo(() => {
    try {
      return new Date(item.occurred_at).toLocaleString();
    } catch {
      return item.occurred_at;
    }
  }, [item.occurred_at]);
  const contactLabel = item.contacts.map((c) => c.display_name).filter(Boolean).join(", ");

  return (
    <li className="space-y-2 py-4">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span className="uppercase tracking-wider">{item.source}</span>
        {item.channel ? <span>· {item.channel}</span> : null}
        <span>· {when}</span>
        {item.source === "mail" ? <AccountBadge account={account} /> : null}
        {item.issue_id ? (
          <Link
            to={`/projects/${projectID}/issues/${item.issue_id}`}
            className="rounded border border-border/70 px-1.5 py-0.5 hover:underline"
          >
            On issue
          </Link>
        ) : null}
      </div>
      {item.source === "mail" && item.message_id && item.account_id ? (
        <Link
          to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
          className="font-medium hover:underline"
        >
          {item.title || "(no subject)"}
        </Link>
      ) : (
        <p className="font-medium">{item.title || "(untitled)"}</p>
      )}
      {contactLabel ? <p className="text-xs text-muted-foreground">{contactLabel}</p> : null}
      {item.snippet ? <p className="text-sm text-foreground/85">{item.snippet}</p> : null}
      {item.source === "manual" && item.body_text ? (
        <div>
          <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => setExpanded((v) => !v)}>
            {expanded ? "Hide paste" : "Show full paste"}
          </Button>
          {expanded ? (
            <pre className="mt-2 whitespace-pre-wrap rounded-md bg-muted/40 p-3 text-xs">
              {item.body_text}
            </pre>
          ) : null}
        </div>
      ) : null}
      {!item.issue_id ? (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          {issues.length > 0 ? (
            <>
              <select
                aria-label="Attach to issue"
                className="h-8 rounded-md border border-input bg-background px-2 text-xs"
                value={attachIssueID}
                onChange={(e) => setAttachIssueID(e.target.value)}
              >
                <option value="">Attach to issue…</option>
                {issues.map((iss) => (
                  <option key={iss.id} value={iss.id}>
                    {iss.title}
                  </option>
                ))}
              </select>
              <Button
                size="sm"
                className="h-8"
                disabled={!attachIssueID || attaching}
                onClick={() => onAttach(attachIssueID)}
              >
                Attach
              </Button>
            </>
          ) : null}
          <Button size="sm" variant="outline" className="h-8" onClick={onCreateIssue}>
            New issue…
          </Button>
          <Button size="sm" variant="outline" className="h-8" onClick={onAddFactEvidence}>
            Add as fact…
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <Button size="sm" variant="outline" className="h-8" onClick={onAddFactEvidence}>
            Add as fact…
          </Button>
        </div>
      )}
    </li>
  );
}

function PasteDialog({
  projectID,
  accessToken,
  onDone,
}: {
  projectID: string;
  accessToken: string;
  onDone: () => Promise<void>;
}) {
  const [channel, setChannel] = useState("teams");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [occurredAt, setOccurredAt] = useState(() => {
    const d = new Date();
    d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
    return d.toISOString().slice(0, 16);
  });
  const [selectedContacts, setSelectedContacts] = useState<string[]>([]);

  const contactsQuery = useQuery({
    queryKey: ["contacts", accessToken, "paste"],
    queryFn: () => listContacts(accessToken, { limit: 100 }),
  });

  const mutation = useMutation({
    mutationFn: async () => {
      const iso = new Date(occurredAt).toISOString();
      return createManualItem(accessToken, {
        channel,
        occurred_at: iso,
        title: title.trim() || channel,
        body_text: body.trim(),
        project_id: projectID,
        participant_contact_ids: selectedContacts,
      });
    },
    onSuccess: async () => {
      toast({ title: "Correspondence added" });
      await onDone();
    },
    onError: (err) => {
      toast({
        title: "Could not paste",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  return (
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Paste correspondence</DialogTitle>
        <DialogDescription>
          Add a Teams, WhatsApp, or other note to this project timeline. Body text is kept as
          evidence and cannot be edited later.
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Channel</label>
          <Select value={channel} onValueChange={setChannel}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CHANNELS.map((c) => (
                <SelectItem key={c.value} value={c.value}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="occurred">
            When
          </label>
          <Input
            id="occurred"
            type="datetime-local"
            value={occurredAt}
            onChange={(e) => setOccurredAt(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="paste-title">
            Title
          </label>
          <Input
            id="paste-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Optional short title"
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="paste-body">
            Body
          </label>
          <Textarea
            id="paste-body"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={6}
            placeholder="Paste the message text…"
          />
        </div>
        {(contactsQuery.data?.length ?? 0) > 0 ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">Participants (optional)</p>
            <ul className="max-h-32 space-y-1 overflow-y-auto text-sm">
              {contactsQuery.data!.map((c) => {
                const checked = selectedContacts.includes(c.id);
                return (
                  <li key={c.id}>
                    <label className="flex cursor-pointer items-center gap-2">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() =>
                          setSelectedContacts((prev) =>
                            checked ? prev.filter((x) => x !== c.id) : [...prev, c.id],
                          )
                        }
                      />
                      <span className={cn(checked && "font-medium")}>{c.display_name}</span>
                    </label>
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}
        <Button
          className="w-full"
          disabled={!body.trim() || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? "Saving…" : "Add to timeline"}
        </Button>
      </div>
    </DialogContent>
  );
}
