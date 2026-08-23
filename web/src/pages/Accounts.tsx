import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { relativeTime } from "@/lib/accounts";
import { Plus, RefreshCw, Unplug, AlertTriangle, Loader2, Slack } from "lucide-react";
import { useState, useEffect, useMemo } from "react";
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
import {
  ApiError,
  createConnectorBinding,
  deleteAccount,
  deleteConnector,
  listConnectorBindings,
  listConnectors,
  listProjects,
  startConnectorConnect,
  startMailboxConnect,
  syncAccount,
  syncConnector,
  type ConnectorAccount,
  type ConnectorBinding,
  type ProjectListItem,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export default function AccountsPage() {
  const [kind, setKind] = useState<"work" | "personal" | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [searchParams] = useSearchParams();
  const connectedAccountID = searchParams.get("connected_account_id");
  const connectedConnectorID = searchParams.get("connected_connector_id");
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const { accounts, isLoading, isError, error } = useAccountsData();

  const connectorsQuery = useQuery({
    queryKey: ["connectors", accessToken],
    enabled: Boolean(accessToken),
    queryFn: () => listConnectors(accessToken!),
  });
  const projectsQuery = useQuery({
    queryKey: ["projects", accessToken],
    enabled: Boolean(accessToken),
    queryFn: () => listProjects(accessToken!),
  });

  const highlightActive = useMemo(() => Boolean(connectedAccountID), [connectedAccountID]);
  const connectors = connectorsQuery.data ?? [];
  const projects = projectsQuery.data ?? [];

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

  useEffect(() => {
    if (connectedConnectorID) {
      void queryClient.invalidateQueries({ queryKey: ["connectors"] });
      toast({
        title: "Slack connected",
        description: "Bind a channel to a project, then sync.",
      });
      const timer = window.setTimeout(() => {
        const url = new URL(window.location.href);
        url.searchParams.delete("connected_connector_id");
        url.searchParams.delete("connector");
        window.history.replaceState(null, document.title, `${url.pathname}${url.search}${url.hash}`);
      }, 6000);
      return () => window.clearTimeout(timer);
    }
  }, [connectedConnectorID, queryClient]);

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

  const slackConnectMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return startConnectorConnect(accessToken, "slack");
    },
    onSuccess: (res) => {
      window.location.assign(res.authorization_url);
    },
    onError: (err) => {
      toast({
        title: "Could not start Slack connect",
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
        title: "Sync queued",
        description: `Run ${result.job_run_id.slice(0, 8)} started in background.`,
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

  const slackSyncMutation = useMutation({
    mutationFn: async (connectorID: string) => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return syncConnector(accessToken, connectorID);
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ["connectors"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      void queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
      toast({
        title: result.status === "queued" ? "Slack sync queued" : "Slack sync complete",
        description:
          typeof result.messages_upserted === "number"
            ? `${result.messages_upserted} message(s) upserted.`
            : `Run ${result.job_run_id.slice(0, 8)}.`,
      });
    },
    onError: (err) => {
      toast({
        title: "Slack sync failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  const slackDisconnectMutation = useMutation({
    mutationFn: async (connectorID: string) => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      await deleteConnector(accessToken, connectorID);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["connectors"] });
      toast({ title: "Slack disconnected" });
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
    <div className="space-y-10">
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
                        : "border-border hover:border-foreground/40",
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
                          : "border-[hsl(28_70%_52%/0.3)] bg-[hsl(28_70%_52%/0.10)] text-[hsl(28_70%_38%)]",
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

      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="text-xs uppercase tracking-wider text-muted-foreground">Connectors</p>
            <h2 className="font-display text-2xl font-medium">Slack</h2>
            <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
              Bind Slack channels to projects so messages land on the trail. Fake mode uses channel{" "}
              <code className="rounded bg-muted px-1 py-0.5 text-xs">C_FAKE_DC01</code>.
            </p>
          </div>
          <Button
            size="sm"
            className="bg-foreground text-background hover:bg-foreground/90"
            onClick={() => slackConnectMutation.mutate()}
            disabled={slackConnectMutation.isPending}
          >
            {slackConnectMutation.isPending ? (
              <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Slack className="mr-1.5 h-3.5 w-3.5" />
            )}
            Connect Slack
          </Button>
        </div>

        {connectorsQuery.isLoading && (
          <div className="surface-card p-5 text-sm text-muted-foreground">Loading Slack workspaces…</div>
        )}
        {connectorsQuery.isError && (
          <div className="surface-card p-5 text-sm text-destructive">
            Could not load connectors:{" "}
            {connectorsQuery.error instanceof Error ? connectorsQuery.error.message : "unknown error"}
          </div>
        )}
        {!connectorsQuery.isLoading && !connectorsQuery.isError && connectors.length === 0 && (
          <div className="surface-card p-6 text-sm text-muted-foreground">
            No Slack workspace connected yet.
          </div>
        )}

        <ul className="space-y-3">
          {connectors.map((connector) => (
            <SlackConnectorCard
              key={connector.id}
              connector={connector}
              projects={projects}
              highlighted={connectedConnectorID === connector.id}
              syncing={slackSyncMutation.isPending}
              disconnecting={slackDisconnectMutation.isPending}
              onSync={() => slackSyncMutation.mutate(connector.id)}
              onDisconnect={() => {
                if (window.confirm(`Disconnect ${connector.label}?`)) {
                  slackDisconnectMutation.mutate(connector.id);
                }
              }}
            />
          ))}
        </ul>
      </section>
    </div>
  );
}

function SlackConnectorCard({
  connector,
  projects,
  highlighted,
  syncing,
  disconnecting,
  onSync,
  onDisconnect,
}: {
  connector: ConnectorAccount;
  projects: ProjectListItem[];
  highlighted: boolean;
  syncing: boolean;
  disconnecting: boolean;
  onSync: () => void;
  onDisconnect: () => void;
}) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [channelID, setChannelID] = useState("C_FAKE_DC01");
  const [projectID, setProjectID] = useState("");
  const [label, setLabel] = useState("#dc01-project");

  const bindingsQuery = useQuery({
    queryKey: ["connector-bindings", accessToken, connector.id],
    enabled: Boolean(accessToken),
    queryFn: () => listConnectorBindings(accessToken!, connector.id),
  });

  const bindMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return createConnectorBinding(accessToken, connector.id, {
        external_channel_id: channelID.trim(),
        project_id: projectID || undefined,
        label: label.trim() || undefined,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["connector-bindings", accessToken, connector.id] });
      toast({ title: "Channel bound", description: "Sync to pull messages onto the project trail." });
    },
    onError: (err) => {
      toast({
        title: "Bind failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  const bindings: ConnectorBinding[] = bindingsQuery.data ?? [];
  const projectName = (id?: string) => projects.find((p) => p.id === id)?.name ?? id ?? "Unassigned";

  return (
    <li
      className={cn(
        "surface-card p-5",
        highlighted && "ring-2 ring-success/50 transition-shadow",
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <Slack className="h-4 w-4 text-muted-foreground" />
            <h3 className="font-display text-lg font-medium">{connector.label}</h3>
            <span className="rounded-full border border-border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider">
              {connector.provider}
            </span>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            Last sync {relativeTime(connector.last_synced_at)}
          </p>
          {connector.last_error ? (
            <p className="mt-2 text-sm text-destructive">{connector.last_error}</p>
          ) : null}
        </div>
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium",
            connector.connection_status === "connected"
              ? "bg-success/10 text-success"
              : "bg-destructive/10 text-destructive",
          )}
        >
          {connector.connection_status}
        </span>
      </div>

      <div className="mt-4 space-y-2">
        <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Bindings</p>
        {bindings.length === 0 ? (
          <p className="text-sm text-muted-foreground">No channels bound yet.</p>
        ) : (
          <ul className="space-y-1 text-sm">
            {bindings.map((b) => (
              <li key={b.id} className="flex flex-wrap gap-x-2 text-muted-foreground">
                <span className="font-medium text-foreground">{b.label || b.external_channel_id}</span>
                <span>→ {projectName(b.project_id)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
        <input
          className="h-9 rounded-md border border-input bg-background px-3 text-sm"
          value={channelID}
          onChange={(e) => setChannelID(e.target.value)}
          placeholder="Channel ID"
          aria-label="Slack channel ID"
        />
        <select
          className="h-9 rounded-md border border-input bg-background px-2 text-sm"
          value={projectID}
          onChange={(e) => setProjectID(e.target.value)}
          aria-label="Project"
        >
          <option value="">Unassigned queue</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.code ? `${p.code} · ${p.name}` : p.name}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          variant="outline"
          disabled={!channelID.trim() || bindMutation.isPending}
          onClick={() => bindMutation.mutate()}
        >
          {bindMutation.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : "Bind"}
        </Button>
      </div>
      <input
        className="mt-2 h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        placeholder="Binding label"
        aria-label="Binding label"
      />

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={onSync} disabled={syncing || disconnecting}>
          <RefreshCw className="mr-1.5 h-3.5 w-3.5" /> Sync now
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="text-muted-foreground hover:text-destructive"
          onClick={onDisconnect}
          disabled={syncing || disconnecting}
        >
          <Unplug className="mr-1.5 h-3.5 w-3.5" /> Disconnect
        </Button>
      </div>
    </li>
  );
}
