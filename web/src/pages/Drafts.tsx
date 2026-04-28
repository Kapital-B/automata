import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { discardDraftSuggestion, listDraftSendAttempts, listDraftSuggestions, saveDraftSuggestion, sendDraftSuggestion } from "@/lib/auth";
import { Send, Trash2, Sparkles } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { cn } from "@/lib/utils";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { relativeTime } from "@/lib/accounts";
import { Link, useSearchParams } from "react-router-dom";
import { toast } from "@/hooks/use-toast";

interface Props {
  accountFilter: AccountFilter;
}

export default function DraftsPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useState<string>("");
  const [searchParams, setSearchParams] = useSearchParams();
  const deepLinkedDraftID = searchParams.get("draft_id");
  const [editableSubject, setEditableSubject] = useState("");
  const [editableBody, setEditableBody] = useState("");
  const draftsQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, accountFilter],
    queryFn: () => listDraftSuggestions(accessToken!, accountFilter === "all" ? undefined : accountFilter),
    enabled: Boolean(accessToken),
    refetchInterval: 3000,
  });
  const visible = useMemo(() => draftsQuery.data ?? [], [draftsQuery.data]);
  const draft = visible.find((d) => d.id === selectedId) ?? visible[0];
  const attemptsQuery = useQuery({
    queryKey: ["draft-send-attempts", accessToken, draft?.id],
    queryFn: () => listDraftSendAttempts(accessToken!, draft!.id),
    enabled: Boolean(accessToken && draft?.id),
    refetchInterval: 3000,
  });
  useEffect(() => {
    if (!draft) return;
    setEditableSubject(draft.subject);
    setEditableBody(draft.body);
  }, [draft]);
  useEffect(() => {
    if (deepLinkedDraftID && visible.find((d) => d.id === deepLinkedDraftID)) {
      setSelectedId(deepLinkedDraftID);
      const next = new URLSearchParams(searchParams);
      next.delete("draft_id");
      setSearchParams(next, { replace: true });
      return;
    }
    if (visible.length === 0) {
      setSelectedId("");
      return;
    }
    if (!selectedId || !visible.find((d) => d.id === selectedId)) {
      setSelectedId(visible[0].id);
    }
  }, [deepLinkedDraftID, searchParams, setSearchParams, visible, selectedId]);
  const getAccount = (id: string) => accounts.find((a) => a.id === id);
  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !draft) return;
      await saveDraftSuggestion(accessToken, draft.id, { subject: editableSubject, body: editableBody });
    },
    onSuccess: () => {
      toast({ title: "Draft saved" });
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions", accessToken, accountFilter] });
    },
    onError: (error) => {
      toast({ title: "Could not save draft", description: error instanceof Error ? error.message : "Unknown error", variant: "destructive" });
    },
  });
  const discardMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !draft) return;
      await discardDraftSuggestion(accessToken, draft.id);
    },
    onSuccess: () => {
      toast({ title: "Draft discarded" });
      setSelectedId("");
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions", accessToken, accountFilter] });
      void queryClient.invalidateQueries({ queryKey: ["runs", accessToken] });
    },
    onError: (error) => {
      toast({ title: "Could not discard draft", description: error instanceof Error ? error.message : "Unknown error", variant: "destructive" });
    },
  });
  const sendMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !draft) return;
      // Persist local edits first so send uses exactly what the user sees.
      await saveDraftSuggestion(accessToken, draft.id, { subject: editableSubject, body: editableBody });
      await sendDraftSuggestion(accessToken, draft.id);
    },
    onSuccess: () => {
      toast({ title: "Draft sent" });
      setSelectedId("");
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions", accessToken, accountFilter] });
      void queryClient.invalidateQueries({ queryKey: ["runs", accessToken] });
    },
    onError: (error) => {
      toast({ title: "Could not send draft", description: error instanceof Error ? error.message : "Unknown error", variant: "destructive" });
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Assistant"
        title="Drafts"
        description="AI-generated reply drafts. Stored locally — sending is always an explicit action."
      />

      {draftsQuery.isLoading && <div className="surface-card p-4 text-sm text-muted-foreground">Loading drafts...</div>}
      {draftsQuery.isError && (
        <div className="surface-card p-4 text-sm text-destructive">
          Could not load drafts: {draftsQuery.error instanceof Error ? draftsQuery.error.message : "unknown error"}
        </div>
      )}
      <div className="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {visible.map((d) => {
            const isSel = draft?.id === d.id;
            return (
              <li
                key={d.id}
                onClick={() => setSelectedId(d.id)}
                className={cn(
                  "cursor-pointer px-4 py-3 transition",
                  isSel ? "bg-secondary/70" : "hover:bg-secondary/40"
                )}
              >
                <div className="flex items-center justify-between">
                  <AccountBadge account={getAccount(d.account_id)} />
                  <span className="text-[11px] text-muted-foreground">
                    {relativeTime(d.created_at)}
                  </span>
                </div>
                <p className="mt-1.5 truncate text-sm font-medium">{d.to_name || d.to_email || "Unknown recipient"}</p>
                <p className="truncate text-sm text-foreground/85">{d.subject}</p>
                <div className="mt-2 flex items-center gap-2 text-[10px] uppercase tracking-wider">
                  <Sparkles className="h-3 w-3 text-accent" />
                  <span className="text-muted-foreground">ready</span>
                </div>
              </li>
            );
          })}
          {visible.length === 0 && (
            <li className="px-4 py-6 text-sm text-muted-foreground">
              No draft suggestions yet. Auto-draft runs will appear here.
            </li>
          )}
        </ul>

        {draft && (
          <div className="surface-card overflow-hidden">
            <div className="border-b border-border/70 px-6 py-4">
              <div className="flex items-center justify-between">
                <AccountBadge account={getAccount(draft.account_id)} showEmail />
                <span className="text-xs text-muted-foreground font-mono">
                  {draft.model || "llm"}
                </span>
              </div>
              <input
                value={editableSubject}
                onChange={(e) => setEditableSubject(e.target.value)}
                className="mt-3 w-full bg-transparent font-display text-xl font-medium focus:outline-none"
              />
              <p className="mt-1 text-sm text-muted-foreground">
                To <span className="text-foreground/85">{draft.to_name || draft.to_email || "Unknown"}</span>{" "}
                &lt;{draft.to_email || "unknown"}&gt;
              </p>
              <Link
                to={`/inbox?message_id=${encodeURIComponent(draft.message_id)}&account_id=${encodeURIComponent(draft.account_id)}`}
                className="mt-2 inline-block text-xs text-primary underline underline-offset-2 hover:text-primary/80"
              >
                Source message
              </Link>
            </div>
            <textarea
              value={editableBody}
              onChange={(e) => setEditableBody(e.target.value)}
              className="min-h-[280px] w-full resize-none bg-transparent px-6 py-5 text-sm leading-relaxed text-foreground focus:outline-none"
            />
            <div className="flex items-center justify-between border-t border-border/70 bg-secondary/40 px-6 py-3">
              <p className="text-xs text-muted-foreground">
                Send goes through <span className="font-medium text-foreground/85">{getAccount(draft.account_id)?.primaryEmail}</span>. Recorded in audit log.
              </p>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => discardMutation.mutate()}
                  disabled={discardMutation.isPending || saveMutation.isPending || sendMutation.isPending}
                >
                  <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Discard
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => saveMutation.mutate()}
                  disabled={discardMutation.isPending || saveMutation.isPending || sendMutation.isPending}
                >
                  Save
                </Button>
                <Button
                  size="sm"
                  className="bg-foreground text-background hover:bg-foreground/90"
                  onClick={() => sendMutation.mutate()}
                  disabled={discardMutation.isPending || saveMutation.isPending || sendMutation.isPending}
                >
                  <Send className="mr-1.5 h-3.5 w-3.5" /> Send
                </Button>
              </div>
            </div>
            <div className="border-t border-border/70 px-6 py-4">
              <h3 className="text-xs uppercase tracking-wider text-muted-foreground">Send attempt history</h3>
              {attemptsQuery.isLoading && (
                <p className="mt-2 text-xs text-muted-foreground">Loading attempts...</p>
              )}
              {attemptsQuery.isError && (
                <p className="mt-2 text-xs text-destructive">
                  Could not load attempts: {attemptsQuery.error instanceof Error ? attemptsQuery.error.message : "unknown error"}
                </p>
              )}
              {!attemptsQuery.isLoading && !attemptsQuery.isError && (attemptsQuery.data?.length ?? 0) === 0 && (
                <p className="mt-2 text-xs text-muted-foreground">No send attempts yet.</p>
              )}
              <ul className="mt-2 space-y-1.5">
                {(attemptsQuery.data ?? []).map((attempt) => (
                  <li key={attempt.id} className="flex items-center justify-between rounded border border-border/60 px-2.5 py-1.5 text-xs">
                    <span className={cn("font-medium", attempt.status === "success" ? "text-success" : "text-destructive")}>
                      {attempt.status}
                    </span>
                    <span className="text-muted-foreground">{relativeTime(attempt.created_at)}</span>
                    <span className="truncate text-muted-foreground max-w-[55%]">
                      {attempt.error_message || attempt.provider_message_id || "sent via graph"}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
