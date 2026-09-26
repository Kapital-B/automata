import { Loader2 } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

/** Forwards the open message to an address on the forwarding allowlist. */
export function ForwardDialog({
  open,
  onOpenChange,
  loading,
  allowlist,
  to,
  onToChange,
  comment,
  onCommentChange,
  pending,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  loading: boolean;
  allowlist: string[];
  to: string;
  onToChange: (to: string) => void;
  comment: string;
  onCommentChange: (comment: string) => void;
  pending: boolean;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Forward message</DialogTitle>
          <DialogDescription>
            Only addresses on your forwarding allowlist can receive mail from Automata. Attachments go with it where the
            provider supports it.
          </DialogDescription>
        </DialogHeader>
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading allowlist…</p>
        ) : allowlist.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Add at least one address under{" "}
            <Link
              to="/rules"
              className="font-medium text-primary underline-offset-4 hover:underline"
              onClick={() => onOpenChange(false)}
            >
              Rules
            </Link>{" "}
            before forwarding.
          </p>
        ) : (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="forward-to-select">To</Label>
              <Select value={to} onValueChange={onToChange}>
                <SelectTrigger id="forward-to-select">
                  <SelectValue placeholder="Choose an address" />
                </SelectTrigger>
                <SelectContent>
                  {allowlist.map((email) => (
                    <SelectItem key={email} value={email}>
                      {email}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="forward-comment">
                Note <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Textarea
                id="forward-comment"
                value={comment}
                onChange={(e) => onCommentChange(e.target.value)}
                placeholder="Shown above the forwarded message"
                rows={3}
              />
            </div>
          </div>
        )}
        <DialogFooter className="gap-2 sm:gap-0">
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" disabled={pending || loading || allowlist.length === 0 || !to.trim()} onClick={onConfirm}>
            {pending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {pending ? "Forwarding…" : "Forward"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
