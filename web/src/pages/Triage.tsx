import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
  assignProjectsBatch,
  listProjects,
  listUnassigned,
  rescanUnassigned,
  type BatchAssignItem,
  type UnassignedItem,
} from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";
import {
  explainReason,
  headlineFor,
  itemID,
  itemKey,
  type TriageProject as Project,
} from "@/lib/triage";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, FolderKanban, Inbox, Loader2, RefreshCw, Sparkles } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";

/** True when the event came from somewhere that owns its own keystrokes. */
function isTextEntryTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  if (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)) return true;
  return target.getAttribute("role") === "combobox";
}

export default function TriagePage() {
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

  const projects = useMemo<Project[]>(() => projectsQuery.data ?? [], [projectsQuery.data]);
  const items = useMemo(() => unassignedQuery.data ?? [], [unassignedQuery.data]);

  const provisional = useMemo(() => items.filter((i) => i.status === "provisional"), [items]);
  const plain = useMemo(() => items.filter((i) => i.status === "unassigned"), [items]);

  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkProject, setBulkProject] = useState("");
  // Per-row project override, keyed by row. Defaults to the suggestion.
  const [overrides, setOverrides] = useState<Record<string, string>>({});
  const [focusIndex, setFocusIndex] = useState(0);
  const [recentProjectIDs, setRecentProjectIDs] = useState<string[]>([]);
  const lastBatch = useRef<BatchAssignItem[] | null>(null);

  // Rows in the order they are rendered, so keyboard movement matches the page.
  const visibleItems = useMemo(() => {
    const byProject = new Map<string, UnassignedItem[]>();
    provisional.forEach((item) => {
      const key = item.project_id ?? "";
      byProject.set(key, [...(byProject.get(key) ?? []), item]);
    });
    return [...[...byProject.values()].flat(), ...plain];
  }, [plain, provisional]);

  // The numeric shortcuts address most-recently-used projects first.
  const shortcutProjects = useMemo(() => {
    const ordered = recentProjectIDs
      .map((id) => projects.find((p) => p.id === id))
      .filter((p): p is Project => Boolean(p));
    const rest = projects.filter((p) => !recentProjectIDs.includes(p.id));
    return [...ordered, ...rest].slice(0, 9);
  }, [projects, recentProjectIDs]);

  // Drop selections whose rows have left the queue.
  useEffect(() => {
    const live = new Set(items.map(itemKey));
    setSelected((prev) => {
      const next = new Set([...prev].filter((k) => live.has(k)));
      return next.size === prev.size ? prev : next;
    });
  }, [items]);

  const projectFor = useCallback(
    (item: UnassignedItem) => overrides[itemKey(item)] ?? item.project_id ?? "",
    [overrides],
  );

  const invalidate = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["unassigned"] }),
      queryClient.invalidateQueries({ queryKey: ["unassigned-summary"] }),
      queryClient.invalidateQueries({ queryKey: ["project-timeline"] }),
    ]);
  }, [queryClient]);

  const batchMutation = useMutation({
    mutationFn: async (batch: BatchAssignItem[]) => {
      if (!accessToken) throw new Error("Not authenticated");
      return assignProjectsBatch(accessToken, batch);
    },
    onSuccess: async (res, batch) => {
      const assignedIDs = new Set(res.results.filter((r) => r.ok).map((r) => r.id));
      const landed = batch.filter((b) => assignedIDs.has(b.id) && b.project_id !== null);
      lastBatch.current = landed.length > 0 ? landed : null;
      if (landed.length > 0) {
        const used = [...new Set(landed.map((b) => b.project_id as string))];
        setRecentProjectIDs((prev) => [...used, ...prev.filter((id) => !used.includes(id))]);
      }
      if (res.failed === 0) {
        toast({ title: res.assigned === 1 ? "Assigned" : `${res.assigned} assigned` });
      } else {
        const firstError = res.results.find((r) => !r.ok)?.error ?? "unknown";
        toast({
          title: `${res.assigned} assigned, ${res.failed} failed`,
          description: `First failure: ${firstError.replace(/_/g, " ")}.`,
          variant: "destructive",
        });
      }
      setSelected(new Set());
      await invalidate();
    },
    onError: (err) => {
      toast({
        title: "Assign failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const rescanMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return rescanUnassigned(accessToken);
    },
    onSuccess: async () => {
      toast({
        title: "Rescan started",
        description: "Suggestions refresh as the run completes.",
      });
      await invalidate();
    },
    onError: (err) => {
      toast({
        title: "Rescan failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const buildItem = useCallback(
    (item: UnassignedItem, projectID: string, scope: "thread" | "message"): BatchAssignItem => ({
      kind: item.kind,
      id: itemID(item),
      project_id: projectID,
      ...(item.kind === "message" ? { scope } : {}),
    }),
    [],
  );

  const assignOne = useCallback(
    (item: UnassignedItem, scope: "thread" | "message", projectID?: string) => {
      const target = projectID ?? projectFor(item);
      if (!target) return;
      batchMutation.mutate([buildItem(item, target, scope)]);
    },
    [batchMutation, buildItem, projectFor],
  );

  const assignSelected = useCallback(
    (scope: "thread" | "message") => {
      if (!bulkProject) return;
      const byKey = new Map(items.map((i) => [itemKey(i), i]));
      const batch: BatchAssignItem[] = [];
      selected.forEach((key) => {
        const item = byKey.get(key);
        if (item) batch.push(buildItem(item, bulkProject, scope));
      });
      if (batch.length > 0) batchMutation.mutate(batch);
    },
    [batchMutation, buildItem, bulkProject, items, selected],
  );

  const confirmAll = useCallback(
    (group: UnassignedItem[]) => {
      const batch = group
        .map((item) => {
          const target = projectFor(item);
          return target ? buildItem(item, target, "thread") : null;
        })
        .filter((x): x is BatchAssignItem => x !== null);
      if (batch.length > 0) batchMutation.mutate(batch);
    },
    [batchMutation, buildItem, projectFor],
  );

  // Undo clears the assignments the last batch made. It is the inverse of
  // "filed into a project", not a restore of any prior suggestion.
  const undoLastBatch = useCallback(() => {
    const batch = lastBatch.current;
    if (!batch || batch.length === 0) return;
    lastBatch.current = null;
    batchMutation.mutate(batch.map((b) => ({ ...b, project_id: null })));
  }, [batchMutation]);

  const moveFocus = useCallback(
    (delta: number, extend: boolean) => {
      setFocusIndex((prev) => {
        const next = Math.max(0, Math.min(visibleItems.length - 1, prev + delta));
        if (extend && visibleItems[next]) {
          const key = itemKey(visibleItems[next]);
          setSelected((s) => new Set(s).add(key));
        }
        return next;
      });
    },
    [visibleItems],
  );

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.metaKey || event.ctrlKey || event.altKey) return;
      // Never steal keys from the project picker or any text entry. The target
      // is not always an element (it can be the window itself), so feature-test
      // before reaching for element APIs.
      if (isTextEntryTarget(event.target)) return;
      const item = visibleItems[focusIndex];
      switch (event.key) {
        case "j":
        case "ArrowDown":
          event.preventDefault();
          moveFocus(1, event.shiftKey);
          return;
        case "k":
        case "ArrowUp":
          event.preventDefault();
          moveFocus(-1, event.shiftKey);
          return;
        case " ":
          if (!item) return;
          event.preventDefault();
          setSelected((prev) => {
            const next = new Set(prev);
            const key = itemKey(item);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
          });
          return;
        case "Enter":
          if (!item) return;
          event.preventDefault();
          assignOne(item, "thread");
          return;
        case "u":
          event.preventDefault();
          undoLastBatch();
          return;
        default:
          break;
      }
      if (/^[1-9]$/.test(event.key) && item) {
        const project = shortcutProjects[Number(event.key) - 1];
        if (project) {
          event.preventDefault();
          assignOne(item, "thread", project.id);
        }
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [assignOne, focusIndex, moveFocus, shortcutProjects, undoLastBatch, visibleItems]);

  // Keep the cursor inside the list as rows leave the queue.
  useEffect(() => {
    setFocusIndex((prev) => Math.max(0, Math.min(prev, visibleItems.length - 1)));
  }, [visibleItems.length]);

  const focusedKey = visibleItems[focusIndex] ? itemKey(visibleItems[focusIndex]) : "";

  const accountFor = (accountID?: string) =>
    accountID ? accounts.find((x) => x.id === accountID) : undefined;

  const busy = batchMutation.isPending;
  const selectedCount = selected.size;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Queue"
        title="Triage"
        description="File mail and pastes into a project. Confirm suggestions or assign items that still need a home."
      />

      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          onClick={() => rescanMutation.mutate()}
          disabled={rescanMutation.isPending}
        >
          <RefreshCw
            className={`mr-1.5 h-3.5 w-3.5 ${rescanMutation.isPending ? "animate-spin" : ""}`}
          />
          Rescan suggestions
        </Button>
        <p className="text-xs text-muted-foreground">
          <kbd>j</kbd>/<kbd>k</kbd> move · <kbd>space</kbd> select · <kbd>enter</kbd> confirm ·{" "}
          <kbd>1</kbd>–<kbd>9</kbd> file to a recent project · <kbd>u</kbd> undo
        </p>
      </div>

      {unassignedQuery.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading…
        </div>
      ) : unassignedQuery.isError ? (
        <p className="text-sm text-destructive">
          {unassignedQuery.error instanceof ApiError
            ? unassignedQuery.error.message
            : "Could not load triage items."}
        </p>
      ) : provisional.length === 0 && plain.length === 0 ? (
        <div className="space-y-4 border-y border-border/70 py-8">
          <div className="flex items-start gap-3 text-sm text-muted-foreground">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" />
            <div className="space-y-1">
              <p className="font-medium text-foreground">Triage is clear</p>
              <p>Nothing waiting to be filed. Sync mail or paste correspondence from a project.</p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline" size="sm">
              <Link to="/projects">
                <FolderKanban className="mr-1.5 h-3.5 w-3.5" />
                Projects
              </Link>
            </Button>
            <Button asChild variant="outline" size="sm">
              <Link to="/">Back to Home</Link>
            </Button>
          </div>
        </div>
      ) : (
        <>
          <SuggestionSection
            focusedKey={focusedKey}
            items={provisional}
            projects={projects}
            accountFor={accountFor}
            busy={busy}
            selected={selected}
            setSelected={setSelected}
            projectFor={projectFor}
            setOverride={(key, value) => setOverrides((p) => ({ ...p, [key]: value }))}
            onAssign={assignOne}
            onConfirmAll={confirmAll}
          />
          <Section
            title="Needs filing"
            empty="Nothing left to file."
            focusedKey={focusedKey}
            items={plain}
            projects={projects}
            accountFor={accountFor}
            busy={busy}
            selected={selected}
            setSelected={setSelected}
            projectFor={projectFor}
            setOverride={(key, value) => setOverrides((p) => ({ ...p, [key]: value }))}
            onAssign={assignOne}
          />
        </>
      )}

      {selectedCount > 0 ? (
        <div
          className="sticky bottom-4 z-10 flex flex-wrap items-center gap-2 rounded-md border border-border bg-background/95 p-3 shadow-lg backdrop-blur"
          role="region"
          aria-label="Bulk assignment"
        >
          <span aria-live="polite" className="text-sm font-medium">
            {selectedCount} selected
          </span>
          <Select value={bulkProject} onValueChange={setBulkProject}>
            <SelectTrigger className="w-[220px]" aria-label="Project for selected items">
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
          <Button size="sm" disabled={!bulkProject || busy} onClick={() => assignSelected("thread")}>
            Assign thread
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={!bulkProject || busy}
            onClick={() => assignSelected("message")}
          >
            This message only
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
            Clear selection
          </Button>
        </div>
      ) : null}
    </div>
  );
}

type SectionProps = {
  items: UnassignedItem[];
  focusedKey: string;
  projects: Project[];
  accountFor: (id?: string) => UiAccount | undefined;
  busy: boolean;
  selected: Set<string>;
  setSelected: (fn: (prev: Set<string>) => Set<string>) => void;
  projectFor: (item: UnassignedItem) => string;
  setOverride: (key: string, value: string) => void;
  onAssign: (item: UnassignedItem, scope: "thread" | "message", projectID?: string) => void;
};

/**
 * Suggestions grouped by the project the scorer picked, so the interaction is
 * "these 14 look like DC01, accept them" rather than a flat list.
 */
function SuggestionSection({
  onConfirmAll,
  ...props
}: SectionProps & { onConfirmAll: (group: UnassignedItem[]) => void }) {
  const groups = useMemo(() => {
    const byProject = new Map<string, UnassignedItem[]>();
    props.items.forEach((item) => {
      const key = item.project_id ?? "";
      byProject.set(key, [...(byProject.get(key) ?? []), item]);
    });
    return [...byProject.entries()];
  }, [props.items]);

  return (
    <section className="space-y-3">
      <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
        Needs confirmation
        <span className="ml-2 font-normal normal-case tracking-normal">({props.items.length})</span>
      </h2>
      {props.items.length === 0 ? (
        <div className="flex items-start gap-2 py-4 text-sm text-muted-foreground">
          <Inbox className="mt-0.5 h-4 w-4 opacity-60" />
          <p>No suggestions waiting.</p>
        </div>
      ) : (
        groups.map(([projectID, group]) => {
          const project = props.projects.find((p) => p.id === projectID);
          return (
            <div key={projectID || "none"} className="space-y-2">
              <div className="flex flex-wrap items-center gap-2 border-t border-border/70 pt-3">
                <Sparkles className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-sm font-medium">
                  {project ? `${project.code} · ${project.name}` : "No project suggested"}
                </span>
                <span className="text-xs text-muted-foreground">({group.length})</span>
                {project ? (
                  <Button
                    size="sm"
                    disabled={props.busy}
                    onClick={() => onConfirmAll(group)}
                  >
                    Confirm all {group.length}
                  </Button>
                ) : null}
              </div>
              <RowList {...props} items={group} />
            </div>
          );
        })
      )}
    </section>
  );
}

function Section({
  title,
  empty,
  ...props
}: SectionProps & { title: string; empty: string }) {
  return (
    <section className="space-y-3">
      <h2 className="text-sm font-medium uppercase tracking-wider text-muted-foreground">
        {title}
        <span className="ml-2 font-normal normal-case tracking-normal">({props.items.length})</span>
      </h2>
      {props.items.length === 0 ? (
        <div className="flex items-start gap-2 py-4 text-sm text-muted-foreground">
          <Inbox className="mt-0.5 h-4 w-4 opacity-60" />
          <p>{empty}</p>
        </div>
      ) : (
        <RowList {...props} />
      )}
    </section>
  );
}

function RowList(props: SectionProps) {
  const { items, selected, setSelected } = props;
  const lastClicked = useRef<number | null>(null);

  const toggle = useCallback(
    (index: number, shiftKey: boolean) => {
      const key = itemKey(items[index]);
      setSelected((prev) => {
        const next = new Set(prev);
        // Shift-click extends from the previous click, which is how operators
        // expect to grab a run of related mail.
        if (shiftKey && lastClicked.current !== null) {
          const [from, to] = [lastClicked.current, index].sort((a, b) => a - b);
          const selecting = !prev.has(key);
          for (let i = from; i <= to; i += 1) {
            const k = itemKey(items[i]);
            if (selecting) next.add(k);
            else next.delete(k);
          }
        } else if (next.has(key)) {
          next.delete(key);
        } else {
          next.add(key);
        }
        return next;
      });
      lastClicked.current = index;
    },
    [items, setSelected],
  );

  const allSelected = items.length > 0 && items.every((i) => selected.has(itemKey(i)));

  return (
    <>
      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        <Checkbox
          checked={allSelected}
          onCheckedChange={(checked) =>
            setSelected((prev) => {
              const next = new Set(prev);
              items.forEach((i) => (checked ? next.add(itemKey(i)) : next.delete(itemKey(i))));
              return next;
            })
          }
          aria-label={allSelected ? "Deselect all in this group" : "Select all in this group"}
        />
        Select all
      </label>
      <ul className="divide-y divide-border/70 border-y border-border/70">
        {items.map((item, index) => (
          <UnassignedRow
            key={itemKey(item)}
            item={item}
            index={index}
            projects={props.projects}
            account={props.accountFor(item.account_id)}
            busy={props.busy}
            selected={selected.has(itemKey(item))}
            focused={props.focusedKey === itemKey(item)}
            onToggle={toggle}
            projectID={props.projectFor(item)}
            setProjectID={(value) => props.setOverride(itemKey(item), value)}
            onAssign={props.onAssign}
          />
        ))}
      </ul>
    </>
  );
}

function UnassignedRow({
  item,
  index,
  projects,
  account,
  busy,
  selected,
  focused,
  onToggle,
  projectID,
  setProjectID,
  onAssign,
}: {
  item: UnassignedItem;
  index: number;
  projects: Project[];
  account: UiAccount | undefined;
  busy: boolean;
  selected: boolean;
  focused: boolean;
  onToggle: (index: number, shiftKey: boolean) => void;
  projectID: string;
  setProjectID: (value: string) => void;
  onAssign: (item: UnassignedItem, scope: "thread" | "message", projectID?: string) => void;
}) {
  const isManual = item.kind === "manual";
  const headline = headlineFor(item);
  const from = item.from_json?.name || item.from_json?.address || "";
  const when = item.occurred_at || item.received_at;
  const threadCount = item.thread_count ?? 1;
  const suggested = projects.find((p) => p.id === item.project_id);
  const explanation = explainReason(item.reason, projects);

  return (
    <li
      className={`space-y-3 py-4 ${focused ? "-mx-2 rounded-sm px-2 ring-2 ring-ring" : ""}`}
      aria-selected={selected}
      aria-current={focused ? "true" : undefined}
    >
      <div className="flex gap-3">
        <Checkbox
          className="mt-1"
          checked={selected}
          onClick={(event) => onToggle(index, (event as React.MouseEvent).shiftKey)}
          aria-label={`Select ${headline}`}
        />
        <div className="min-w-0 flex-1 space-y-1">
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
            {threadCount > 1 ? (
              <span className="rounded bg-muted px-1.5 py-0.5 text-xs text-muted-foreground">
                {threadCount} messages
              </span>
            ) : null}
          </div>
          {from ? <p className="text-xs text-muted-foreground">{from}</p> : null}
          {when ? (
            <p className="text-xs text-muted-foreground">{new Date(when).toLocaleString()}</p>
          ) : null}
          {explanation ? (
            <p className="text-xs text-muted-foreground">
              {item.source ? (
                <span className="mr-1.5 rounded bg-muted px-1.5 py-0.5 uppercase tracking-wider">
                  {item.source}
                </span>
              ) : null}
              {explanation}
              {typeof item.confidence === "number"
                ? ` · ${Math.round(item.confidence * 100)}% confident`
                : ""}
            </p>
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 pl-7">
        {suggested && projectID === suggested.id ? (
          <Button
            size="sm"
            disabled={busy || (!isManual && !item.message_id)}
            onClick={() => onAssign(item, "thread")}
          >
            Confirm → {suggested.code} · {suggested.name}
          </Button>
        ) : null}
        <Select value={projectID} onValueChange={setProjectID}>
          <SelectTrigger className="w-[200px]" aria-label={`Project for ${headline}`}>
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
            variant={suggested ? "outline" : "default"}
            disabled={!projectID || busy || !item.manual_item_id}
            onClick={() => onAssign(item, "thread")}
          >
            Assign
          </Button>
        ) : (
          <>
            <Button
              size="sm"
              variant={suggested && projectID === suggested.id ? "outline" : "default"}
              disabled={!projectID || busy || !item.conversation_id || !item.message_id}
              onClick={() => onAssign(item, "thread")}
            >
              Assign thread
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={!projectID || busy || !item.message_id}
              onClick={() => onAssign(item, "message")}
            >
              This message only
            </Button>
          </>
        )}
      </div>
    </li>
  );
}
