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
import { useAuth } from "@/components/auth/AuthProvider";
import {
  ApiError,
  getIssue,
  listContacts,
  removeIssueItem,
  updateIssue,
} from "@/lib/auth";
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

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !issueId) throw new Error("Not authenticated");
      const body: Record<string, unknown> = {
        title: title.trim(),
        current_position_note: note.trim(),
        status,
      };
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
    onSuccess: async () => {
      toast({ title: "Issue saved" });
      await queryClient.invalidateQueries({ queryKey: ["issue", accessToken, issueId] });
      await queryClient.invalidateQueries({ queryKey: ["project-issues"] });
    },
    onError: (err) => {
      toast({
        title: "Save failed",
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
          <label className="text-xs text-muted-foreground">Status</label>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger>
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
          <label className="text-xs text-muted-foreground">Assignee</label>
          <Select value={assignee} onValueChange={setAssignee}>
            <SelectTrigger>
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
        <Button disabled={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
          {saveMutation.isPending ? "Saving…" : "Save"}
        </Button>
      </section>

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
