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
import { useAccountsData } from "@/hooks/useAccountsData";
import {
  ApiError,
  createCategory,
  deleteCategory,
  getSummarySettings,
  getScheduleSettings,
  listCategories,
  type ScheduleChain,
  updateSummarySettings,
  updateScheduleSettings,
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
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const [newCategory, setNewCategory] = useState<CategoryFormState>(blankCategory);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [replacementByID, setReplacementByID] = useState<Record<string, string>>({});
  const [isEditMode, setIsEditMode] = useState(false);
  const [draftByID, setDraftByID] = useState<Record<string, CategoryFormState>>({});
  const [includeSlugs, setIncludeSlugs] = useState<string[]>([]);
  const [excludeSlugs, setExcludeSlugs] = useState<string[]>([]);
  const [chunkSize, setChunkSize] = useState<number>(12);
  const [scheduleChains, setScheduleChains] = useState<ScheduleChain[]>([]);

  const categoriesQuery = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const categoryList = useMemo(() => categoriesQuery.data ?? [], [categoriesQuery.data]);
  const summarySettingsQuery = useQuery({
    queryKey: ["summary-settings", accessToken],
    queryFn: () => getSummarySettings(accessToken!),
    enabled: Boolean(accessToken),
  });
  const schedulesQuery = useQuery({
    queryKey: ["schedule-settings", accessToken],
    queryFn: () => getScheduleSettings(accessToken!),
    enabled: Boolean(accessToken),
  });

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
  useEffect(() => {
    const row = summarySettingsQuery.data;
    if (!row) return;
    setIncludeSlugs(row.include_category_slugs ?? []);
    setExcludeSlugs(row.exclude_category_slugs ?? []);
    setChunkSize(row.chunk_size || 12);
  }, [summarySettingsQuery.data]);
  useEffect(() => {
    setScheduleChains(schedulesQuery.data?.chains ?? []);
  }, [schedulesQuery.data]);

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
  const summarySettingsMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return updateSummarySettings(accessToken, {
        include_category_slugs: includeSlugs,
        exclude_category_slugs: excludeSlugs,
        chunk_size: chunkSize,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["summary-settings"] });
      toast({ title: "Summary settings updated" });
    },
  });
  const scheduleSettingsMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return updateScheduleSettings(accessToken, scheduleChains);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["schedule-settings"] });
      toast({ title: "Schedule settings updated" });
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
          <TabsTrigger value="summarization">Summarization</TabsTrigger>
          <TabsTrigger value="scheduling">Scheduling</TabsTrigger>
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
        <TabsContent value="summarization" className="space-y-4">
          <div className="surface-card space-y-4 p-4">
            <h3 className="font-display text-lg">Summary category filters</h3>
            <p className="text-sm text-muted-foreground">
              Include limits summaries to selected categories. Exclude always removes selected categories.
            </p>
            <div className="grid gap-4 md:grid-cols-2">
              <div>
                <p className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">Include</p>
                <div className="flex flex-wrap gap-2">
                  {categoryList.map((c) => (
                    <Button
                      key={`inc-${c.id}`}
                      size="sm"
                      variant={includeSlugs.includes(c.slug) ? "default" : "outline"}
                      onClick={() =>
                        setIncludeSlugs((prev) => (prev.includes(c.slug) ? prev.filter((s) => s !== c.slug) : [...prev, c.slug]))
                      }
                    >
                      {c.display_name}
                    </Button>
                  ))}
                </div>
              </div>
              <div>
                <p className="mb-2 text-xs uppercase tracking-wider text-muted-foreground">Exclude</p>
                <div className="flex flex-wrap gap-2">
                  {categoryList.map((c) => (
                    <Button
                      key={`exc-${c.id}`}
                      size="sm"
                      variant={excludeSlugs.includes(c.slug) ? "destructive" : "outline"}
                      onClick={() =>
                        setExcludeSlugs((prev) => (prev.includes(c.slug) ? prev.filter((s) => s !== c.slug) : [...prev, c.slug]))
                      }
                    >
                      {c.display_name}
                    </Button>
                  ))}
                </div>
              </div>
            </div>
            <div className="max-w-xs space-y-2">
              <Label htmlFor="summary-chunk-size">Chunk size</Label>
              <Input
                id="summary-chunk-size"
                type="number"
                min={3}
                max={30}
                value={chunkSize}
                onChange={(e) => setChunkSize(Number(e.target.value) || 12)}
              />
              <p className="text-xs text-muted-foreground">Messages per LLM summarize request (recommended: 8-16).</p>
            </div>
            <div>
              <Button onClick={() => summarySettingsMutation.mutate()} disabled={summarySettingsMutation.isPending}>
                {summarySettingsMutation.isPending ? "Saving..." : "Save summary settings"}
              </Button>
            </div>
          </div>
        </TabsContent>
        <TabsContent value="scheduling" className="space-y-4">
          <div className="surface-card space-y-4 p-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="font-display text-lg">Job schedule chains</h3>
                <p className="text-sm text-muted-foreground">Create recurring chains like sync -&gt; categorize -&gt; summarize.</p>
              </div>
              <Button
                variant="outline"
                onClick={() =>
                  setScheduleChains((prev) => [
                    ...prev,
                    {
                      id: crypto.randomUUID(),
                      name: "New chain",
                      jobs: ["sync", "categorize", "summarize"],
                      interval_minutes: 10,
                      enabled: true,
                    },
                  ])
                }
              >
                <Plus className="mr-1.5 h-3.5 w-3.5" />
                Add chain
              </Button>
            </div>
            <div className="space-y-3">
              {scheduleChains.map((chain) => (
                <div key={chain.id} className="rounded-md border border-border p-3 space-y-3">
                  <div className="grid gap-3 md:grid-cols-4">
                    <div className="space-y-1">
                      <Label>Name</Label>
                      <Input
                        value={chain.name}
                        onChange={(e) =>
                          setScheduleChains((prev) =>
                            prev.map((c) => (c.id === chain.id ? { ...c, name: e.target.value } : c)),
                          )
                        }
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Account</Label>
                      <Select
                        value={chain.account_id ?? "__all__"}
                        onValueChange={(value) =>
                          setScheduleChains((prev) =>
                            prev.map((c) => (c.id === chain.id ? { ...c, account_id: value === "__all__" ? undefined : value } : c)),
                          )
                        }
                      >
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="__all__">All connected accounts</SelectItem>
                          {accounts.map((a) => (
                            <SelectItem key={a.id} value={a.id}>
                              {a.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-1">
                      <Label>Every (minutes)</Label>
                      <Input
                        type="number"
                        min={1}
                        value={chain.interval_minutes}
                        onChange={(e) =>
                          setScheduleChains((prev) =>
                            prev.map((c) => (c.id === chain.id ? { ...c, interval_minutes: Number(e.target.value) || 10 } : c)),
                          )
                        }
                      />
                    </div>
                    <div className="space-y-1">
                      <Label>Enabled</Label>
                      <Select
                        value={chain.enabled ? "enabled" : "disabled"}
                        onValueChange={(value) =>
                          setScheduleChains((prev) =>
                            prev.map((c) => (c.id === chain.id ? { ...c, enabled: value === "enabled" } : c)),
                          )
                        }
                      >
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="enabled">Enabled</SelectItem>
                          <SelectItem value="disabled">Disabled</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className="space-y-1">
                    <Label>Jobs chain (comma separated)</Label>
                    <Input
                      value={chain.jobs.join(", ")}
                      onChange={(e) =>
                        setScheduleChains((prev) =>
                          prev.map((c) =>
                            c.id === chain.id
                              ? { ...c, jobs: e.target.value.split(",").map((v) => v.trim()).filter(Boolean) }
                              : c,
                          ),
                        )
                      }
                      placeholder="sync, categorize, summarize, forward_rules, auto-draft"
                    />
                    <p className="text-xs text-muted-foreground">Supported jobs: sync, categorize, summarize, forward_rules, auto-draft.</p>
                  </div>
                  <div>
                    <Button
                      variant="ghost"
                      className="text-destructive"
                      onClick={() => setScheduleChains((prev) => prev.filter((c) => c.id !== chain.id))}
                    >
                      <Trash2 className="mr-1.5 h-3.5 w-3.5" />
                      Remove chain
                    </Button>
                  </div>
                </div>
              ))}
              {scheduleChains.length === 0 && (
                <p className="text-sm text-muted-foreground">No schedule chains configured.</p>
              )}
            </div>
            <div>
              <Button onClick={() => scheduleSettingsMutation.mutate()} disabled={scheduleSettingsMutation.isPending}>
                {scheduleSettingsMutation.isPending ? "Saving..." : "Save schedules"}
              </Button>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
