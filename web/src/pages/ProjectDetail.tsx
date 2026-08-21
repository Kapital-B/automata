import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/components/auth/AuthProvider";
import {
  ApiError,
  getProject,
  updateProject,
  updateProjectMember,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const query = useQuery({
    queryKey: ["project", accessToken, id],
    queryFn: () => getProject(accessToken!, id!),
    enabled: Boolean(accessToken && id),
  });

  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [discipline, setDiscipline] = useState("");
  const [scope, setScope] = useState("");

  useEffect(() => {
    if (!query.data) return;
    setName(query.data.name);
    setRole(query.data.member?.role ?? "");
    setDiscipline(query.data.member?.discipline ?? "");
    setScope(query.data.member?.current_scope ?? "");
  }, [query.data]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      await updateProject(accessToken, id, { name: name.trim() });
      await updateProjectMember(accessToken, id, {
        role: role.trim(),
        discipline: discipline.trim() || null,
        current_scope: scope.trim() || null,
      });
    },
    onSuccess: async () => {
      toast({ title: "Project saved" });
      await queryClient.invalidateQueries({ queryKey: ["project", accessToken, id] });
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
    },
    onError: (err) => {
      toast({
        title: "Save failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const archiveMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !id) throw new Error("Not authenticated");
      return updateProject(accessToken, id, { archived: true });
    },
    onSuccess: async () => {
      toast({ title: "Project archived" });
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      navigate("/projects");
    },
  });

  if (query.isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading project…
      </div>
    );
  }

  if (query.isError || !query.data) {
    return (
      <div className="space-y-4">
        <p className="text-sm text-destructive">
          {query.error instanceof ApiError ? query.error.message : "Project not found."}
        </p>
        <Button variant="outline" onClick={() => navigate("/projects")}>
          Back to Projects
        </Button>
      </div>
    );
  }

  const project = query.data;

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow={project.code}
        title={project.name}
        description="Project timeline arrives in the next slice. Edit header and your member role here."
        actions={
          <div className="flex gap-2">
            <Button variant="outline" asChild>
              <Link to="/projects">All projects</Link>
            </Button>
            <Button
              variant="outline"
              disabled={archiveMutation.isPending}
              onClick={() => archiveMutation.mutate()}
            >
              Archive
            </Button>
          </div>
        }
      />

      <section className="max-w-xl space-y-4">
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="proj-name">
            Name
          </label>
          <Input id="proj-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Code</label>
          <p className="font-mono text-sm">{project.code}</p>
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="member-role">
            Your role
          </label>
          <Input
            id="member-role"
            value={role}
            onChange={(e) => setRole(e.target.value)}
            placeholder="Mechanical Engineer"
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="member-discipline">
            Discipline
          </label>
          <Input
            id="member-discipline"
            value={discipline}
            onChange={(e) => setDiscipline(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="member-scope">
            Current scope
          </label>
          <Input id="member-scope" value={scope} onChange={(e) => setScope(e.target.value)} />
        </div>
        <Button disabled={saveMutation.isPending} onClick={() => saveMutation.mutate()}>
          {saveMutation.isPending ? "Saving…" : "Save"}
        </Button>
      </section>
    </div>
  );
}
