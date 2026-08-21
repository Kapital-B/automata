import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  createForwardRule,
  deleteForwardRule,
  getForwardAllowlist,
  listForwardRules,
  putForwardAllowlist,
  runForwardRules,
  type ForwardRule,
  updateForwardRule,
} from "@/lib/auth";
import { Plus, ShieldCheck, X } from "lucide-react";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/hooks/use-toast";
import { useMemo, useState } from "react";

interface Props {
  accountFilter: AccountFilter;
}

export default function RulesPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const accountID = accountFilter === "all" ? accounts[0]?.id : accountFilter;
  const [newAllow, setNewAllow] = useState("");
  const [newRuleName, setNewRuleName] = useState("Invoice forward");
  const [newRuleMode, setNewRuleMode] = useState<"logic" | "llm">("logic");
  const [newForwardTo, setNewForwardTo] = useState("");
  const [newConditionJSON, setNewConditionJSON] = useState(
    JSON.stringify({ all: [{ field: "has_attachments", op: "equals", value: true }, { field: "category_slug", op: "equals", value: "invoice" }] }),
  );

  const allowlistQuery = useQuery({
    queryKey: ["forward-allowlist", accessToken],
    queryFn: () => getForwardAllowlist(accessToken!),
    enabled: Boolean(accessToken),
  });
  const rulesQuery = useQuery({
    queryKey: ["forward-rules", accessToken, accountID],
    queryFn: () => listForwardRules(accessToken!, accountID!),
    enabled: Boolean(accessToken && accountID),
  });
  const allowlist = useMemo(() => allowlistQuery.data?.emails ?? [], [allowlistQuery.data?.emails]);
  const visible = useMemo(() => rulesQuery.data ?? [], [rulesQuery.data]);
  const getAccount = (id: string) => accounts.find((a) => a.id === id);

  const saveAllowlist = useMutation({
    mutationFn: async (emails: string[]) => {
      if (!accessToken) throw new Error("Not authenticated");
      await putForwardAllowlist(accessToken, emails);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["forward-allowlist"] });
      toast({ title: "Allowlist updated" });
    },
    onError: (e) => {
      toast({
        title: "Could not update allowlist",
        description: e instanceof Error ? e.message : "Unknown error",
        variant: "destructive",
      });
    },
  });
  const createRule = useMutation({
    mutationFn: async () => {
      if (!accessToken || !accountID) throw new Error("No account selected");
      await createForwardRule(accessToken, accountID, {
        name: newRuleName,
        mode: newRuleMode,
        condition_json: JSON.parse(newConditionJSON),
        forward_to: newForwardTo,
        enabled: false,
      });
    },
    onSuccess: () => {
      toast({ title: "Rule created (paused)", description: "Enable the rule when you are ready to allow auto-forwarding." });
      void queryClient.invalidateQueries({ queryKey: ["forward-rules"] });
    },
    onError: (e) => toast({ title: "Could not create rule", description: e instanceof Error ? e.message : "Unknown error", variant: "destructive" }),
  });
  const toggleRule = useMutation({
    mutationFn: async (r: ForwardRule) => {
      if (!accessToken) throw new Error("Not authenticated");
      await updateForwardRule(accessToken, r.id, {
        name: r.name,
        mode: r.mode,
        condition_json: r.condition_json,
        forward_to: r.forward_to,
        enabled: !r.enabled,
      });
    },
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["forward-rules"] }),
  });
  const removeRule = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      await deleteForwardRule(accessToken, id);
    },
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["forward-rules"] }),
  });
  const runRulesMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !accountID) throw new Error("No account selected");
      return runForwardRules(accessToken, accountID);
    },
    onSuccess: () => {
      toast({ title: "Forward rules queued" });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
    },
  });

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Automation"
        title="Forwarding rules"
        description="New rules stay paused until you enable them — nothing auto-forwards by mistake. Destinations must be on your allowlist; each run is audited per account."
        actions={
          <Button
            size="sm"
            className="bg-foreground text-background hover:bg-foreground/90"
            onClick={() => runRulesMutation.mutate()}
            disabled={!accountID || runRulesMutation.isPending}
          >
            Run now
          </Button>
        }
      />
      <div className="surface-card p-4 space-y-3">
        <h3 className="font-display text-lg">Create rule</h3>
        <p className="text-xs text-muted-foreground">
          Rules are created <span className="font-medium text-foreground">paused</span>. Use the toggle on a rule to turn forwarding on after you have reviewed conditions and the allowlist destination.
        </p>
        <div className="grid gap-3 md:grid-cols-4">
          <Input value={newRuleName} onChange={(e) => setNewRuleName(e.target.value)} placeholder="Rule name" />
          <Select value={newRuleMode} onValueChange={(v) => setNewRuleMode(v as "logic" | "llm")}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="logic">logic</SelectItem>
              <SelectItem value="llm">llm</SelectItem>
            </SelectContent>
          </Select>
          <Input value={newForwardTo} onChange={(e) => setNewForwardTo(e.target.value)} placeholder="forward_to@email.com" />
          <Button onClick={() => createRule.mutate()} disabled={!accountID || createRule.isPending}>
            <Plus className="mr-1.5 h-3.5 w-3.5" /> New rule
          </Button>
        </div>
        <Input
          value={newConditionJSON}
          onChange={(e) => setNewConditionJSON(e.target.value)}
          placeholder='{"all":[{"field":"has_attachments","op":"equals","value":true}]} or {"prompt":"looks like invoice"}'
        />
      </div>

      <ul className="space-y-3">
        {visible.map((r) => (
          <li key={r.id} className="surface-card p-5">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <h3 className="font-display text-lg font-medium">{r.name}</h3>
                  {!r.enabled && (
                    <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                      paused
                    </span>
                  )}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                  <AccountBadge account={getAccount(r.account_id)} />
                  <span>·</span>
                  <span>
                    Forwards to{" "}
                    <span className="font-mono text-foreground/80">{r.forward_to}</span>
                  </span>
                </div>
                <div className="mt-3 rounded-md bg-secondary/60 px-3 py-2 font-mono text-xs text-foreground/80">
                  {JSON.stringify(r.condition_json)}
                </div>
                <p className="mt-3 text-xs text-muted-foreground">Mode: {r.mode}</p>
              </div>
              <div className="flex items-center gap-2">
                <Switch checked={r.enabled} onCheckedChange={() => toggleRule.mutate(r)} />
                <Button size="sm" variant="ghost" onClick={() => removeRule.mutate(r.id)}>Delete</Button>
              </div>
            </div>
          </li>
        ))}
      </ul>

      <section className="space-y-4">
        <div>
          <h2 className="font-display text-2xl">Allowlist</h2>
          <p className="text-sm text-muted-foreground">
            Forwarding destinations are restricted to these addresses.
          </p>
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <div className="min-w-[220px] flex-1 space-y-1">
            <label htmlFor="allowlist-email" className="text-xs text-muted-foreground">
              Email address
            </label>
            <Input
              id="allowlist-email"
              value={newAllow}
              onChange={(e) => setNewAllow(e.target.value)}
              placeholder="accounting@example.com"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  const trimmed = newAllow.trim().toLowerCase();
                  if (!trimmed || saveAllowlist.isPending) return;
                  const next = Array.from(new Set([...allowlist, trimmed]));
                  saveAllowlist.mutate(next);
                  setNewAllow("");
                }
              }}
            />
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={saveAllowlist.isPending || !newAllow.trim()}
            onClick={() => {
              const trimmed = newAllow.trim().toLowerCase();
              if (!trimmed) return;
              const next = Array.from(new Set([...allowlist, trimmed]));
              saveAllowlist.mutate(next);
              setNewAllow("");
            }}
          >
            <ShieldCheck className="mr-1.5 h-3.5 w-3.5" /> Add address
          </Button>
        </div>
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {allowlist.map((email) => (
            <li key={email} className="flex items-center justify-between px-4 py-3">
              <span className="font-mono text-sm">{email}</span>
              <button
                className="text-muted-foreground transition hover:text-destructive"
                onClick={() => saveAllowlist.mutate(allowlist.filter((v) => v !== email))}
              >
                <X className="h-4 w-4" />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
