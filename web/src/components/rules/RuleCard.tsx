import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ChevronDown, CheckCircle2, Clock, Pencil, Trash2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountBadge } from "@/components/AccountBadge";
import { ConfirmAction, Tag } from "@/components/ConnectionCard";
import { useAuth } from "@/components/auth/AuthProvider";
import { relativeTime, type UiAccount } from "@/lib/accounts";
import { cn } from "@/lib/utils";
import { listForwardRuleActivity, type CategoryDefinition, type ForwardActivity, type ForwardRule } from "@/lib/auth";
import { coversExistingMail, describeCondition, ruleState } from "@/lib/forwardRules";

function formatDay(iso: string) {
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

function RuleStatus({ state }: { state: ReturnType<typeof ruleState> }) {
  if (state === "on") {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 text-xs font-medium text-success">
        <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-success" /> On
      </span>
    );
  }
  if (state === "blocked") {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
        <AlertTriangle aria-hidden="true" className="h-3 w-3" /> Blocked
      </span>
    );
  }
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
      Paused
    </span>
  );
}

export function RuleCard({
  rule,
  account,
  categories,
  showAccount,
  busy,
  onSwitchOn,
  onSwitchOff,
  onEdit,
  onDelete,
}: {
  rule: ForwardRule;
  account?: UiAccount;
  categories: CategoryDefinition[];
  showAccount: boolean;
  busy: boolean;
  onSwitchOn: () => void;
  onSwitchOff: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [showActivity, setShowActivity] = useState(false);
  const state = ruleState(rule);
  const s = rule.stats;
  const scope = coversExistingMail(rule) ? "Covers existing mail too" : `Mail from ${formatDay(rule.applies_from)} on`;

  return (
    <li className="surface-card overflow-hidden">
      <div className="flex flex-wrap items-start justify-between gap-4 p-5">
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="truncate font-display text-lg font-medium">{rule.name}</h3>
            <Tag>{rule.mode === "llm" ? "AI" : "Conditions"}</Tag>
            {showAccount && <AccountBadge account={account} />}
          </div>
          <p className="text-sm">{describeCondition(rule.condition_json, categories)}</p>
          <p className="text-sm text-muted-foreground">
            Forwards to <span className="font-mono text-foreground/80">{rule.forward_to}</span>
            {rule.enabled && <> · {scope}</>}
          </p>
          <p className="text-xs text-muted-foreground">
            {s.forwarded} forwarded
            {s.failed > 0 && <> · {s.failed} failed</>}
            {s.pending > 0 && <> · {s.pending} waiting</>}
            {s.last_forwarded_at && <> · last forwarded {relativeTime(s.last_forwarded_at)}</>}
          </p>
        </div>
        <div className="flex items-center gap-3">
          <RuleStatus state={state} />
          <Switch
            checked={rule.enabled}
            disabled={busy}
            aria-label={rule.enabled ? `Switch off ${rule.name}` : `Switch on ${rule.name}`}
            onCheckedChange={(on) => (on ? onSwitchOn() : onSwitchOff())}
          />
        </div>
      </div>

      {state === "blocked" && (
        <div
          role="alert"
          className="mx-5 mb-4 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"
        >
          <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
          <span>Not running: {rule.blocked_reason}. Edit the rule or add the address back to the allowlist.</span>
        </div>
      )}

      {showActivity && <RuleActivity ruleID={rule.id} />}

      <div className="flex flex-wrap items-center gap-2 border-t border-border/70 bg-muted/30 px-5 py-3">
        <Button size="sm" variant="outline" onClick={onEdit} disabled={busy}>
          <Pencil aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Edit
        </Button>
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={showActivity}
          onClick={() => setShowActivity((v) => !v)}
          className="text-muted-foreground"
        >
          <ChevronDown
            aria-hidden="true"
            className={cn("mr-1.5 h-3.5 w-3.5 transition-transform motion-reduce:transition-none", showActivity && "rotate-180")}
          />
          Activity
        </Button>
        <ConfirmAction
          title={`Delete “${rule.name}”?`}
          description="The rule and its history are removed. Mail it already forwarded stays forwarded."
          confirmLabel="Delete rule"
          destructive
          onConfirm={onDelete}
          trigger={(open) => (
            <Button
              size="sm"
              variant="ghost"
              className="ml-auto text-muted-foreground hover:text-destructive"
              onClick={open}
              disabled={busy}
            >
              <Trash2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Delete
            </Button>
          )}
        />
      </div>
    </li>
  );
}

function ActivityIcon({ row }: { row: ForwardActivity }) {
  if (row.pending) return <Clock aria-label="Waiting" className="h-4 w-4 shrink-0 text-muted-foreground" />;
  if (row.status === "forwarded") return <CheckCircle2 aria-label="Forwarded" className="h-4 w-4 shrink-0 text-success" />;
  return <XCircle aria-label="Failed" className="h-4 w-4 shrink-0 text-destructive" />;
}

function RuleActivity({ ruleID }: { ruleID: string }) {
  const { accessToken } = useAuth();
  const q = useQuery({
    queryKey: ["forward-activity", accessToken, ruleID],
    enabled: Boolean(accessToken),
    queryFn: () => listForwardRuleActivity(accessToken!, ruleID),
  });
  return (
    <section aria-label="Recent activity" className="border-t border-border/70">
      {q.isLoading ? (
        <div className="space-y-2 px-5 py-4">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-4 w-52" />
        </div>
      ) : q.isError ? (
        <p className="px-5 py-4 text-sm text-destructive">Could not load activity.</p>
      ) : (q.data ?? []).length === 0 ? (
        <p className="px-5 py-4 text-sm text-muted-foreground">Nothing forwarded, failed or waiting yet.</p>
      ) : (
        <ul className="divide-y divide-border/70">
          {(q.data ?? []).map((row) => (
            <li key={row.message_id} className="flex items-start gap-3 px-5 py-2.5">
              <ActivityIcon row={row} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{row.subject || "(no subject)"}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {row.from_name || row.from_address} · {row.reason}
                </p>
              </div>
              <time dateTime={row.at} className="shrink-0 text-xs text-muted-foreground">
                {relativeTime(row.at)}
              </time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
