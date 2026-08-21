import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/components/auth/AuthProvider";
import {
  ApiError,
  assignMessageProject,
  listProjects,
} from "@/lib/auth";
import { toast } from "@/hooks/use-toast";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

type Props = {
  messageID: string;
  hasConversation: boolean;
};

export function ProjectAssignControl({ messageID, hasConversation }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [projectID, setProjectID] = useState("");

  const projectsQuery = useQuery({
    queryKey: ["projects", accessToken],
    queryFn: () => listProjects(accessToken!),
    enabled: Boolean(accessToken),
  });

  const assignMutation = useMutation({
    mutationFn: async (scope: "thread" | "message") => {
      if (!accessToken) throw new Error("Not authenticated");
      return assignMessageProject(accessToken, messageID, {
        project_id: projectID,
        scope,
        status: "committed",
      });
    },
    onSuccess: async () => {
      toast({ title: "Project assigned" });
      await queryClient.invalidateQueries({ queryKey: ["unassigned"] });
      await queryClient.invalidateQueries({ queryKey: ["unassigned-summary"] });
    },
    onError: (err) => {
      toast({
        title: "Assign failed",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const projects = projectsQuery.data ?? [];
  if (projects.length === 0) return null;

  return (
    <div className="mt-3 flex flex-wrap items-center gap-2">
      <Select value={projectID} onValueChange={setProjectID}>
        <SelectTrigger className="h-8 w-[180px] text-xs">
          <SelectValue placeholder="Assign project" />
        </SelectTrigger>
        <SelectContent>
          {projects.map((p) => (
            <SelectItem key={p.id} value={p.id}>
              {p.code} — {p.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button
        size="sm"
        className="h-8"
        disabled={!projectID || assignMutation.isPending || !hasConversation}
        onClick={() => assignMutation.mutate("thread")}
      >
        This thread
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="h-8"
        disabled={!projectID || assignMutation.isPending}
        onClick={() => assignMutation.mutate("message")}
      >
        This message only
      </Button>
    </div>
  );
}
