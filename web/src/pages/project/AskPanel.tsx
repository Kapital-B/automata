import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, askProject } from "@/lib/auth";

/** Questions answered from this project's facts, decisions and mail. */
export function AskPanel({ projectID, enabled }: { projectID: string; enabled: boolean }) {
  const { accessToken } = useAuth();
  const [question, setQuestion] = useState("");
  const ask = useMutation({
    mutationFn: async (q: string) => {
      if (!accessToken) throw new Error("Not authenticated");
      return askProject(accessToken, projectID, q);
    },
  });
  const submit = () => {
    if (question.trim() && !ask.isPending) ask.mutate(question.trim());
  };
  const answer = ask.data;

  return (
    <section aria-labelledby="ask-heading" className="space-y-3">
      <h2 id="ask-heading" className="flex items-center gap-2 font-display text-xl font-medium">
        <Sparkles aria-hidden="true" className="h-4 w-4 text-muted-foreground" /> Ask this project
      </h2>
      <form
        className="surface-card space-y-3 p-4"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <Label htmlFor="ask-question" className="sr-only">
          Question
        </Label>
        <Textarea
          id="ask-question"
          rows={2}
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              submit();
            }
          }}
          disabled={!enabled}
          placeholder={enabled ? "What duty did we agree for pump P-03?" : "Needs an AI model configured on the server"}
        />
        <div className="flex items-center justify-between gap-2">
          <p className="text-xs text-muted-foreground">Answers cite the facts, decisions and mail they come from.</p>
          <Button type="submit" size="sm" disabled={!enabled || !question.trim() || ask.isPending}>
            {ask.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            Ask
          </Button>
        </div>
        {ask.isError && (
          <p role="alert" className="text-sm text-destructive">
            {ask.error instanceof ApiError ? ask.error.message : "Could not get an answer. Please try again."}
          </p>
        )}
        {answer && (
          <div aria-live="polite" className="space-y-1.5 border-t border-border/70 pt-3 text-sm">
            <p className="whitespace-pre-wrap">{answer.answer}</p>
            {answer.citations.length > 0 && (
              <p className="text-xs text-muted-foreground">
                Based on {answer.citations.length} {answer.citations.length === 1 ? "source" : "sources"} on this project.
              </p>
            )}
          </div>
        )}
      </form>
    </section>
  );
}
