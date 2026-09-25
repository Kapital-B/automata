import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, FolderKanban, Loader2, Plus } from "lucide-react";
import { PageHeader } from "@/components/PageHeader";
import { SearchField } from "@/components/SearchField";
import { ListSkeleton } from "@/components/ListSkeleton";
import { Tag } from "@/components/ConnectionCard";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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
import { ApiError, createProject, listProjects, type ProjectListItem } from "@/lib/auth";

function parseKeywords(raw: string): string[] {
  return Array.from(
    new Set(
      raw
        .split(/[,;\n]+/)
        .map((k) => k.trim())
        .filter(Boolean),
    ),
  );
}

// The server's rule, checked as the code is typed rather than after submit.
function codeProblem(code: string): string | null {
  if (code === "") return null;
  if (!/^[A-Z]/.test(code)) return "Start with a letter.";
  if (!/^[A-Z0-9]*$/.test(code)) return "Letters and digits only.";
  if (code.length < 2) return "At least 2 characters.";
  if (code.length > 8) return "At most 8 characters.";
  return null;
}

function matches(p: ProjectListItem, q: string): boolean {
  const needle = q.toLowerCase();
  return [p.name, p.code, p.client ?? "", p.description ?? "", ...(p.keywords ?? [])].some((v) =>
    v.toLowerCase().includes(needle),
  );
}

export default function ProjectsPage() {
  const { accessToken } = useAuth();
  const [q, setQ] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const [creating, setCreating] = useState(false);

  const query = useQuery({
    queryKey: ["projects", accessToken, showArchived],
    queryFn: () => listProjects(accessToken!, showArchived),
    enabled: Boolean(accessToken),
    placeholderData: (previous) => previous,
  });

  const all = useMemo(() => query.data ?? [], [query.data]);
  // Active first, then by code, so the list reads the same every time.
  const projects = useMemo(
    () =>
      all
        .filter((p) => !q.trim() || matches(p, q.trim()))
        .sort((a, b) => Number(Boolean(a.archived_at)) - Number(Boolean(b.archived_at)) || a.code.localeCompare(b.code)),
    [all, q],
  );

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Correspondence"
        title="Projects"
        description="Each project gathers the mail, decisions and issues that belong to it. Its code and keywords help file correspondence automatically."
        actions={
          <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90" onClick={() => setCreating(true)}>
            <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> New project
          </Button>
        }
      />

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SearchField value={q} onChange={setQ} label="Search projects" placeholder="Search name, code, client or keyword" />
        <div className="flex items-center gap-4">
          {!query.isLoading && !query.isError && all.length > 0 && (
            <p className="text-sm text-muted-foreground" aria-live="polite">
              {q.trim()
                ? `${projects.length} of ${all.length}`
                : `${all.length} ${all.length === 1 ? "project" : "projects"}`}
            </p>
          )}
          <div className="flex items-center gap-2">
            <Switch id="projects-archived" checked={showArchived} onCheckedChange={setShowArchived} />
            <Label htmlFor="projects-archived" className="text-sm font-normal">
              Show archived
            </Label>
          </div>
        </div>
      </div>

      {query.isLoading ? (
        <ListSkeleton label="Loading projects" />
      ) : query.isError ? (
        <div role="alert" className="surface-card p-5 text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Could not load projects."}
        </div>
      ) : projects.length === 0 ? (
        <div className="surface-card flex flex-col items-center gap-3 px-6 py-12 text-center">
          <FolderKanban aria-hidden="true" className="h-8 w-8 text-muted-foreground" />
          {q.trim() ? (
            <>
              <p className="text-sm text-muted-foreground">No project matches “{q.trim()}”.</p>
              <button
                type="button"
                onClick={() => setQ("")}
                className="text-sm font-medium text-primary underline-offset-4 hover:underline"
              >
                Show all projects
              </button>
            </>
          ) : (
            <>
              <p className="font-medium">No projects yet</p>
              <p className="max-w-sm text-sm text-muted-foreground">
                Create one with a short code like DC01. Mail that mentions the code is filed to it automatically.
              </p>
              <Button size="sm" onClick={() => setCreating(true)}>
                <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> New project
              </Button>
            </>
          )}
        </div>
      ) : (
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {projects.map((p) => (
            <ProjectRow key={p.id} project={p} />
          ))}
        </ul>
      )}

      <CreateProjectDialog open={creating} onOpenChange={setCreating} />
    </div>
  );
}

function ProjectRow({ project: p }: { project: ProjectListItem }) {
  const secondary = [p.client, p.description].filter(Boolean).join(" · ");
  const keywords = p.keywords ?? [];
  return (
    <li>
      <Link
        to={`/projects/${p.id}`}
        className="group flex min-h-14 items-center gap-3 px-4 py-2.5 transition-colors hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
      >
        <span
          aria-hidden="true"
          className="inline-flex h-9 min-w-9 shrink-0 items-center justify-center rounded-md border border-border bg-secondary px-1.5 font-mono text-[11px] font-medium"
        >
          {p.code}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="truncate font-medium text-foreground">{p.name}</p>
            {p.archived_at && <Tag>Archived</Tag>}
          </div>
          {secondary ? (
            <p className="truncate text-sm text-muted-foreground">{secondary}</p>
          ) : keywords.length > 0 ? (
            <p className="truncate text-sm text-muted-foreground">
              {keywords.slice(0, 4).join(", ")}
              {keywords.length > 4 && ` +${keywords.length - 4}`}
            </p>
          ) : null}
        </div>
        <span className="sr-only">Code {p.code}</span>
        <ChevronRight
          aria-hidden="true"
          className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 motion-reduce:transition-none motion-reduce:group-hover:translate-x-0"
        />
      </Link>
    </li>
  );
}

function CreateProjectDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [keywords, setKeywords] = useState("");
  const [error, setError] = useState<string | null>(null);

  const problem = codeProblem(code);
  const parsed = parseKeywords(keywords);
  const ready = name.trim() !== "" && code !== "" && problem === null;

  const reset = () => {
    setName("");
    setCode("");
    setKeywords("");
    setError(null);
  };

  const create = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return createProject(accessToken, { name: name.trim(), code, keywords: parsed });
    },
    onSuccess: async () => {
      toast({ title: `${code} created` });
      onOpenChange(false);
      reset();
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not create the project. Please try again."),
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) reset();
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">New project</DialogTitle>
          <DialogDescription>You can add the client, team and description on the project page afterwards.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (ready && !create.isPending) create.mutate();
          }}
        >
          <div className="grid gap-4 sm:grid-cols-[1fr_9rem]">
            <div className="space-y-1.5">
              <Label htmlFor="project-name">Name</Label>
              <Input id="project-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Cooling upgrade" autoFocus />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="project-code">Code</Label>
              <Input
                id="project-code"
                value={code}
                maxLength={8}
                onChange={(e) => setCode(e.target.value.toUpperCase().replace(/\s+/g, ""))}
                placeholder="DC01"
                className="font-mono uppercase"
                aria-invalid={problem !== null}
                aria-describedby="project-code-help"
              />
            </div>
          </div>
          <p id="project-code-help" className={problem ? "-mt-2 text-xs text-destructive" : "-mt-2 text-xs text-muted-foreground"}>
            {problem ?? "2–8 letters and digits, starting with a letter. Mail that mentions it is filed here automatically."}
          </p>
          <div className="space-y-1.5">
            <Label htmlFor="project-keywords">
              Keywords <span className="font-normal text-muted-foreground">(optional)</span>
            </Label>
            <Input
              id="project-keywords"
              value={keywords}
              onChange={(e) => setKeywords(e.target.value)}
              placeholder="chiller, P-03, cooling tower"
              aria-describedby="project-keywords-help"
            />
            {parsed.length > 0 ? (
              <ul id="project-keywords-help" aria-label="Keywords" className="flex flex-wrap gap-1.5 pt-1">
                {parsed.map((k) => (
                  <li key={k} className="rounded-full bg-secondary px-2 py-0.5 text-xs">
                    {k}
                  </li>
                ))}
              </ul>
            ) : (
              <p id="project-keywords-help" className="text-xs text-muted-foreground">
                Separate with commas. Words that tie mail to this project.
              </p>
            )}
          </div>
          {error && (
            <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!ready || create.isPending}>
              {create.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
              Create project
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
