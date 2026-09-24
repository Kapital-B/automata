import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Hash, Loader2, Plus, RefreshCw, Slack, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import { relativeTime } from "@/lib/accounts";
import {
  ConfirmAction,
  ConnectionCard,
  ConnectionCardBody,
  ConnectionIcon,
  Tag,
} from "@/components/ConnectionCard";
import {
  ApiError,
  createConnectorBinding,
  listConnectorBindings,
  type ConnectorAccount,
  type ConnectorBinding,
  type ProjectListItem,
} from "@/lib/auth";

// The fake Slack client (no client id configured) signs every workspace in
// as this team and serves a single channel.
const FAKE_TEAM_ID = "T_FAKE";
const FAKE_CHANNEL_ID = "C_FAKE_DC01";

export function SlackConnectorCard({
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
  const [adding, setAdding] = useState(false);
  const fake = connector.external_tenant_id === FAKE_TEAM_ID;
  const connected = connector.connection_status === "connected";

  const bindingsQuery = useQuery({
    queryKey: ["connector-bindings", accessToken, connector.id],
    enabled: Boolean(accessToken),
    queryFn: () => listConnectorBindings(accessToken!, connector.id),
  });
  const bindings: ConnectorBinding[] = bindingsQuery.data ?? [];
  const projectName = (id?: string) => {
    if (!id) return undefined;
    const p = projects.find((x) => x.id === id);
    return p ? (p.code ? `${p.code} · ${p.name}` : p.name) : undefined;
  };

  return (
    <ConnectionCard
      icon={<ConnectionIcon icon={Slack} />}
      title={connector.label}
      tags={fake && <Tag>Test workspace</Tag>}
      meta={
        <>
          Last sync {relativeTime(connector.last_synced_at)}
          {bindingsQuery.isSuccess && ` · ${bindings.length} ${bindings.length === 1 ? "channel" : "channels"}`}
        </>
      }
      status={connector.connection_status}
      problem={connector.last_error}
      highlighted={highlighted}
      actions={
        <>
          {!adding && (
            <Button size="sm" variant="outline" onClick={() => setAdding(true)} disabled={!connected || disconnecting}>
              <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add channel
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={onSync} disabled={syncing || disconnecting || !connected}>
            {syncing ? (
              <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
            ) : (
              <RefreshCw aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
            )}
            Sync now
          </Button>
          <ConfirmAction
            title={`Disconnect ${connector.label}?`}
            description="This removes its channel links and deletes every Slack message synced from it. Connecting it again starts with no channels."
            confirmLabel="Disconnect"
            destructive
            onConfirm={onDisconnect}
            trigger={(open) => (
              <Button
                size="sm"
                variant="ghost"
                className="ml-auto text-muted-foreground hover:text-destructive"
                onClick={open}
                disabled={syncing || disconnecting}
              >
                <Unplug aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Disconnect
              </Button>
            )}
          />
        </>
      }
    >
      <ConnectionCardBody label={`Channels in ${connector.label}`}>
        {bindingsQuery.isLoading ? (
          <div className="space-y-2 px-5 py-4">
            <Skeleton className="h-4 w-48" />
            <Skeleton className="h-4 w-40" />
          </div>
        ) : bindings.length === 0 ? (
          <p className="px-5 py-4 text-sm text-muted-foreground">
            No channels yet. Add one to bring its messages onto a project's trail.
          </p>
        ) : (
          <ul className="divide-y divide-border/70">
            {bindings.map((b) => (
              <li key={b.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 px-5 py-2.5 text-sm">
                <span className="flex min-w-0 items-center gap-1.5 font-medium">
                  <Hash aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <span className="truncate">{b.label.replace(/^#/, "") || b.external_channel_id}</span>
                </span>
                <span className="truncate text-muted-foreground">
                  {projectName(b.project_id) ?? "Triage queue"}
                </span>
              </li>
            ))}
          </ul>
        )}
      </ConnectionCardBody>

      {adding && (
        <BindChannelForm
          connectorID={connector.id}
          projects={projects}
          fake={fake}
          onDone={() => setAdding(false)}
        />
      )}
    </ConnectionCard>
  );
}

function BindChannelForm({
  connectorID,
  projects,
  fake,
  onDone,
}: {
  connectorID: string;
  projects: ProjectListItem[];
  fake: boolean;
  onDone: () => void;
}) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [channelID, setChannelID] = useState(fake ? FAKE_CHANNEL_ID : "");
  const [label, setLabel] = useState("");
  const [projectID, setProjectID] = useState("");
  const [error, setError] = useState<string | null>(null);
  const ids = { channel: `${connectorID}-channel`, label: `${connectorID}-label`, project: `${connectorID}-project` };

  const bind = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return createConnectorBinding(accessToken, connectorID, {
        external_channel_id: channelID.trim(),
        project_id: projectID || undefined,
        label: label.trim() ? `#${label.trim().replace(/^#/, "")}` : undefined,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["connector-bindings", accessToken, connectorID] });
      toast({ title: "Channel added", description: "Sync to pull its messages onto the project trail." });
      onDone();
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not add the channel. Please retry."),
  });

  return (
    <form
      className="space-y-4 border-t border-border/70 px-5 py-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (channelID.trim()) bind.mutate();
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={ids.channel}>Channel ID</Label>
          <Input
            id={ids.channel}
            value={channelID}
            onChange={(e) => setChannelID(e.target.value)}
            placeholder="C0123ABCD"
            className="font-mono"
            aria-describedby={`${ids.channel}-help`}
            autoFocus
          />
          <p id={`${ids.channel}-help`} className="text-xs text-muted-foreground">
            {fake
              ? `This test workspace has one channel, ${FAKE_CHANNEL_ID}.`
              : "In Slack, open the channel's details; the ID is at the bottom."}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={ids.label}>
            Name <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Input id={ids.label} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="dc01-project" />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={ids.project}>Project</Label>
        <select
          id={ids.project}
          className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:max-w-sm"
          value={projectID}
          onChange={(e) => setProjectID(e.target.value)}
        >
          <option value="">None — send to the triage queue</option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.code ? `${p.code} · ${p.name}` : p.name}
            </option>
          ))}
        </select>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={!channelID.trim() || bind.isPending}>
          {bind.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
          Add channel
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
