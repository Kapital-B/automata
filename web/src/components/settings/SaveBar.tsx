import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * Shown while a tab has unsaved edits: says so, and keeps Save and Discard in
 * reach at the bottom of the screen however long the tab is.
 */
export function SaveBar({
  dirty,
  saving,
  onSave,
  onDiscard,
  label = "You have unsaved changes",
  canSave = true,
}: {
  dirty: boolean;
  saving: boolean;
  onSave: () => void;
  onDiscard: () => void;
  label?: string;
  canSave?: boolean;
}) {
  if (!dirty) return null;
  return (
    <div
      role="region"
      aria-label="Unsaved changes"
      className="sticky bottom-4 z-10 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3 shadow-lg"
    >
      <p className="text-sm font-medium">{label}</p>
      <div className="flex gap-2">
        <Button type="button" size="sm" variant="ghost" onClick={onDiscard} disabled={saving}>
          Discard
        </Button>
        <Button type="button" size="sm" onClick={onSave} disabled={saving || !canSave}>
          {saving && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
          Save changes
        </Button>
      </div>
    </div>
  );
}
