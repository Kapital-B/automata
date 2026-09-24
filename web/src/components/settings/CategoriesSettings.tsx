import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Loader2, Pencil, Plus, Tags, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import {
  ApiError,
  createCategory,
  deleteCategory,
  listCategories,
  updateCategory,
  type CategoryDefinition,
} from "@/lib/auth";
import { slugFor } from "@/lib/categories";

const MAX_DEFINITION = 280;
const selectClass =
  "h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

type Editing = { mode: "create" } | { mode: "edit"; category: CategoryDefinition } | null;

export function CategoriesSettings() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<Editing>(null);
  const [deleting, setDeleting] = useState<CategoryDefinition | null>(null);

  const query = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categories = useMemo(
    () => [...(query.data ?? [])].sort((a, b) => a.sort_order - b.sort_order || a.display_name.localeCompare(b.display_name)),
    [query.data],
  );
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["categories"] });
    void queryClient.invalidateQueries({ queryKey: ["messages"] });
  };

  // Moving writes an even spacing for the whole list, so orders that were
  // never set (or collide) settle on the first move.
  const move = useMutation({
    mutationFn: async ({ index, delta }: { index: number; delta: -1 | 1 }) => {
      if (!accessToken) throw new Error("Not authenticated");
      const next = [...categories];
      const [item] = next.splice(index, 1);
      next.splice(index + delta, 0, item);
      await Promise.all(
        next
          .map((c, i) => ({ c, order: (i + 1) * 10 }))
          .filter(({ c, order }) => c.sort_order !== order)
          .map(({ c, order }) =>
            updateCategory(accessToken, c.id, {
              slug: c.slug,
              display_name: c.display_name,
              definition: c.definition ?? "",
              sort_order: order,
            }),
          ),
      );
    },
    onSuccess: invalidate,
    onError: (e) =>
      toast({ title: "Could not reorder", description: e instanceof ApiError ? e.message : "Please try again.", variant: "destructive" }),
  });

  return (
    <section aria-labelledby="categories-heading" className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="space-y-1">
          <h2 id="categories-heading" className="font-display text-2xl font-medium">
            Categories
          </h2>
          <p className="max-w-2xl text-sm text-muted-foreground">
            The AI sorts each message into one of these, using the description to decide. They appear in this order in
            the Inbox filters.
          </p>
        </div>
        <Button size="sm" onClick={() => setEditing({ mode: "create" })}>
          <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add category
        </Button>
      </div>

      {query.isLoading ? (
        <div aria-label="Loading categories" className="surface-card space-y-3 p-5">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-80" />
          <Skeleton className="h-5 w-36" />
        </div>
      ) : query.isError ? (
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          Could not load categories.
        </div>
      ) : categories.length === 0 ? (
        <div className="surface-card flex items-center gap-3 p-5 text-sm text-muted-foreground">
          <Tags aria-hidden="true" className="h-5 w-5" /> No categories yet.
        </div>
      ) : (
        <ol className="surface-card divide-y divide-border/70 overflow-hidden">
          {categories.map((c, i) => (
            <li key={c.id} className="flex items-start gap-3 px-4 py-3">
              <div className="flex shrink-0 flex-col">
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-9 w-9"
                  aria-label={`Move ${c.display_name} up`}
                  disabled={i === 0 || move.isPending}
                  onClick={() => move.mutate({ index: i, delta: -1 })}
                >
                  <ArrowUp aria-hidden="true" className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="h-9 w-9"
                  aria-label={`Move ${c.display_name} down`}
                  disabled={i === categories.length - 1 || move.isPending}
                  onClick={() => move.mutate({ index: i, delta: 1 })}
                >
                  <ArrowDown aria-hidden="true" className="h-3.5 w-3.5" />
                </Button>
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="font-medium">{c.display_name}</p>
                  <code className="rounded bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">{c.slug}</code>
                </div>
                <p className="mt-0.5 line-clamp-2 text-sm text-muted-foreground">
                  {c.definition || <span className="italic">No description. The AI will guess from the name alone.</span>}
                </p>
              </div>
              <div className="flex shrink-0 gap-1">
                <Button size="sm" variant="ghost" onClick={() => setEditing({ mode: "edit", category: c })}>
                  <Pencil aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Edit
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="text-muted-foreground hover:text-destructive"
                  aria-label={`Delete ${c.display_name}`}
                  disabled={categories.length <= 1}
                  title={categories.length <= 1 ? "At least one category is needed" : undefined}
                  onClick={() => setDeleting(c)}
                >
                  <Trash2 aria-hidden="true" className="h-4 w-4" />
                </Button>
              </div>
            </li>
          ))}
        </ol>
      )}

      <CategoryDialog
        editing={editing}
        nextOrder={(categories.length + 1) * 10}
        existingSlugs={categories.map((c) => c.slug)}
        onClose={() => setEditing(null)}
        onSaved={invalidate}
      />
      <DeleteCategoryDialog
        category={deleting}
        others={categories.filter((c) => c.id !== deleting?.id)}
        onClose={() => setDeleting(null)}
        onDeleted={invalidate}
      />
    </section>
  );
}

function CategoryDialog({
  editing,
  nextOrder,
  existingSlugs,
  onClose,
  onSaved,
}: {
  editing: Editing;
  nextOrder: number;
  existingSlugs: string[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const { accessToken } = useAuth();
  const [name, setName] = useState("");
  const [definition, setDefinition] = useState("");
  const [error, setError] = useState<string | null>(null);
  const category = editing?.mode === "edit" ? editing.category : undefined;

  useEffect(() => {
    if (!editing) return;
    setName(category?.display_name ?? "");
    setDefinition(category?.definition ?? "");
    setError(null);
  }, [editing, category]);

  // The slug is what rules and summary filters refer to, so it is set once,
  // from the name, and never changes: renaming a category is safe.
  const slug = category ? category.slug : slugFor(name);
  const slugTaken = !category && slug !== "" && existingSlugs.includes(slug);
  const ready = name.trim() !== "" && slug !== "" && !slugTaken && definition.length <= MAX_DEFINITION;

  const save = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      const body = { slug, display_name: name.trim(), definition: definition.trim(), sort_order: category?.sort_order ?? nextOrder };
      return category ? updateCategory(accessToken, category.id, body) : createCategory(accessToken, body);
    },
    onSuccess: () => {
      onSaved();
      toast({ title: category ? "Category saved" : "Category added" });
      onClose();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not save the category."),
  });

  return (
    <Dialog open={editing !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">{category ? `Edit ${category.display_name}` : "Add a category"}</DialogTitle>
          <DialogDescription>
            A clear description is what lets the AI put the right mail here. Say what belongs, and what does not.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (ready && !save.isPending) save.mutate();
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="category-name">Name</Label>
            <Input id="category-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Travel" autoFocus />
            <p className="text-xs text-muted-foreground">
              {category ? (
                <>
                  Identifier <code className="text-foreground/80">{slug}</code> stays the same when you rename it, so rules and
                  filters keep working.
                </>
              ) : slug ? (
                slugTaken ? (
                  <span className="text-destructive">A category with the identifier “{slug}” already exists.</span>
                ) : (
                  <>
                    Identifier: <code className="text-foreground/80">{slug}</code>
                  </>
                )
              ) : (
                "Letters or numbers, please."
              )}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="category-definition">Description</Label>
            <Textarea
              id="category-definition"
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              rows={3}
              placeholder="Flights, hotels, itineraries and travel receipts. Not conference invitations."
              aria-describedby="category-definition-count"
            />
            <p
              id="category-definition-count"
              className={definition.length > MAX_DEFINITION ? "text-xs text-destructive" : "text-xs text-muted-foreground"}
            >
              {definition.length}/{MAX_DEFINITION}
            </p>
          </div>
          {error && (
            <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={!ready || save.isPending}>
              {save.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
              {category ? "Save changes" : "Add category"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function DeleteCategoryDialog({
  category,
  others,
  onClose,
  onDeleted,
}: {
  category: CategoryDefinition | null;
  others: CategoryDefinition[];
  onClose: () => void;
  onDeleted: () => void;
}) {
  const { accessToken } = useAuth();
  const [replacement, setReplacement] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Reset only when a different category is opened: `others` is rebuilt on
  // every render, and depending on it would undo the user's choice.
  useEffect(() => {
    if (!category) return;
    setError(null);
    setReplacement(others.find((c) => c.slug === "other")?.id ?? others[0]?.id ?? "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [category?.id]);

  const remove = useMutation({
    mutationFn: async () => {
      if (!accessToken || !category) throw new Error("Not authenticated");
      return deleteCategory(accessToken, category.id, replacement || undefined);
    },
    onSuccess: () => {
      onDeleted();
      toast({ title: `${category?.display_name} deleted` });
      onClose();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not delete the category."),
  });

  return (
    <Dialog open={category !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Delete {category?.display_name}?</DialogTitle>
          <DialogDescription>
            Mail already in this category moves to the one you choose. Rules or summary filters that name it stop matching.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="category-replacement">Move its messages to</Label>
          <select
            id="category-replacement"
            className={selectClass}
            value={replacement}
            onChange={(e) => setReplacement(e.target.value)}
          >
            {others.map((c) => (
              <option key={c.id} value={c.id}>
                {c.display_name}
              </option>
            ))}
          </select>
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="button" variant="destructive" onClick={() => remove.mutate()} disabled={!replacement || remove.isPending}>
            {remove.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            Delete category
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
