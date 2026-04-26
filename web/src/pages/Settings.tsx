import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/components/auth/AuthProvider";
import {
  ApiError,
  createCategory,
  deleteCategory,
  listCategories,
  updateCategory,
  type UpsertCategoryInput,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Plus, Save, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

type CategoryFormState = UpsertCategoryInput;

const blankCategory: CategoryFormState = {
  slug: "",
  display_name: "",
  definition: "",
  sort_order: 0,
};

const noReplacementValue = "__none__";

export default function SettingsPage() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [newCategory, setNewCategory] = useState<CategoryFormState>(blankCategory);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [replacementByID, setReplacementByID] = useState<Record<string, string>>({});
  const [isEditMode, setIsEditMode] = useState(false);
  const [draftByID, setDraftByID] = useState<Record<string, CategoryFormState>>({});

  const categoriesQuery = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categoryList = useMemo(() => categoriesQuery.data ?? [], [categoriesQuery.data]);

  useEffect(() => {
    const next: Record<string, CategoryFormState> = {};
    for (const c of categoryList) {
      next[c.id] = {
        slug: c.slug,
        display_name: c.display_name,
        definition: c.definition ?? "",
        sort_order: c.sort_order,
      };
    }
    setDraftByID(next);
  }, [categoryList]);

  const saveMutation = useMutation({
    mutationFn: async (payload: { id: string; body: CategoryFormState }[]) => {
      if (!accessToken) throw new Error("Not authenticated");
      for (const item of payload) {
        await updateCategory(accessToken, item.id, item.body);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["categories"] });
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      setIsEditMode(false);
      toast({ title: "Category updated" });
    },
    onError: (err) => {
      toast({
        title: "Update failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  const createMutation = useMutation({
    mutationFn: async (payload: CategoryFormState) => {
      if (!accessToken) throw new Error("Not authenticated");
      return createCategory(accessToken, payload);
    },
    onSuccess: () => {
      setNewCategory(blankCategory);
      setAddDialogOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["categories"] });
      toast({ title: "Category created" });
    },
    onError: (err) => {
      toast({
        title: "Create failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (payload: { id: string; replacementID?: string }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return deleteCategory(accessToken, payload.id, payload.replacementID);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["categories"] });
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      toast({ title: "Category deleted" });
    },
    onError: (err) => {
      toast({
        title: "Delete failed",
        description: err instanceof ApiError ? err.message : "Please retry.",
        variant: "destructive",
      });
    },
  });

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Configuration"
        title="Settings"
        description="Configure category vocabulary used for Inbox filters and LLM categorization."
      />

      <Tabs defaultValue="categorization" className="space-y-4">
        <TabsList>
          <TabsTrigger value="categorization">Categorization</TabsTrigger>
        </TabsList>

        <TabsContent value="categorization" className="space-y-4">
          <div className="surface-card p-4">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div>
                <h3 className="font-display text-lg">Category definitions</h3>
                <p className="text-sm text-muted-foreground">
                  One row per category. Use Edit to update labels, slugs, definitions, and order.
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Dialog open={addDialogOpen} onOpenChange={setAddDialogOpen}>
                  <DialogTrigger asChild>
                    <Button variant="outline" disabled={createMutation.isPending}>
                      <Plus className="mr-1.5 h-3.5 w-3.5" />
                      Add category
                    </Button>
                  </DialogTrigger>
                  <DialogContent className="max-w-2xl">
                    <DialogHeader>
                      <DialogTitle>Add category</DialogTitle>
                      <DialogDescription>
                        Define a category and describe what emails should map to it.
                      </DialogDescription>
                    </DialogHeader>
                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <Label htmlFor="new-category-display-name">Display name</Label>
                        <Input
                          id="new-category-display-name"
                          placeholder="Example: Travel"
                          value={newCategory.display_name}
                          onChange={(e) => setNewCategory((prev) => ({ ...prev, display_name: e.target.value }))}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="new-category-slug">Slug</Label>
                        <Input
                          id="new-category-slug"
                          placeholder="example: travel"
                          value={newCategory.slug}
                          onChange={(e) => setNewCategory((prev) => ({ ...prev, slug: e.target.value }))}
                        />
                      </div>
                      <div className="space-y-2">
                        <Label htmlFor="new-category-sort-order">Sort order</Label>
                        <Input
                          id="new-category-sort-order"
                          type="number"
                          value={newCategory.sort_order}
                          onChange={(e) =>
                            setNewCategory((prev) => ({ ...prev, sort_order: Number(e.target.value) || 0 }))
                          }
                        />
                      </div>
                      <div className="space-y-2 md:col-span-2">
                        <Label htmlFor="new-category-definition">Definition</Label>
                        <Textarea
                          id="new-category-definition"
                          placeholder="Explain what kinds of emails should map to this category."
                          value={newCategory.definition}
                          onChange={(e) => setNewCategory((prev) => ({ ...prev, definition: e.target.value }))}
                        />
                      </div>
                    </div>
                    <DialogFooter>
                      <Button
                        className="bg-foreground text-background hover:bg-foreground/90"
                        disabled={createMutation.isPending}
                        onClick={() => createMutation.mutate(newCategory)}
                      >
                        {createMutation.isPending ? (
                          <>
                            <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                            Creating...
                          </>
                        ) : (
                          <>
                            <Plus className="mr-1.5 h-3.5 w-3.5" />
                            Create category
                          </>
                        )}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>

                <Button
                  variant={isEditMode ? "default" : "outline"}
                  className={isEditMode ? "bg-foreground text-background hover:bg-foreground/90" : ""}
                  disabled={saveMutation.isPending || deleteMutation.isPending}
                  onClick={() => {
                    if (!isEditMode) {
                      setIsEditMode(true);
                      return;
                    }
                    const payload = categoryList.map((c) => ({ id: c.id, body: draftByID[c.id] ?? blankCategory }));
                    saveMutation.mutate(payload);
                  }}
                >
                  {saveMutation.isPending ? (
                    <>
                      <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                      Saving...
                    </>
                  ) : isEditMode ? (
                    <>
                      <Save className="mr-1.5 h-3.5 w-3.5" />
                      Save
                    </>
                  ) : (
                    <>
                      <Pencil className="mr-1.5 h-3.5 w-3.5" />
                      Edit
                    </>
                  )}
                </Button>
              </div>
            </div>

            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[190px]">Display name</TableHead>
                  <TableHead className="w-[160px]">Slug</TableHead>
                  <TableHead>Definition</TableHead>
                  <TableHead className="w-[120px]">Sort order</TableHead>
                  <TableHead className="w-[320px]">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {categoryList.map((c) => {
                  const draft = draftByID[c.id] ?? {
                    slug: c.slug,
                    display_name: c.display_name,
                    definition: c.definition ?? "",
                    sort_order: c.sort_order,
                  };
                  return (
                    <TableRow key={c.id}>
                      <TableCell>
                        {isEditMode ? (
                          <Input
                            value={draft.display_name}
                            onChange={(e) =>
                              setDraftByID((prev) => ({
                                ...prev,
                                [c.id]: { ...draft, display_name: e.target.value },
                              }))
                            }
                          />
                        ) : (
                          <span>{c.display_name}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {isEditMode ? (
                          <Input
                            value={draft.slug}
                            onChange={(e) =>
                              setDraftByID((prev) => ({
                                ...prev,
                                [c.id]: { ...draft, slug: e.target.value },
                              }))
                            }
                          />
                        ) : (
                          <code className="text-xs">{c.slug}</code>
                        )}
                      </TableCell>
                      <TableCell>
                        {isEditMode ? (
                          <Textarea
                            className="min-h-20"
                            value={draft.definition}
                            onChange={(e) =>
                              setDraftByID((prev) => ({
                                ...prev,
                                [c.id]: { ...draft, definition: e.target.value },
                              }))
                            }
                          />
                        ) : (
                          <span className="text-sm text-muted-foreground">{c.definition || "No definition provided."}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {isEditMode ? (
                          <Input
                            type="number"
                            value={draft.sort_order}
                            onChange={(e) =>
                              setDraftByID((prev) => ({
                                ...prev,
                                [c.id]: { ...draft, sort_order: Number(e.target.value) || 0 },
                              }))
                            }
                          />
                        ) : (
                          <span>{c.sort_order}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Select
                            value={replacementByID[c.id] || noReplacementValue}
                            onValueChange={(value) =>
                              setReplacementByID((prev) => ({
                                ...prev,
                                [c.id]: value === noReplacementValue ? "" : value,
                              }))
                            }
                          >
                            <SelectTrigger className="w-[210px]">
                              <SelectValue placeholder="Replacement category" />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value={noReplacementValue}>No replacement</SelectItem>
                              {categoryList
                                .filter((replacement) => replacement.id !== c.id)
                                .map((replacement) => (
                                  <SelectItem key={replacement.id} value={replacement.id}>
                                    {replacement.display_name}
                                  </SelectItem>
                                ))}
                            </SelectContent>
                          </Select>
                          <Button
                            size="sm"
                            variant="destructive"
                            disabled={deleteMutation.isPending || categoryList.length <= 1}
                            onClick={() =>
                              deleteMutation.mutate({
                                id: c.id,
                                replacementID: replacementByID[c.id] || undefined,
                              })
                            }
                          >
                            <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                            Delete
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
