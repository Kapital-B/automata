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
import { useId, useState } from "react";

type Props = {
  messageID: string;
  hasConversation: boolean;
  /** The project the message is filed to now, if any. Key the control by message so this resets. */
  currentProjectID?: string;
};

export function ProjectAssignControl({ messageID, hasConversation, currentProjectID }: Props) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [projectID, setProjectID] = useState(currentProjectID ?? "");
  const labelID = useId();

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
      // In parallel: the summary query counts the whole queue, so serialising
      // these made every assignment wait on two full passes.
      await Promise.all(
        ["unassigned", "unassigned-summary", "messages", "project-timeline", "attention", "project-todos"].map((key) =>
          queryClient.invalidateQueries({ queryKey: [key] }),
        ),
      );
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
    <div className="space-y-2">
      <p id={labelID} className="text-xs font-medium text-muted-foreground">
        File to a project
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Select value={projectID} onValueChange={setProjectID}>
          <SelectTrigger aria-labelledby={labelID} className="h-9 w-full text-sm sm:w-[240px]">
            <SelectValue placeholder="Choose a project" />
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
          className="h-9"
          disabled={!projectID || assignMutation.isPending || !hasConversation}
          onClick={() => assignMutation.mutate("thread")}
        >
          Whole thread
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="h-9"
          disabled={!projectID || assignMutation.isPending}
          onClick={() => assignMutation.mutate("message")}
        >
          This message only
        </Button>
      </div>
    </div>
  );
}
