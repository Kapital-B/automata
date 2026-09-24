import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2, ShieldCheck, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ConfirmAction } from "@/components/ConnectionCard";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import { ApiError, putForwardAllowlist } from "@/lib/auth";
import { looksLikeEmail } from "@/lib/forwardRules";

export function AllowlistSection({
  allowlist,
  rulesByAddress,
}: {
  allowlist: string[];
  /** Names of the rules that forward to each address. */
  rulesByAddress: Record<string, string[]>;
}) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: async (emails: string[]) => {
      if (!accessToken) throw new Error("Not authenticated");
      await putForwardAllowlist(accessToken, emails);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["forward-allowlist"] });
      void queryClient.invalidateQueries({ queryKey: ["forward-rules"] });
    },
    onError: (e) =>
      toast({
        title: "Could not update the allowlist",
        description: e instanceof ApiError ? e.message : "Please try again.",
        variant: "destructive",
      }),
  });

  const add = () => {
    const email = draft.trim().toLowerCase();
    if (!email) return;
    if (!looksLikeEmail(email)) {
      setError(`“${draft.trim()}” is not an email address.`);
      return;
    }
    if (allowlist.includes(email)) {
      setError("That address is already on the allowlist.");
      return;
    }
    setError(null);
    save.mutate([...allowlist, email], { onSuccess: () => setDraft("") });
  };

  return (
    <section aria-labelledby="allowlist-heading" className="space-y-4">
      <div className="space-y-1">
        <h2 id="allowlist-heading" className="font-display text-2xl font-medium">
          Allowlist
        </h2>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Rules can only forward to these addresses, so a rule can never send mail somewhere you have not approved.
        </p>
      </div>
      <form
        noValidate
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          add();
        }}
      >
        <div className="min-w-[220px] flex-1 space-y-1.5">
          <Label htmlFor="allowlist-email">Add an address</Label>
          <Input
            id="allowlist-email"
            type="email"
            value={draft}
            onChange={(e) => {
              setDraft(e.target.value);
              setError(null);
            }}
            placeholder="accounting@example.com"
            aria-invalid={Boolean(error)}
            aria-describedby={error ? "allowlist-error" : undefined}
          />
        </div>
        <Button type="submit" size="sm" variant="outline" disabled={save.isPending || !draft.trim()}>
          {save.isPending ? (
            <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
          ) : (
            <ShieldCheck aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
          )}
          Add address
        </Button>
      </form>
      {error && (
        <p id="allowlist-error" role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {allowlist.length === 0 ? (
        <p className="surface-card p-5 text-sm text-muted-foreground">No addresses yet. Add one before creating a rule.</p>
      ) : (
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {allowlist.map((email) => {
            const users = rulesByAddress[email] ?? [];
            const removeButton = (open: () => void) => (
              <button
                type="button"
                onClick={open}
                disabled={save.isPending}
                aria-label={`Remove ${email}`}
                className="inline-flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <X aria-hidden="true" className="h-4 w-4" />
              </button>
            );
            const remove = () => save.mutate(allowlist.filter((v) => v !== email));
            return (
              <li key={email} className="flex items-center justify-between gap-3 px-4 py-2">
                <div className="min-w-0">
                  <p className="truncate font-mono text-sm">{email}</p>
                  {users.length > 0 && (
                    <p className="truncate text-xs text-muted-foreground">Used by {users.join(", ")}</p>
                  )}
                </div>
                {users.length > 0 ? (
                  <ConfirmAction
                    title={`Remove ${email}?`}
                    description={`${users.join(", ")} ${users.length === 1 ? "forwards" : "forward"} here and will stop running until you pick another address or add this one back.`}
                    confirmLabel="Remove address"
                    destructive
                    onConfirm={remove}
                    trigger={removeButton}
                  />
                ) : (
                  removeButton(remove)
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
