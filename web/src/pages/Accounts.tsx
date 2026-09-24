import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import type { UiAccount } from "@/lib/accounts";
import { Loader2, Mail, Plus, Slack } from "lucide-react";
import { useState, useEffect, useMemo } from "react";
import { useSearchParams } from "react-router-dom";
import { ConnectMailboxDialog, type ConnectTarget } from "@/components/ConnectMailboxDialog";
import { SlackConnectorCard } from "@/components/SlackConnectorCard";
import { MailboxCard } from "@/components/MailboxCard";
import {
  ConnectionEmpty,
  ConnectionError,
  ConnectionIcon,
  ConnectionSection,
  ConnectionSkeleton,
} from "@/components/ConnectionCard";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import {
  ApiError,
  deleteAccount,
  deleteConnector,
  listConnectors,
  listMailProviders,
  listProjects,
  startConnectorConnect,
  syncAccount,
  syncConnector,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export default function AccountsPage() {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [connectTarget, setConnectTarget] = useState<ConnectTarget | undefined>(undefined);
  const [searchParams, setSearchParams] = useSearchParams();
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
  const providersQuery = useQuery({
    queryKey: ["mail-providers", accessToken],
    enabled: Boolean(accessToken),
    queryFn: () => listMailProviders(accessToken!),
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
        description: "Add a channel to a project, then sync.",
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

  const openConnect = (target?: ConnectTarget) => {
    setConnectTarget(target);
    setDialogOpen(true);
  };

  // Reconnecting goes through the same connect flow; the server matches the
  // mailbox to the existing account and restores it rather than adding one.
  const reconnect = (a: UiAccount) => {
    if (a.provider === "m365") {
      openConnect({ provider: "m365", kind: a.kind === "personal" ? "personal" : "work" });
    } else if (a.provider === "google" || a.provider === "imap") {
      openConnect({ provider: a.provider, email: a.primaryEmail });
    }
  };

  // A password connect has no redirect; it lands the same way an OAuth one
  // does, through ?connected_account_id, so both confirm identically.
  const onPasswordConnected = (accountID: string) => {
    setDialogOpen(false);
    setSearchParams({ connected_account_id: accountID }, { replace: true });
  };

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
    mutationFn: async (args: { accountID: string; force?: boolean }) => {
      if (!accessToken) {
        throw new Error("Not authenticated");
      }
      return syncAccount(accessToken, args.accountID, { force: args.force });
    },
    onSuccess: (_result, args) => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: args.force ? "Full resync queued" : "Sync queued",
        description: "It runs in the background; progress shows on the Runs page.",
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
      toast({ title: "Mailbox disconnected" });
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
        title: result.status === "queued" ? "Sync queued" : "Sync complete",
        description:
          typeof result.messages_upserted === "number"
            ? `${result.messages_upserted} message(s) brought in.`
            : "It runs in the background; progress shows on the Runs page.",
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

  const mailboxBusy = disconnectMutation.isPending;
  const addMailbox = (primary: boolean) => (
    <Button
      size="sm"
      variant={primary ? "default" : "outline"}
      className={primary ? "bg-foreground text-background hover:bg-foreground/90" : undefined}
      onClick={() => openConnect()}
    >
      <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add mailbox
    </Button>
  );
  const addSlack = (primary: boolean) => (
    <Button
      size="sm"
      variant={primary ? "default" : "outline"}
      className={primary ? "bg-foreground text-background hover:bg-foreground/90" : undefined}
      onClick={() => slackConnectMutation.mutate()}
      disabled={slackConnectMutation.isPending}
    >
      {slackConnectMutation.isPending ? (
        <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
      ) : (
        <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
      )}
      Add workspace
    </Button>
  );

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Settings"
        title="Accounts"
        description="The mailboxes and workspaces Automata reads from. Every summary, draft, rule and run is tagged with the account it came from."
      />

      <ConnectMailboxDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        providers={providersQuery.data ?? []}
        target={connectTarget}
        onConnected={onPasswordConnected}
      />

      <ConnectionSection
        id="mailboxes-heading"
        title="Mailboxes"
        description="Mail is synced, summarised and filed to projects. Each mailbox is its own account."
        action={accounts.length > 0 ? addMailbox(false) : undefined}
      >
        {isLoading ? (
          <ConnectionSkeleton label="Loading mailboxes" />
        ) : isError ? (
          <ConnectionError>
            Could not load mailboxes: {error instanceof Error ? error.message : "unknown error"}
          </ConnectionError>
        ) : accounts.length === 0 ? (
          <ConnectionEmpty
            icon={<ConnectionIcon icon={Mail} />}
            message="No mailbox connected yet. Connect Microsoft, Google or any IMAP mailbox."
            action={addMailbox(true)}
          />
        ) : (
          <ul className="space-y-3">
            {accounts.map((a) => (
              <MailboxCard
                key={a.id}
                account={a}
                highlighted={highlightActive && connectedAccountID === a.id}
                syncing={syncMutation.isPending && syncMutation.variables?.accountID === a.id}
                busy={mailboxBusy}
                onSync={(force) => syncMutation.mutate({ accountID: a.id, force })}
                onReconnect={() => reconnect(a)}
                onDisconnect={() => disconnectMutation.mutate(a.id)}
              />
            ))}
          </ul>
        )}
      </ConnectionSection>

      <ConnectionSection
        id="slack-heading"
        title="Slack"
        description="Add a channel to a project and its messages join that project's trail alongside mail."
        action={connectors.length > 0 ? addSlack(false) : undefined}
      >
        {connectorsQuery.isLoading ? (
          <ConnectionSkeleton label="Loading Slack workspaces" />
        ) : connectorsQuery.isError ? (
          <ConnectionError>
            Could not load Slack workspaces:{" "}
            {connectorsQuery.error instanceof Error ? connectorsQuery.error.message : "unknown error"}
          </ConnectionError>
        ) : connectors.length === 0 ? (
          <ConnectionEmpty
            icon={<ConnectionIcon icon={Slack} />}
            message="No Slack workspace connected yet."
            action={addSlack(true)}
          />
        ) : (
          <ul className="space-y-3">
            {connectors.map((connector) => (
              <SlackConnectorCard
                key={connector.id}
                connector={connector}
                projects={projects}
                highlighted={connectedConnectorID === connector.id}
                syncing={slackSyncMutation.isPending && slackSyncMutation.variables === connector.id}
                disconnecting={slackDisconnectMutation.isPending}
                onSync={() => slackSyncMutation.mutate(connector.id)}
                onDisconnect={() => slackDisconnectMutation.mutate(connector.id)}
              />
            ))}
          </ul>
        )}
      </ConnectionSection>
    </div>
  );
}
