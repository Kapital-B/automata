import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
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
  assignMessageProject,
  listProjects,
  listUnassigned,
  type UnassignedItem,
} from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Inbox, Loader2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router-dom";

export default function UnassignedPage() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const { accounts } = useAccountsData();

  const unassignedQuery = useQuery({
    queryKey: ["unassigned", accessToken, "all"],
    queryFn: () => listUnassigned(accessToken!, { status: "all", limit: 100 }),
    enabled: Boolean(accessToken),
  });
  const projectsQuery = useQuery({
    queryKey: ["projects", accessToken],
    queryFn: () => listProjects(accessToken!),
    enabled: Boolean(accessToken),
  });

  const provisional = useMemo(
    () => (unassignedQuery.data ?? []).filter((i) => i.status === "provisional"),
    [unassignedQuery.data],
  );
  const plain = useMemo(
    () => (unassignedQuery.data ?? []).filter((i) => i.status === "unassigned"),
    [unassignedQuery.data],
  );

  const assignMutation = useMutation({
    mutationFn: async (args: {
      messageID: string;
      projectID: string;
      scope: "thread" | "message";
    }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return assignMessageProject(accessToken, args.messageID, {
        project_id: args.projectID,
        scope: args.scope,
        status: "committed",
      });
    },
    onSuccess: async () => {
      toast({ title: "Assigned" });
      await queryClient.invalidateQueries({ queryKey: ["unassigned"] });
      await queryClient.invalidateQueries({ queryKey: ["unassigned-summary"] });
    },
    onError: (err) => {
      toast({
        title: "Assign failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const accountFor = (accountID: string) => accounts.find((x) => x.id === accountID);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Queue"
        title="Unassigned"
        description="Mail without a committed project, plus provisional suggestions to confirm."
      />

      {unassignedQuery.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading…
        </div>
      ) : unassignedQuery.isError ? (
        <p className="text-sm text-destructive">
          {unassignedQuery.error instanceof ApiError
            ? unassignedQuery.error.message
            : "Could not load unassigned mail."}
        </p>
      ) : (
        <>
          <Section
            title="Needs confirmation"
            empty="No provisional assignments."
            items={provisional}
            projects={projectsQuery.data ?? []}
            accountFor={accountFor}
            assigning={assignMutation.isPending}
            onAssign={(messageID, projectID, scope) =>
              assignMutation.mutate({ messageID, projectID, scope })
            }
          />
          <Section
            title="Unassigned"
            empty="Inbox is clear — sync a mailbox or create a project to start assigning."
            items={plain}
            projects={projectsQuery.data ?? []}
            accountFor={accountFor}
            assigning={assignMutation.isPending}
            onAssign={(messageID, projectID, scope) =>
              assignMutation.mutate({ messageID, projectID, scope })
            }
          />
        </>
      )}
    </div>
  );
}

function Section({
  title,
  empty,
  items,
  projects,
  accountFor,
  assigning,
  onAssign,
}: {
  title: string;
  empty: string;
  items: UnassignedItem[];
  projects: { id: string; name: string; code: string }[];
  accountFor: (id: string) => UiAccount | undefined;
  assigning: boolean;
  onAssign: (messageID: string, projectID: string, scope: "thread" | "message") => void;
}) {
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
        {title}
        <span className="ml-2 font-normal normal-case tracking-normal">({items.length})</span>
      </h2>
      {items.length === 0 ? (
        <div className="flex items-start gap-2 py-4 text-sm text-muted-foreground">
          <Inbox className="mt-0.5 h-4 w-4 opacity-60" />
          <p>{empty}</p>
        </div>
      ) : (
        <ul className="divide-y divide-border/70 border-y border-border/70">
          {items.map((item) => (
            <UnassignedRow
              key={item.message_id}
              item={item}
              projects={projects}
              account={accountFor(item.account_id)}
              assigning={assigning}
              onAssign={onAssign}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

function UnassignedRow({
  item,
  projects,
  account,
  assigning,
  onAssign,
}: {
  item: UnassignedItem;
  projects: { id: string; name: string; code: string }[];
  account: UiAccount | undefined;
  assigning: boolean;
  onAssign: (messageID: string, projectID: string, scope: "thread" | "message") => void;
}) {
  const [projectID, setProjectID] = useState("");
  const from = item.from_json?.name || item.from_json?.address || "Unknown sender";

  return (
    <li className="space-y-3 py-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <AccountBadge account={account} />
            <Link
              to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
              className="font-medium hover:underline"
            >
              {item.subject || "(no subject)"}
            </Link>
          </div>
          <p className="text-xs text-muted-foreground">{from}</p>
          {item.reason ? (
            <p className="text-xs text-muted-foreground">Reason: {item.reason}</p>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Select value={projectID} onValueChange={setProjectID}>
          <SelectTrigger className="w-[200px]">
            <SelectValue placeholder="Choose project" />
          </SelectTrigger>
          <SelectContent>
            {projects.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.code} — {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          size="sm"
          disabled={!projectID || assigning || !item.conversation_id}
          onClick={() => onAssign(item.message_id, projectID, "thread")}
        >
          Assign thread
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={!projectID || assigning}
          onClick={() => onAssign(item.message_id, projectID, "message")}
        >
          This message only
        </Button>
      </div>
    </li>
  );
}
