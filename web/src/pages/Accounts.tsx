import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { relativeTime } from "@/lib/accounts";
import { Plus, RefreshCw, Unplug, AlertTriangle, Loader2 } from "lucide-react";
import { useState } from "react";
import { useEffect, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { ApiError, deleteAccount, startMailboxConnect, syncAccount } from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export default function AccountsPage() {
  const [kind, setKind] = useState<"work" | "personal" | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [searchParams] = useSearchParams();
  const connectedAccountID = searchParams.get("connected_account_id");
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const { accounts, isLoading, isError, error } = useAccountsData();

  const highlightActive = useMemo(() => Boolean(connectedAccountID), [connectedAccountID]);

  useEffect(() => {
    if (connectedAccountID) {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: "Mailbox connected",
        description: "The newly connected account is highlighted below.",
      });

      const timer = window.setTimeout(() => {
        const url = new URL(window.location.href);
        url.searchParams.delete("connected_account_id");
        window.history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
      }, 6000);
      return () => window.clearTimeout(timer);
    }
  }, [connectedAccountID, queryClient]);

  const connectMutation = useMutation({
    mutationFn: async (selectedKind: "work" | "personal") => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return startMailboxConnect(accessToken, selectedKind);
    },
    onSuccess: (res) => {
      window.location.assign(res.authorization_url);
    },
    onError: (err) => {
      toast({
        title: "Could not start Microsoft connect",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const syncMutation = useMutation({
    mutationFn: async (accountID: string) => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return syncAccount(accessToken, accountID);
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: "Sync started",
        description: `Run ${result.job_run_id.slice(0, 8)} started.`,
      });
    },
    onError: (err) => {
      toast({
        title: "Sync failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  const disconnectMutation = useMutation({
    mutationFn: async (accountID: string) => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      await deleteAccount(accessToken, accountID);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({ title: "Account disconnected" });
    },
    onError: (err) => {
      toast({
        title: "Disconnect failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Settings · Accounts"
        title="Connected mailboxes"
        description="Each connected account is treated as its own source. Every summary, draft, rule, and run is tagged with the account it came from."
        actions={
          <Dialog
            open={dialogOpen}
            onOpenChange={(open) => {
              setDialogOpen(open);
              if (!open) setKind(null);
            }}
          >
            <DialogTrigger asChild>
              <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90">
                <Plus className="mr-1.5 h-3.5 w-3.5" /> Add account
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-md">
              <DialogHeader>
                <DialogTitle className="font-display text-xl">Connect a Microsoft mailbox</DialogTitle>
                <DialogDescription>
                  We'll redirect you to Microsoft to sign in and grant Mail.Read,
                  Mail.Send, and offline access. Your tokens are stored encrypted, per account.
                </DialogDescription>
              </DialogHeader>
              <div className="grid grid-cols-2 gap-3 pt-2">
                {(["work", "personal"] as const).map((k) => (
                  <button
                    key={k}
                    onClick={() => setKind(k)}
                    className={cn(
                      "rounded-lg border p-4 text-left transition",
                      kind === k
                        ? "border-foreground bg-secondary"
                        : "border-border hover:border-foreground/40"
                    )}
                  >
                    <p className="font-display text-base font-medium capitalize">{k}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {k === "work"
                        ? "Microsoft 365 / Entra (organizations)"
                        : "Outlook.com, Hotmail, Live (consumers)"}
                    </p>
                  </button>
                ))}
              </div>
              <Button
                disabled={!kind}
                onClick={() => {
                  if (kind) connectMutation.mutate(kind);
                }}
                className="mt-2 w-full bg-foreground text-background hover:bg-foreground/90"
              >
                {connectMutation.isPending ? (
                  <>
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                    Redirecting...
                  </>
                ) : (
                  "Continue to Microsoft sign-in"
                )}
              </Button>
            </DialogContent>
          </Dialog>
        }
      />

      {isLoading && (
        <div className="surface-card p-5 text-sm text-muted-foreground">Loading connected accounts...</div>
      )}

      {isError && (
        <div className="surface-card p-5 text-sm text-destructive">
          Could not load accounts: {error instanceof Error ? error.message : "unknown error"}
        </div>
      )}

      {!isLoading && !isError && accounts.length === 0 && (
        <div className="surface-card p-6">
          <h3 className="font-display text-lg font-medium">No connected mailboxes yet</h3>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect a Microsoft work or personal mailbox to unlock sync, summaries, and runs.
          </p>
          <Button
            className="mt-4 bg-foreground text-background hover:bg-foreground/90"
            onClick={() => setDialogOpen(true)}
          >
            <Plus className="mr-1.5 h-3.5 w-3.5" /> Add account
          </Button>
        </div>
      )}

      <ul className="space-y-3">
        {accounts.map((a) => (
          <li
            key={a.id}
            className={cn(
              "surface-card p-5",
              highlightActive && connectedAccountID === a.id && "ring-2 ring-success/50 transition-shadow",
            )}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-start gap-3">
                <span
                  className="mt-1.5 acct-dot h-3 w-3"
                  style={{ background: `hsl(var(--${a.colorVar}))` }}
                />
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="font-display text-lg font-medium">{a.label}</h3>
                    <span
                      className={cn(
                        "rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider",
                        a.kind === "work"
                          ? "border-[hsl(184_55%_22%/0.3)] bg-[hsl(184_55%_22%/0.08)] text-[hsl(184_55%_22%)]"
                          : a.kind === "personal"
                            ? "border-[hsl(28_70%_52%/0.3)] bg-[hsl(28_70%_52%/0.10)] text-[hsl(28_70%_38%)]"
                          : "border-[hsl(28_70%_52%/0.3)] bg-[hsl(28_70%_52%/0.10)] text-[hsl(28_70%_38%)]"
                      )}
                    >
                      {a.kind}
                    </span>
                  </div>
                  <p className="mt-0.5 text-sm text-muted-foreground">{a.primaryEmail}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Last sync {relativeTime(a.lastSyncedAt)}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {a.status === "connected" ? (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 text-xs font-medium text-success">
                    <span className="h-1.5 w-1.5 rounded-full bg-success" /> Connected
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
                    <AlertTriangle className="h-3 w-3" /> {a.status}
                  </span>
                )}
              </div>
            </div>
            {a.status !== "connected" && (
              <div className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                {a.lastError ?? "Session expired. Sign in again to resume sync and forwarding."}
              </div>
            )}
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => syncMutation.mutate(a.id)}
                disabled={syncMutation.isPending || disconnectMutation.isPending}
              >
                <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                {a.status === "connected" ? "Sync now" : "Reconnect"}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => {
                  if (window.confirm(`Disconnect ${a.label}?`)) {
                    disconnectMutation.mutate(a.id);
                  }
                }}
                disabled={syncMutation.isPending || disconnectMutation.isPending}
              >
                <Unplug className="mr-1.5 h-3.5 w-3.5" /> Disconnect
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
