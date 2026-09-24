import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import { cn } from "@/lib/utils";
import {
  ApiError,
  getSummarySettings,
  listCategories,
  updateSummarySettings,
  type SummarySettings as SummarySettingsPayload,
} from "@/lib/auth";
import { SaveBar } from "@/components/settings/SaveBar";

type Mode = "all" | "only";
type Draft = { mode: Mode; selected: string[]; chunkSize: number };

const batchOptions = [
  { value: 6, label: "Small — 6 messages", hint: "Most detail, most model calls" },
  { value: 12, label: "Balanced — 12 messages", hint: "Recommended" },
  { value: 20, label: "Large — 20 messages", hint: "Fewer calls, less detail per message" },
  { value: 30, label: "Largest — 30 messages", hint: "Cheapest; long threads get compressed" },
];

/**
 * The stored form has both an include and an exclude list, which a person
 * could set to contradict each other. The page offers one decision instead:
 * summarise everything except some categories, or only some.
 */
function toDraft(s: SummarySettingsPayload): Draft {
  const include = s.include_category_slugs ?? [];
  const exclude = s.exclude_category_slugs ?? [];
  if (include.length > 0) {
    return { mode: "only", selected: include.filter((x) => !exclude.includes(x)), chunkSize: s.chunk_size || 12 };
  }
  return { mode: "all", selected: exclude, chunkSize: s.chunk_size || 12 };
}

function toPayload(d: Draft): SummarySettingsPayload {
  const selected = [...d.selected].sort();
  return d.mode === "only"
    ? { include_category_slugs: selected, exclude_category_slugs: [], chunk_size: d.chunkSize }
    : { include_category_slugs: [], exclude_category_slugs: selected, chunk_size: d.chunkSize };
}

export function SummarySettings() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const settingsQuery = useQuery({
    queryKey: ["summary-settings", accessToken],
    queryFn: () => getSummarySettings(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categoriesQuery = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categories = useMemo(
    () => [...(categoriesQuery.data ?? [])].sort((a, b) => a.sort_order - b.sort_order),
    [categoriesQuery.data],
  );
  const saved = useMemo(() => (settingsQuery.data ? toDraft(settingsQuery.data) : null), [settingsQuery.data]);
  const [draft, setDraft] = useState<Draft | null>(null);
  useEffect(() => {
    if (saved) setDraft(saved);
  }, [saved]);

  const dirty = Boolean(saved && draft && JSON.stringify(toPayload(saved)) !== JSON.stringify(toPayload(draft)));
  const nothingSummarised = draft?.mode === "only" && draft.selected.length === 0;

  const save = useMutation({
    mutationFn: async () => {
      if (!accessToken || !draft) throw new Error("Not authenticated");
      return updateSummarySettings(accessToken, toPayload(draft));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary-settings"] });
      toast({ title: "Summary settings saved" });
    },
    onError: (e) =>
      toast({
        title: "Could not save summary settings",
        description: e instanceof ApiError ? e.message : "Please try again.",
        variant: "destructive",
      }),
  });

  if (settingsQuery.isLoading || categoriesQuery.isLoading || !draft) {
    return settingsQuery.isError ? (
      <div role="alert" className="surface-card p-5 text-sm text-destructive">
        Could not load summary settings.
      </div>
    ) : (
      <div aria-label="Loading summary settings" className="surface-card space-y-3 p-5">
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }

  const toggle = (slug: string, on: boolean) =>
    setDraft((d) => d && { ...d, selected: on ? [...d.selected, slug] : d.selected.filter((s) => s !== slug) });

  return (
    <div className="space-y-6">
      <section aria-labelledby="summary-scope-heading" className="surface-card space-y-5 p-5">
        <div className="space-y-1">
          <h2 id="summary-scope-heading" className="font-display text-xl font-medium">
            What gets summarised
          </h2>
          <p className="text-sm text-muted-foreground">Choose which categories of mail go into the inbox summary.</p>
        </div>
        <div role="radiogroup" aria-label="What gets summarised" className="grid gap-3 sm:grid-cols-2">
          {(
            [
              ["all", "All categories, except…", "New categories are included automatically."],
              ["only", "Only the categories I pick", "New categories stay out until you add them."],
            ] as const
          ).map(([value, title, blurb]) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={draft.mode === value}
              onClick={() => setDraft((d) => d && (d.mode === value ? d : { ...d, mode: value, selected: [] }))}
              className={cn(
                "rounded-lg border p-4 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                draft.mode === value ? "border-foreground bg-secondary" : "border-border hover:border-foreground/40",
              )}
            >
              <p className="text-sm font-medium">{title}</p>
              <p className="mt-1 text-xs text-muted-foreground">{blurb}</p>
            </button>
          ))}
        </div>

        <fieldset className="space-y-2">
          <legend className="mb-2 text-sm font-medium">
            {draft.mode === "all" ? "Leave these out" : "Summarise these"}
          </legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {categories.map((c) => {
              const id = `summary-cat-${c.slug}`;
              return (
                <div key={c.id} className="flex items-start gap-3 rounded-md border border-border px-3 py-2.5">
                  <Checkbox
                    id={id}
                    checked={draft.selected.includes(c.slug)}
                    onCheckedChange={(v) => toggle(c.slug, v === true)}
                    className="mt-0.5"
                  />
                  <Label htmlFor={id} className="cursor-pointer space-y-0.5 font-normal">
                    <span className="block text-sm font-medium">{c.display_name}</span>
                    {c.definition && <span className="block text-xs text-muted-foreground line-clamp-1">{c.definition}</span>}
                  </Label>
                </div>
              );
            })}
          </div>
          {nothingSummarised && (
            <p role="alert" className="flex items-center gap-2 text-sm text-destructive">
              <AlertTriangle aria-hidden="true" className="h-4 w-4" /> Pick at least one category, or nothing will be
              summarised.
            </p>
          )}
        </fieldset>
      </section>

      <section aria-labelledby="summary-batch-heading" className="surface-card space-y-4 p-5">
        <div className="space-y-1">
          <h2 id="summary-batch-heading" className="font-display text-xl font-medium">
            Batch size
          </h2>
          <p className="text-sm text-muted-foreground">
            How many messages go to the AI in each request when building a summary.
          </p>
        </div>
        <div role="radiogroup" aria-label="Batch size" className="grid gap-2 sm:grid-cols-2">
          {[...batchOptions, ...(batchOptions.some((o) => o.value === draft.chunkSize) ? [] : [{ value: draft.chunkSize, label: `Custom — ${draft.chunkSize} messages`, hint: "Set earlier" }])].map(
            (o) => (
              <button
                key={o.value}
                type="button"
                role="radio"
                aria-checked={draft.chunkSize === o.value}
                onClick={() => setDraft((d) => d && { ...d, chunkSize: o.value })}
                className={cn(
                  "rounded-md border px-3 py-2.5 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  draft.chunkSize === o.value ? "border-foreground bg-secondary" : "border-border hover:border-foreground/40",
                )}
              >
                <span className="block text-sm font-medium">{o.label}</span>
                <span className="block text-xs text-muted-foreground">{o.hint}</span>
              </button>
            ),
          )}
        </div>
      </section>

      <SaveBar
        dirty={dirty}
        saving={save.isPending}
        canSave={!nothingSummarised}
        onSave={() => save.mutate()}
        onDiscard={() => saved && setDraft(saved)}
      />
    </div>
  );
}
