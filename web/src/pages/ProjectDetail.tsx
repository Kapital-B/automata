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
  createManualItem,
  createProjectIssue,
  getProject,
  getProjectTimeline,
  listContacts,
  listProjectIssues,
  suggestProjectIssue,
  updateProject,
  updateProjectMember,
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
  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [discipline, setDiscipline] = useState("");
  const [scope, setScope] = useState("");

  useEffect(() => {
    if (!projectQuery.data) return;
    setName(projectQuery.data.name);
    setRole(projectQuery.data.member?.role ?? "");
    setDiscipline(projectQuery.data.member?.discipline ?? "");
    setScope(projectQuery.data.member?.current_scope ?? "");
  }, [projectQuery.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      await updateProject(accessToken, id, { name: name.trim() });
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
        description="Correspondence timeline with an issues rail for trails."
        actions={
          <div className="flex flex-wrap gap-2">
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
              disabled={suggestIssueMutation.isPending}
              onClick={() => suggestIssueMutation.mutate()}
            >
              {suggestIssueMutation.isPending ? "Suggesting…" : "Suggest issue"}
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

      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_220px]">
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
              issues={issuesQuery.data ?? []}
              attaching={attachMutation.isPending}
              onAttach={(issueID) =>
                attachMutation.mutate({
                  issueID,
                  messageID: item.message_id,
                  manualItemID: item.manual_item_id,
                })
              }
            />
          ))}
        </ol>
      )}
        </div>

        <aside className="space-y-3 lg:border-l lg:border-border/70 lg:pl-4">
          <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
            Issues
          </h2>
          {issuesQuery.isLoading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          ) : (issuesQuery.data ?? []).length === 0 ? (
            <p className="text-xs text-muted-foreground">No issues yet.</p>
          ) : (
            <ul className="space-y-2">
              {(issuesQuery.data ?? []).map((iss) => (
                <li key={iss.id}>
                  <Link
                    to={`/projects/${id}/issues/${iss.id}`}
                    className="block text-sm hover:underline"
                  >
                    <span className="font-medium">{iss.title}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">
                      {iss.status}
                      {iss.awaiting_me ? " · awaiting you" : ""}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </aside>
      </div>
    </div>
  );
}

function TimelineRow({
  item,
  account,
  projectID,
  issues,
  attaching,
  onAttach,
}: {
  item: TimelineItem;
  account: UiAccount | undefined;
  projectID: string;
  issues: IssueListItem[];
  attaching: boolean;
  onAttach: (issueID: string) => void;
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
      {!item.issue_id && issues.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2 pt-1">
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
        </div>
      ) : null}
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
