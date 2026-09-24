import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { relativeTime } from "@/lib/accounts";
import { cn } from "@/lib/utils";
import {
  ApiError,
  previewForwardRule,
  updateForwardRule,
  type ForwardRule,
  type ForwardStart,
} from "@/lib/auth";

/**
 * Switching a rule on is a decision about existing mail, so it is asked,
 * with numbers, rather than implied: the first run used to forward every
 * matching message ever synced.
 */
export function StartRuleDialog({
  rule,
  onOpenChange,
}: {
  rule: ForwardRule | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [start, setStart] = useState<ForwardStart>("new");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (rule) {
      setStart("new");
      setError(null);
    }
  }, [rule]);

  const preview = useQuery({
    queryKey: ["forward-preview", accessToken, rule?.id, rule?.condition_json],
    enabled: Boolean(accessToken && rule && start === "existing"),
    queryFn: () =>
      previewForwardRule(accessToken!, rule!.account_id, { mode: rule!.mode, condition_json: rule!.condition_json }),
    staleTime: 30_000,
  });

  const enable = useMutation({
    mutationFn: async () => {
      if (!accessToken || !rule) throw new Error("Not authenticated");
      return updateForwardRule(accessToken, rule.id, {
        name: rule.name,
        mode: rule.mode,
        condition_json: rule.condition_json,
        forward_to: rule.forward_to,
        enabled: true,
        start,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["forward-rules"] });
      toast({
        title: "Rule switched on",
        description: "It forwards when rules next run: from Run now, or a schedule that includes forward_rules.",
      });
      onOpenChange(false);
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not switch the rule on."),
  });

  const p = preview.data;
  return (
    <Dialog open={rule !== null} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Switch on “{rule?.name}”</DialogTitle>
          <DialogDescription>Which mail should this rule look at?</DialogDescription>
        </DialogHeader>
        <div role="radiogroup" aria-label="Which mail" className="space-y-2">
          {(
            [
              ["new", "New mail only", "Mail that arrives from now on. Nothing already in the mailbox is forwarded."],
              ["existing", "Existing mail too", "Also every message already synced. Check the count below first."],
            ] as const
          ).map(([value, title, blurb]) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={start === value}
              onClick={() => setStart(value)}
              className={cn(
                "w-full rounded-lg border p-3 text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                start === value ? "border-foreground bg-secondary" : "border-border hover:border-foreground/40",
              )}
            >
              <p className="text-sm font-medium">{title}</p>
              <p className="mt-0.5 text-xs text-muted-foreground">{blurb}</p>
            </button>
          ))}
        </div>

        {start === "existing" && (
          <div aria-live="polite" className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
            {preview.isLoading ? (
              <span className="inline-flex items-center gap-2 text-muted-foreground">
                <Loader2 aria-hidden="true" className="h-3.5 w-3.5 animate-spin" /> Counting matching mail…
              </span>
            ) : preview.isError ? (
              <span className="text-destructive">Could not count existing mail.</span>
            ) : p ? (
              <div className="space-y-2">
                <p>
                  {p.matched !== undefined ? (
                    <>
                      <strong>{p.matched}</strong> of {p.in_scope}
                      {p.capped ? "+" : ""} existing messages match and would be forwarded.
                    </>
                  ) : (
                    <>
                      The AI would check <strong>{p.in_scope}</strong>
                      {p.capped ? "+" : ""} existing messages, one model call each.
                    </>
                  )}
                </p>
                {p.samples.length > 0 && (
                  <ul className="space-y-1 text-xs text-muted-foreground">
                    {p.samples.map((m, i) => (
                      <li key={i} className="truncate">
                        {m.subject || "(no subject)"} · {m.from_name || m.from_address} · {relativeTime(m.received_at)}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            ) : null}
          </div>
        )}

        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => enable.mutate()}
            disabled={enable.isPending || (start === "existing" && preview.isLoading)}
          >
            {enable.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            Switch on
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
