import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, CalendarClock, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmAction } from "@/components/ConnectionCard";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { toast } from "@/hooks/use-toast";
import { relativeTime } from "@/lib/accounts";
import { ApiError, getScheduleSettings, updateScheduleSettings, type ScheduleChain } from "@/lib/auth";
import { defaultAvailableJobs, intervalLabel, intervalPresets, jobInfo, jobLabel, untilTime } from "@/lib/scheduleJobs";
import { SaveBar } from "@/components/settings/SaveBar";

const selectClass =
  "h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

/** The part of a chain a person edits; run times come from the server. */
function editable(c: ScheduleChain) {
  return {
    id: c.id,
    name: c.name.trim(),
    account_id: c.account_id ?? null,
    jobs: c.jobs,
    interval_minutes: c.interval_minutes,
    enabled: c.enabled,
  };
}

export function ScheduleSettings() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const { accounts } = useAccountsData();
  const query = useQuery({
    queryKey: ["schedule-settings", accessToken],
    queryFn: () => getScheduleSettings(accessToken!),
    enabled: Boolean(accessToken),
  });
  const available = useMemo(() => query.data?.available_jobs ?? defaultAvailableJobs, [query.data?.available_jobs]);
  const saved = useMemo(() => query.data?.chains ?? [], [query.data?.chains]);

  // Steps a schedule cannot run (a typo, or the "auto-draft" this page once
  // suggested) are dropped from the draft and listed, so saving cleans them up.
  const invalidSteps = useMemo(() => {
    const out: Record<string, string[]> = {};
    for (const c of saved) {
      const bad = c.jobs.filter((j) => !available.includes(j));
      if (bad.length) out[c.id] = bad;
    }
    return out;
  }, [saved, available]);
  const clean = useMemo(
    () => saved.map((c) => ({ ...c, jobs: available.filter((j) => c.jobs.includes(j)) })),
    [saved, available],
  );
  const [chains, setChains] = useState<ScheduleChain[]>([]);
  useEffect(() => setChains(clean), [clean]);

  const dirty = JSON.stringify(chains.map(editable)) !== JSON.stringify(saved.map(editable));
  const emptyChain = chains.find((c) => c.jobs.length === 0);

  const save = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return updateScheduleSettings(
        accessToken,
        chains.map((c) => ({ ...c, name: c.name.trim() || "Schedule" })),
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["schedule-settings"] });
      toast({ title: "Schedules saved" });
    },
    onError: (e) =>
      toast({
        title: "Could not save schedules",
        description: e instanceof ApiError ? e.message : "Please try again.",
        variant: "destructive",
      }),
  });

  const update = (id: string, patch: Partial<ScheduleChain>) =>
    setChains((cs) => cs.map((c) => (c.id === id ? { ...c, ...patch } : c)));

  const addChain = () =>
    setChains((cs) => [
      ...cs,
      {
        id: crypto.randomUUID(),
        name: cs.length === 0 ? "Mailbox refresh" : `Schedule ${cs.length + 1}`,
        jobs: available.filter((j) => ["sync", "resolve_contacts", "categorize", "summarize"].includes(j)),
        interval_minutes: 60,
        enabled: true,
      },
    ]);

  return (
    <section aria-labelledby="schedules-heading" className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="space-y-1">
          <h2 id="schedules-heading" className="font-display text-2xl font-medium">
            Schedules
          </h2>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Run jobs automatically on a timer. Each schedule runs its steps in order, for one mailbox or all of them.
          </p>
        </div>
        <Button size="sm" onClick={addChain}>
          <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add schedule
        </Button>
      </div>

      {query.isLoading ? (
        <div aria-label="Loading schedules" className="surface-card space-y-3 p-5">
          <Skeleton className="h-5 w-48" />
          <Skeleton className="h-4 w-72" />
        </div>
      ) : query.isError ? (
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          Could not load schedules.
        </div>
      ) : chains.length === 0 ? (
        <div className="surface-card flex flex-col items-start gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-3">
            <span aria-hidden="true" className="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-secondary">
              <CalendarClock className="h-5 w-5" />
            </span>
            <p className="text-sm text-muted-foreground">
              No schedules. Mail only syncs when you ask it to until you add one.
            </p>
          </div>
          <Button size="sm" onClick={addChain}>
            <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add schedule
          </Button>
        </div>
      ) : (
        <ul className="space-y-3">
          {chains.map((chain) => {
            const persisted = saved.find((s) => s.id === chain.id);
            const bad = invalidSteps[chain.id] ?? [];
            const presetValues = intervalPresets.map((p) => p.minutes);
            return (
              <li key={chain.id} className="surface-card overflow-hidden">
                <div className="flex flex-wrap items-start justify-between gap-4 p-5">
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <Label htmlFor={`chain-name-${chain.id}`} className="sr-only">
                      Schedule name
                    </Label>
                    <Input
                      id={`chain-name-${chain.id}`}
                      value={chain.name}
                      onChange={(e) => update(chain.id, { name: e.target.value })}
                      className="h-9 max-w-sm font-display text-lg font-medium"
                    />
                    <p className="text-sm text-muted-foreground">
                      {!persisted
                        ? "Not saved yet"
                        : !chain.enabled
                          ? "Off"
                          : `${intervalLabel(persisted.interval_minutes)} · next run ${untilTime(persisted.next_run_at)}${
                              persisted.last_run_at ? ` · last ran ${relativeTime(persisted.last_run_at)}` : ""
                            }`}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Label htmlFor={`chain-on-${chain.id}`} className="text-sm">
                      {chain.enabled ? "On" : "Off"}
                    </Label>
                    <Switch
                      id={`chain-on-${chain.id}`}
                      checked={chain.enabled}
                      onCheckedChange={(on) => update(chain.id, { enabled: on })}
                    />
                  </div>
                </div>

                <div className="grid gap-4 border-t border-border/70 px-5 py-4 sm:grid-cols-2">
                  <div className="space-y-1.5">
                    <Label htmlFor={`chain-account-${chain.id}`}>Mailbox</Label>
                    <select
                      id={`chain-account-${chain.id}`}
                      className={selectClass}
                      value={chain.account_id ?? ""}
                      onChange={(e) => update(chain.id, { account_id: e.target.value || undefined })}
                    >
                      <option value="">All connected mailboxes</option>
                      {accounts.map((a) => (
                        <option key={a.id} value={a.id}>
                          {a.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor={`chain-interval-${chain.id}`}>How often</Label>
                    <select
                      id={`chain-interval-${chain.id}`}
                      className={selectClass}
                      value={chain.interval_minutes}
                      onChange={(e) => update(chain.id, { interval_minutes: Number(e.target.value) })}
                    >
                      {intervalPresets.map((p) => (
                        <option key={p.minutes} value={p.minutes}>
                          {p.label}
                        </option>
                      ))}
                      {!presetValues.includes(chain.interval_minutes) && (
                        <option value={chain.interval_minutes}>{intervalLabel(chain.interval_minutes)}</option>
                      )}
                    </select>
                  </div>
                </div>

                <fieldset className="border-t border-border/70 px-5 py-4">
                  <legend className="sr-only">Steps</legend>
                  <p className="mb-3 text-sm font-medium">Steps, run in this order</p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {available.map((job) => {
                      const id = `chain-${chain.id}-${job}`;
                      return (
                        <div key={job} className="flex items-start gap-3 rounded-md border border-border px-3 py-2.5">
                          <Checkbox
                            id={id}
                            className="mt-0.5"
                            checked={chain.jobs.includes(job)}
                            onCheckedChange={(v) =>
                              update(chain.id, {
                                // Kept in pipeline order whatever order they are ticked in.
                                jobs: available.filter((j) => (j === job ? v === true : chain.jobs.includes(j))),
                              })
                            }
                          />
                          <Label htmlFor={id} className="cursor-pointer space-y-0.5 font-normal">
                            <span className="block text-sm font-medium">{jobLabel(job)}</span>
                            <span className="block text-xs text-muted-foreground">{jobInfo[job]?.description}</span>
                          </Label>
                        </div>
                      );
                    })}
                  </div>
                  {chain.jobs.length === 0 && (
                    <p role="alert" className="mt-3 flex items-center gap-2 text-sm text-destructive">
                      <AlertTriangle aria-hidden="true" className="h-4 w-4" /> Pick at least one step.
                    </p>
                  )}
                  {bad.length > 0 && (
                    <p className="mt-3 flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-sm">
                      <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
                      <span>
                        This schedule included {bad.map((b) => `“${b}”`).join(", ")}, which a schedule cannot run, so it
                        never ran. It is removed when you save.
                      </span>
                    </p>
                  )}
                </fieldset>

                <div className="flex justify-end border-t border-border/70 bg-muted/30 px-5 py-3">
                  <ConfirmAction
                    title={`Remove “${chain.name || "this schedule"}”?`}
                    description="It stops running once you save. Jobs already running finish."
                    confirmLabel="Remove schedule"
                    destructive
                    onConfirm={() => setChains((cs) => cs.filter((c) => c.id !== chain.id))}
                    trigger={(open) => (
                      <Button size="sm" variant="ghost" className="text-muted-foreground hover:text-destructive" onClick={open}>
                        <Trash2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Remove schedule
                      </Button>
                    )}
                  />
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <SaveBar
        dirty={dirty}
        saving={save.isPending}
        canSave={!emptyChain}
        label={emptyChain ? `“${emptyChain.name || "A schedule"}” needs at least one step` : undefined}
        onSave={() => save.mutate()}
        onDiscard={() => setChains(clean)}
      />
    </section>
  );
}
