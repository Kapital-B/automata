import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { Info, Loader2, Play, Plus, Workflow } from "lucide-react";
import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { toast } from "@/hooks/use-toast";
import {
  ApiError,
  deleteForwardRule,
  getApiHealth,
  getForwardAllowlist,
  listCategories,
  listForwardRules,
  runForwardRules,
  updateForwardRule,
  type ForwardRule,
} from "@/lib/auth";
import { RuleCard } from "@/components/rules/RuleCard";
import { RuleEditorDialog } from "@/components/rules/RuleEditorDialog";
import { StartRuleDialog } from "@/components/rules/StartRuleDialog";
import { AllowlistSection } from "@/components/rules/AllowlistSection";

interface Props {
  accountFilter: AccountFilter;
}

export default function RulesPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const { accounts, isLoading: accountsLoading } = useAccountsData();
  // "All accounts" means all of them: every mailbox's rules are shown and
  // run, not silently just the first one's.
  const scope = useMemo(
    () => (accountFilter === "all" ? accounts : accounts.filter((a) => a.id === accountFilter)),
    [accountFilter, accounts],
  );
  const [editor, setEditor] = useState<{ open: boolean; rule?: ForwardRule }>({ open: false });
  const [starting, setStarting] = useState<ForwardRule | null>(null);

  const allowlistQuery = useQuery({
    queryKey: ["forward-allowlist", accessToken],
    queryFn: () => getForwardAllowlist(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categoriesQuery = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const healthQuery = useQuery({ queryKey: ["api-health"], queryFn: getApiHealth, staleTime: 5 * 60_000 });
  const rulesQueries = useQueries({
    queries: scope.map((a) => ({
      queryKey: ["forward-rules", accessToken, a.id],
      queryFn: () => listForwardRules(accessToken!, a.id),
      enabled: Boolean(accessToken),
    })),
  });

  const allowlist = useMemo(() => allowlistQuery.data?.emails ?? [], [allowlistQuery.data?.emails]);
  const categories = useMemo(
    () => [...(categoriesQuery.data ?? [])].sort((a, b) => a.sort_order - b.sort_order),
    [categoriesQuery.data],
  );
  const groups = scope.map((account, i) => ({ account, rules: rulesQueries[i]?.data ?? [] }));
  const allRules = groups.flatMap((g) => g.rules);
  const rulesLoading = accountsLoading || rulesQueries.some((q) => q.isLoading);
  const rulesError = rulesQueries.find((q) => q.isError)?.error;
  const runnable = groups.filter((g) => g.rules.some((r) => r.enabled && !r.blocked_reason));
  const rulesByAddress = useMemo(() => {
    const out: Record<string, string[]> = {};
    for (const r of allRules) (out[r.forward_to] ??= []).push(r.name);
    return out;
  }, [allRules]);

  const invalidateRules = () => void queryClient.invalidateQueries({ queryKey: ["forward-rules"] });
  const fail = (title: string) => (e: unknown) =>
    toast({ title, description: e instanceof ApiError ? e.message : "Please try again.", variant: "destructive" });

  const switchOff = useMutation({
    mutationFn: async (r: ForwardRule) => {
      if (!accessToken) throw new Error("Not authenticated");
      await updateForwardRule(accessToken, r.id, {
        name: r.name,
        mode: r.mode,
        condition_json: r.condition_json,
        forward_to: r.forward_to,
        enabled: false,
      });
    },
    onSuccess: (_d, r) => {
      invalidateRules();
      toast({ title: `“${r.name}” switched off` });
    },
    onError: fail("Could not switch the rule off"),
  });
  const remove = useMutation({
    mutationFn: async (id: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      await deleteForwardRule(accessToken, id);
    },
    onSuccess: () => {
      invalidateRules();
      toast({ title: "Rule deleted" });
    },
    onError: fail("Could not delete the rule"),
  });
  const runNow = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return Promise.all(runnable.map((g) => runForwardRules(accessToken, g.account.id)));
    },
    onSuccess: (runs) => {
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: runs.length === 1 ? "Forwarding run started" : `${runs.length} forwarding runs started`,
        description: "Results appear on each rule, and on the Runs page.",
      });
    },
    onError: fail("Could not start forwarding"),
  });

  const busy = switchOff.isPending || remove.isPending;
  const defaultAccountID = accountFilter === "all" ? scope[0]?.id : accountFilter;

  return (
    <div className="space-y-10">
      <PageHeader
        eyebrow="Automation"
        title="Forwarding rules"
        description="Forward matching mail to an approved address. Every rule starts paused, and each decision it makes is recorded on the rule."
        actions={
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runNow.mutate()}
              disabled={runnable.length === 0 || runNow.isPending}
              title={runnable.length === 0 ? "No rule is switched on" : undefined}
            >
              {runNow.isPending ? (
                <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : (
                <Play aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
              )}
              Run now
            </Button>
            <Button
              size="sm"
              className="bg-foreground text-background hover:bg-foreground/90"
              onClick={() => setEditor({ open: true })}
              disabled={accounts.length === 0}
            >
              <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> New rule
            </Button>
          </>
        }
      />

      <p className="flex items-start gap-2 rounded-md border border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
        <Info aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
        <span>
          Rules that are on forward when rules run: when you press <strong className="text-foreground">Run now</strong>, or
          on a schedule with the <strong className="text-foreground">Run forwarding rules</strong> step in{" "}
          <Link to="/settings?tab=schedules" className="font-medium text-primary underline-offset-4 hover:underline">
            Settings
          </Link>
          . Switching a rule on does not send anything by itself.
        </span>
      </p>

      <section aria-labelledby="rules-heading" className="space-y-4">
        <h2 id="rules-heading" className="sr-only">
          Rules
        </h2>
        {rulesLoading ? (
          <div aria-label="Loading rules" className="surface-card space-y-2 p-5">
            <Skeleton className="h-5 w-48" />
            <Skeleton className="h-4 w-72" />
          </div>
        ) : rulesError ? (
          <div role="alert" className="surface-card p-5 text-sm text-destructive">
            Could not load rules: {rulesError instanceof Error ? rulesError.message : "unknown error"}
          </div>
        ) : scope.length === 0 ? (
          <div className="surface-card p-5 text-sm text-muted-foreground">
            Connect a mailbox on the{" "}
            <Link to="/accounts" className="font-medium text-primary underline-offset-4 hover:underline">
              Accounts
            </Link>{" "}
            page first.
          </div>
        ) : allRules.length === 0 ? (
          <div className="surface-card flex flex-col items-start gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-center gap-3">
              <span
                aria-hidden="true"
                className="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-secondary"
              >
                <Workflow className="h-5 w-5" />
              </span>
              <p className="text-sm text-muted-foreground">No rules yet.</p>
            </div>
            <Button size="sm" onClick={() => setEditor({ open: true })}>
              <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> New rule
            </Button>
          </div>
        ) : (
          groups
            .filter((g) => g.rules.length > 0)
            .map((g) => (
              <div key={g.account.id} className="space-y-3">
                {scope.length > 1 && (
                  <h3 className="text-sm">
                    <AccountBadge account={g.account} showEmail size="md" />
                  </h3>
                )}
                <ul className="space-y-3">
                  {g.rules.map((r) => (
                    <RuleCard
                      key={r.id}
                      rule={r}
                      account={g.account}
                      categories={categories}
                      showAccount={false}
                      busy={busy}
                      onSwitchOn={() => setStarting(r)}
                      onSwitchOff={() => switchOff.mutate(r)}
                      onEdit={() => setEditor({ open: true, rule: r })}
                      onDelete={() => remove.mutate(r.id)}
                    />
                  ))}
                </ul>
              </div>
            ))
        )}
      </section>

      <AllowlistSection allowlist={allowlist} rulesByAddress={rulesByAddress} />

      <RuleEditorDialog
        open={editor.open}
        onOpenChange={(open) => setEditor((e) => ({ ...e, open }))}
        rule={editor.rule}
        accounts={editor.rule ? accounts : scope}
        defaultAccountID={defaultAccountID}
        allowlist={allowlist}
        categories={categories}
        aiAvailable={healthQuery.data?.llm !== false}
      />
      <StartRuleDialog rule={starting} onOpenChange={(open) => !open && setStarting(null)} />
    </div>
  );
}
