import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { relativeTime } from "@/lib/accounts";

/**
 * Replaces the Interpret and Reconcile buttons (spec §8.1). Extraction runs on
 * its own now, so the page reports when it last ran rather than offering its
 * stages as verbs. "Check now" exists for impatience, not because the operator
 * is expected to drive the pipeline.
 */
export function ExtractionStatus({
  lastExtractedAt,
  reviewing,
  stalled,
  pending,
  onCheckNow,
}: {
  lastExtractedAt?: string;
  reviewing: boolean;
  stalled: boolean;
  pending: boolean;
  onCheckNow: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
      {reviewing ? (
        <span className="flex items-center gap-2">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          Reviewing…
        </span>
      ) : stalled ? (
        // A queued run that never moved the watermark. Saying so beats
        // reverting to the old one, which reads as "nothing is happening"
        // when in fact something is wedged.
        <span className="text-destructive">
          Last check didn&rsquo;t finish — the run may be stuck.
        </span>
      ) : (
        <span>
          {lastExtractedAt
            ? `Reviewed ${relativeTime(lastExtractedAt)}`
            : "Not reviewed yet"}
        </span>
      )}
      <Button
        variant="outline"
        size="sm"
        className="h-8 text-xs"
        disabled={pending || reviewing}
        onClick={onCheckNow}
      >
        Check now
      </Button>
    </div>
  );
}
