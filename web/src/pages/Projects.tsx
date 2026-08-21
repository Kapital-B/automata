import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, createProject, listProjects, type ProjectListItem } from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderKanban, Loader2, Plus } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

export default function ProjectsPage() {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [code, setCode] = useState("");

  const query = useQuery({
    queryKey: ["projects", accessToken],
    queryFn: () => listProjects(accessToken!),
    enabled: Boolean(accessToken),
  });

  const createMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return createProject(accessToken, { name: name.trim(), code: code.trim() });
    },
    onSuccess: async () => {
      toast({ title: "Project created" });
      setOpen(false);
      setName("");
      setCode("");
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
    onError: (err) => {
      toast({
        title: "Could not create project",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const projects = query.data ?? [];

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Correspondence"
        title="Projects"
        description="Structured project codes for assigning mail threads."
        actions={
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button>
                <Plus className="mr-2 h-4 w-4" />
                New project
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create project</DialogTitle>
                <DialogDescription>
                  Code must be 2–8 characters (e.g. DC01), letters and digits, starting with a
                  letter.
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-3">
                <div className="space-y-1">
                  <label className="text-xs text-muted-foreground" htmlFor="project-name">
                    Name
                  </label>
                  <Input
                    id="project-name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="Cooling Upgrade"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs text-muted-foreground" htmlFor="project-code">
                    Code
                  </label>
                  <Input
                    id="project-code"
                    value={code}
                    onChange={(e) => setCode(e.target.value.toUpperCase())}
                    placeholder="DC01"
                    className="font-mono uppercase"
                  />
                </div>
                <Button
                  className="w-full"
                  disabled={!name.trim() || !code.trim() || createMutation.isPending}
                  onClick={() => createMutation.mutate()}
                >
                  {createMutation.isPending ? "Creating…" : "Create"}
                </Button>
              </div>
            </DialogContent>
          </Dialog>
        }
      />

      {query.isLoading ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading projects…
        </div>
      ) : query.isError ? (
        <p className="text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Could not load projects."}
        </p>
      ) : projects.length === 0 ? (
        <div className="flex flex-col items-start gap-3 py-10 text-muted-foreground">
          <FolderKanban className="h-8 w-8 opacity-60" />
          <p className="text-sm">No projects yet. Create one with a code like DC01.</p>
        </div>
      ) : (
        <ul className="divide-y divide-border/70 border-y border-border/70">
          {projects.map((p) => (
            <ProjectRow key={p.id} project={p} />
          ))}
        </ul>
      )}
    </div>
  );
}

function ProjectRow({ project }: { project: ProjectListItem }) {
  return (
    <li>
      <Link
        to={`/projects/${project.id}`}
        className="flex items-center justify-between gap-4 py-3 transition-colors hover:bg-muted/40"
      >
        <div>
          <p className="font-medium text-foreground">{project.name}</p>
          <p className="font-mono text-xs text-muted-foreground">{project.code}</p>
        </div>
        <span className="text-xs text-muted-foreground">Open</span>
      </Link>
    </li>
  );
}
