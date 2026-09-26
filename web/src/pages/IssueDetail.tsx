import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useAuth } from "@/components/auth/AuthProvider";
import { TodoItem } from "@/components/TodoItem";
import {
  ApiError,
  getIssue,
  listContacts,
  markActionItemDone,
  removeIssueItem,
  updateIssue,
} from "@/lib/auth";
import { inboxHref } from "@/lib/todos";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

export default function IssueDetailPage() {
  const { id: projectID, issueId } = useParams<{ id: string; issueId: string }>();
  const { accessToken, user } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const issueQuery = useQuery({
    queryKey: ["issue", accessToken, issueId],
    queryFn: () => getIssue(accessToken!, issueId!),
    enabled: Boolean(accessToken && issueId),
  });
  const contactsQuery = useQuery({
    queryKey: ["contacts", accessToken, "issue-assignee"],
    queryFn: () => listContacts(accessToken!, { limit: 100 }),
    enabled: Boolean(accessToken),
  });

  const [title, setTitle] = useState("");
  const [note, setNote] = useState("");
  const [status, setStatus] = useState("open");
  const [assignee, setAssignee] = useState("me");
  // Resolving an issue with open to-dos asks whether they are done too.
  const [confirmResolve, setConfirmResolve] = useState(false);

  useEffect(() => {
    if (!issueQuery.data) return;
    setTitle(issueQuery.data.title);
    setNote(issueQuery.data.current_position_note ?? "");
    setStatus(issueQuery.data.status);
    if (issueQuery.data.assignee_contact_id) {
      setAssignee(`contact:${issueQuery.data.assignee_contact_id}`);
    } else if (issueQuery.data.assignee_user_id) {
      setAssignee("me");
    } else {
      setAssignee("none");
    }
  }, [issueQuery.data]);

  // To-dos, the project's Needs you and Home all read the same items.
  const refreshTodos = () =>
    Promise.all(
      ["attention", "project-attention", "overview", "summary"].map((key) =>
        queryClient.invalidateQueries({ queryKey: [key] }),
      ),
    );

  const saveMutation = useMutation({
    mutationFn: async ({ completeTodos = false }: { completeTodos?: boolean } = {}) => {
      if (!accessToken || !issueId) throw new Error("Not authenticated");
      const body: Record<string, unknown> = {
        title: title.trim(),
        current_position_note: note.trim(),
        status,
      };
      if (completeTodos) body.complete_todos = true;
      if (assignee === "none") {
        body.assignee_user_id = null;
        body.assignee_contact_id = null;
      } else if (assignee === "me") {
        body.assignee_user_id = user?.userId ?? null;
        body.assignee_contact_id = null;
      } else if (assignee.startsWith("contact:")) {
        body.assignee_user_id = null;
        body.assignee_contact_id = assignee.slice("contact:".length);
      }
      return updateIssue(accessToken, issueId, body);
    },
    onSuccess: async (_data, vars) => {
      setConfirmResolve(false);
      toast({ title: vars?.completeTodos ? "Issue resolved and to-dos done" : "Issue saved" });
      await queryClient.invalidateQueries({ queryKey: ["issue", accessToken, issueId] });
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
      await refreshTodos();
    },
    onError: (err) => {
      toast({
        title: "Save failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const todoMutation = useMutation({
    mutationFn: async (actionItemID: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return markActionItemDone(accessToken, actionItemID);
    },
    onSuccess: async () => {
      toast({ title: "To-do done" });
      await queryClient.invalidateQueries({ queryKey: ["issue", accessToken, issueId] });
      await refreshTodos();
    },
    onError: (err) => {
      toast({
        title: "Could not mark the to-do done",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const detachMutation = useMutation({
    mutationFn: async (itemID: string) => {
      if (!accessToken || !issueId) throw new Error("Not authenticated");
      return removeIssueItem(accessToken, issueId, itemID);
    },
    onSuccess: async () => {
      toast({ title: "Detached" });
      await queryClient.invalidateQueries({ queryKey: ["issue", accessToken, issueId] });
      await queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
    },
  });

  if (issueQuery.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading issue…
      </div>
    );
  }

  if (issueQuery.isError || !issueQuery.data) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-destructive">Issue not found.</p>
        <Button variant="outline" onClick={() => navigate(`/projects/${projectID}`)}>
          Back to project
        </Button>
      </div>
    );
  }

  const issue = issueQuery.data;
  const todos = issue.todos ?? [];
  const save = () => {
    if (status === "resolved" && issue.status !== "resolved" && todos.length > 0) {
      setConfirmResolve(true);
      return;
    }
    saveMutation.mutate({});
  };

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Issue trail"
        title={issue.title}
        description={issue.awaiting_me ? "Awaiting you" : `Status: ${issue.status}`}
        actions={
          <Button variant="outline" asChild>
            <Link to={`/projects/${projectID}`}>Back to project</Link>
          </Button>
        }
      />

      {todos.length > 0 && (
        <section aria-labelledby="issue-todos-heading" className="max-w-3xl space-y-3">
          <div>
            <h2 id="issue-todos-heading" className="font-display text-xl font-medium">
              Your to-dos
            </h2>
            <p className="text-sm text-muted-foreground">
              What this issue&apos;s mail asks you to do. Only you see these.
            </p>
          </div>
          <ul aria-label="Your to-dos" className="surface-card divide-y divide-border/70 overflow-hidden">
            {todos.map((t) => (
              <TodoItem
                key={t.id}
                text={t.text}
                href={inboxHref(t.message_id, t.account_id)}
                dueAt={t.due_at}
                pending={todoMutation.isPending}
                onDone={() => todoMutation.mutate(t.id)}
              />
            ))}
          </ul>
        </section>
      )}

      <section className="max-w-xl space-y-3">
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="issue-title">
            Title
          </label>
          <Input id="issue-title" value={title} onChange={(e) => setTitle(e.target.value)} />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="issue-note">
            Current position
          </label>
          <Textarea id="issue-note" value={note} onChange={(e) => setNote(e.target.value)} rows={3} />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="issue-status">
            Status
          </label>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger id="issue-status">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="open">Open</SelectItem>
              <SelectItem value="awaiting_input">Awaiting input</SelectItem>
              <SelectItem value="resolved">Resolved</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="issue-assignee">
            Assignee
          </label>
          <Select value={assignee} onValueChange={setAssignee}>
            <SelectTrigger id="issue-assignee">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="me">Me (profile)</SelectItem>
              <SelectItem value="none">Unassigned</SelectItem>
              {(contactsQuery.data ?? []).map((c) => (
                <SelectItem key={c.id} value={`contact:${c.id}`}>
                  Contact: {c.display_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button disabled={saveMutation.isPending} onClick={save}>
          {saveMutation.isPending ? "Saving…" : "Save"}
        </Button>
      </section>

      <AlertDialog open={confirmResolve} onOpenChange={setConfirmResolve}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Resolve this issue?</AlertDialogTitle>
            <AlertDialogDescription>
              You still have {todos.length} open {todos.length === 1 ? "to-do" : "to-dos"} from this issue&apos;s mail. Mark{" "}
              {todos.length === 1 ? "it" : "them"} done as well?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <Button variant="outline" disabled={saveMutation.isPending} onClick={() => saveMutation.mutate({})}>
              Resolve only
            </Button>
            <Button disabled={saveMutation.isPending} onClick={() => saveMutation.mutate({ completeTodos: true })}>
              Resolve and mark done
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <section className="space-y-3">
        <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
          Trail ({issue.items?.length ?? 0})
        </h2>
        {(issue.items?.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">
            No items yet. Attach mail or pasted notes from the project timeline.
          </p>
        ) : (
          <ol className="divide-y divide-border/70 border-y border-border/70">
            {issue.items.map((item) => (
              <li key={item.id} className="space-y-2 py-4">
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span className="uppercase tracking-wider">{item.source}</span>
                  {item.channel ? <span>· {item.channel}</span> : null}
                  {item.occurred_at ? (
                    <span>· {new Date(item.occurred_at).toLocaleString()}</span>
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
                {item.snippet ? (
                  <p className="text-sm text-foreground/85">{item.snippet}</p>
                ) : null}
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={detachMutation.isPending}
                  onClick={() => detachMutation.mutate(item.id)}
                >
                  Detach
                </Button>
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}
