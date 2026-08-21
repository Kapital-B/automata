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
  assignManualItem,
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
      kind: "message" | "manual";
      id: string;
      projectID: string;
      scope?: "thread" | "message";
    }) => {
      if (!accessToken) throw new Error("Not authenticated");
      if (args.kind === "manual") {
        return assignManualItem(accessToken, args.id, { project_id: args.projectID });
      }
      return assignMessageProject(accessToken, args.id, {
        project_id: args.projectID,
        scope: args.scope ?? "thread",
        status: "committed",
      });
    },
    onSuccess: async () => {
      toast({ title: "Assigned" });
      await queryClient.invalidateQueries({ queryKey: ["unassigned"] });
      await queryClient.invalidateQueries({ queryKey: ["unassigned-summary"] });
      await queryClient.invalidateQueries({ queryKey: ["project-timeline"] });
    },
    onError: (err) => {
      toast({
        title: "Assign failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const accountFor = (accountID?: string) =>
    accountID ? accounts.find((x) => x.id === accountID) : undefined;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Queue"
        title="Unassigned"
        description="Mail and pasted notes without a committed project, plus provisional suggestions."
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
            : "Could not load unassigned items."}
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
            onAssign={(args) => assignMutation.mutate(args)}
          />
          <Section
            title="Unassigned"
            empty="Queue is clear — sync mail or paste correspondence from a project."
            items={plain}
            projects={projectsQuery.data ?? []}
            accountFor={accountFor}
            assigning={assignMutation.isPending}
            onAssign={(args) => assignMutation.mutate(args)}
          />
        </>
      )}
    </div>
  );
}

type AssignArgs = {
  kind: "message" | "manual";
  id: string;
  projectID: string;
  scope?: "thread" | "message";
};

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
  accountFor: (id?: string) => UiAccount | undefined;
  assigning: boolean;
  onAssign: (args: AssignArgs) => void;
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
              key={item.message_id ?? item.manual_item_id ?? item.occurred_at}
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
  onAssign: (args: AssignArgs) => void;
}) {
  const [projectID, setProjectID] = useState("");
  const isManual = item.kind === "manual";
  const headline = isManual
    ? item.title || item.subject || "(untitled)"
    : item.subject || "(no subject)";
  const from = item.from_json?.name || item.from_json?.address || "";
  const when = item.occurred_at || item.received_at;

  return (
    <li className="space-y-3 py-4">
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          {isManual ? (
            <span className="text-xs uppercase tracking-wider text-muted-foreground">
              {item.channel || "manual"}
            </span>
          ) : (
            <AccountBadge account={account} />
          )}
          {!isManual && item.message_id && item.account_id ? (
            <Link
              to={`/inbox?message_id=${encodeURIComponent(item.message_id)}&account_id=${encodeURIComponent(item.account_id)}`}
              className="font-medium hover:underline"
            >
              {headline}
            </Link>
          ) : (
            <span className="font-medium">{headline}</span>
          )}
        </div>
        {from ? <p className="text-xs text-muted-foreground">{from}</p> : null}
        {when ? (
          <p className="text-xs text-muted-foreground">{new Date(when).toLocaleString()}</p>
        ) : null}
        {item.reason ? (
          <p className="text-xs text-muted-foreground">Reason: {item.reason}</p>
        ) : null}
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
        {isManual ? (
          <Button
            size="sm"
            disabled={!projectID || assigning || !item.manual_item_id}
            onClick={() =>
              onAssign({ kind: "manual", id: item.manual_item_id!, projectID })
            }
          >
            Assign
          </Button>
        ) : (
          <>
            <Button
              size="sm"
              disabled={!projectID || assigning || !item.conversation_id || !item.message_id}
              onClick={() =>
                onAssign({
                  kind: "message",
                  id: item.message_id!,
                  projectID,
                  scope: "thread",
                })
              }
            >
              Assign thread
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!projectID || assigning || !item.message_id}
              onClick={() =>
                onAssign({
                  kind: "message",
                  id: item.message_id!,
                  projectID,
                  scope: "message",
                })
              }
            >
              This message only
            </Button>
          </>
        )}
      </div>
    </li>
  );
}
