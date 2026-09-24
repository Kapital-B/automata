import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Loader2, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
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
import { cn } from "@/lib/utils";
import type { UiAccount } from "@/lib/accounts";
import {
  ApiError,
  createForwardRule,
  updateForwardRule,
  type CategoryDefinition,
  type ForwardField,
  type ForwardPredicate,
  type ForwardRule,
} from "@/lib/auth";
import {
  defaultPredicate,
  fieldLabels,
  fieldOps,
  isLogic,
  opLabels,
  predicateComplete,
} from "@/lib/forwardRules";

const selectClass =
  "h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export function RuleEditorDialog({
  open,
  onOpenChange,
  rule,
  accounts,
  defaultAccountID,
  allowlist,
  categories,
  aiAvailable,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The rule being edited; absent to create one. */
  rule?: ForwardRule;
  accounts: UiAccount[];
  defaultAccountID?: string;
  allowlist: string[];
  categories: CategoryDefinition[];
  aiAvailable: boolean;
}) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [accountID, setAccountID] = useState("");
  const [mode, setMode] = useState<"logic" | "llm">("logic");
  const [predicates, setPredicates] = useState<ForwardPredicate[]>([]);
  const [prompt, setPrompt] = useState("");
  const [forwardTo, setForwardTo] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Reset from the rule (or blank) each time the dialog opens.
  useEffect(() => {
    if (!open) return;
    setError(null);
    setName(rule?.name ?? "");
    setAccountID(rule?.account_id ?? defaultAccountID ?? accounts[0]?.id ?? "");
    setForwardTo(rule?.forward_to ?? allowlist[0] ?? "");
    if (rule && !isLogic(rule.condition_json)) {
      setMode("llm");
      setPrompt(rule.condition_json.prompt);
      setPredicates([defaultPredicate("category_slug", categories)]);
    } else {
      setMode("logic");
      setPrompt("");
      setPredicates(
        rule && isLogic(rule.condition_json) ? rule.condition_json.all : [defaultPredicate("category_slug", categories)],
      );
    }
    // Only on open: later prop changes must not wipe what the user typed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const account = accounts.find((a) => a.id === accountID);
  const resends = Boolean(account?.capabilities && !account.capabilities.server_side_forward);
  const ready =
    name.trim() !== "" &&
    accountID !== "" &&
    forwardTo !== "" &&
    (mode === "llm" ? prompt.trim() !== "" : predicates.length > 0 && predicates.every(predicateComplete));

  const save = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      const payload = {
        name: name.trim(),
        mode,
        condition_json: mode === "llm" ? { prompt: prompt.trim() } : { all: predicates },
        forward_to: forwardTo,
        enabled: rule?.enabled ?? false,
      };
      if (rule) return updateForwardRule(accessToken, rule.id, payload);
      return createForwardRule(accessToken, accountID, payload);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["forward-rules"] });
      toast({
        title: rule ? "Rule saved" : "Rule created, paused",
        description: rule ? undefined : "Switch it on when you are ready; you choose then whether it covers existing mail.",
      });
      onOpenChange(false);
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not save the rule. Please try again."),
  });

  const setPredicate = (i: number, next: ForwardPredicate) =>
    setPredicates((ps) => ps.map((p, j) => (j === i ? next : p)));

  const fieldChoices = useMemo(() => Object.keys(fieldLabels) as ForwardField[], []);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">{rule ? "Edit rule" : "New forwarding rule"}</DialogTitle>
          <DialogDescription>
            {rule
              ? "Changes apply to mail the rule has not already decided about."
              : "New rules start paused. Nothing is forwarded until you switch the rule on."}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (ready && !save.isPending) save.mutate();
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="rule-name">Name</Label>
              <Input id="rule-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Invoices to accounts" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rule-account">Mailbox</Label>
              <select
                id="rule-account"
                className={selectClass}
                value={accountID}
                disabled={Boolean(rule)}
                onChange={(e) => setAccountID(e.target.value)}
              >
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">Forward messages that…</legend>
            <div role="radiogroup" aria-label="How the rule decides" className="inline-flex rounded-md border border-border p-0.5">
              {(
                [
                  ["logic", "Match conditions"],
                  ["llm", "Describe it (AI)"],
                ] as const
              ).map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  role="radio"
                  aria-checked={mode === value}
                  disabled={value === "llm" && !aiAvailable && mode !== "llm"}
                  onClick={() => setMode(value)}
                  className={cn(
                    "rounded px-3 py-1.5 text-sm transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
                    mode === value ? "bg-secondary font-medium" : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {label}
                </button>
              ))}
            </div>
            {!aiAvailable && mode === "logic" && (
              <p className="text-xs text-muted-foreground">AI rules need a model configured on the server.</p>
            )}

            {mode === "logic" ? (
              <div className="space-y-2">
                <p className="text-xs text-muted-foreground">All of these must match.</p>
                {predicates.map((p, i) => (
                  <ConditionRow
                    key={i}
                    index={i}
                    predicate={p}
                    categories={categories}
                    fieldChoices={fieldChoices}
                    onChange={(next) => setPredicate(i, next)}
                    onRemove={predicates.length > 1 ? () => setPredicates((ps) => ps.filter((_, j) => j !== i)) : undefined}
                  />
                ))}
                {predicates.length < 10 && (
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    onClick={() => setPredicates((ps) => [...ps, defaultPredicate("subject", categories)])}
                  >
                    <Plus aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Add condition
                  </Button>
                )}
              </div>
            ) : (
              <div className="space-y-1.5">
                <Label htmlFor="rule-prompt">Which messages?</Label>
                <Textarea
                  id="rule-prompt"
                  value={prompt}
                  maxLength={1000}
                  onChange={(e) => setPrompt(e.target.value)}
                  placeholder="Supplier invoices and receipts that need paying"
                  rows={3}
                />
                <p className="text-xs text-muted-foreground">
                  The AI reads each message's subject, sender and body and decides. It costs a model call per message.
                </p>
              </div>
            )}
          </fieldset>

          <div className="space-y-1.5">
            <Label htmlFor="rule-forward-to">Forward to</Label>
            {allowlist.length === 0 ? (
              <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">
                Add an address to the allowlist below first; rules can only forward to allowlisted addresses.
              </p>
            ) : (
              <select id="rule-forward-to" className={selectClass} value={forwardTo} onChange={(e) => setForwardTo(e.target.value)}>
                {allowlist.map((email) => (
                  <option key={email} value={email}>
                    {email}
                  </option>
                ))}
              </select>
            )}
          </div>

          {resends && account && (
            <p
              role="note"
              className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-foreground/80"
            >
              <AlertTriangle aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {account.label} cannot forward server-side, so this rule sends a new message from {account.primaryEmail} with
              the original attached. It may look different to the recipient, and very large messages cannot be forwarded.
            </p>
          )}

          {error && (
            <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!ready || save.isPending}>
              {save.isPending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
              {rule ? "Save changes" : "Create rule"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ConditionRow({
  index,
  predicate: p,
  categories,
  fieldChoices,
  onChange,
  onRemove,
}: {
  index: number;
  predicate: ForwardPredicate;
  categories: CategoryDefinition[];
  fieldChoices: ForwardField[];
  onChange: (p: ForwardPredicate) => void;
  onRemove?: () => void;
}) {
  const n = index + 1;
  const ops = fieldOps[p.field];
  return (
    <div className="flex flex-wrap items-center gap-2">
      <select
        aria-label={`Condition ${n} field`}
        className={cn(selectClass, "w-36")}
        value={p.field}
        onChange={(e) => onChange(defaultPredicate(e.target.value as ForwardField, categories))}
      >
        {fieldChoices.map((f) => (
          <option key={f} value={f}>
            {fieldLabels[f]}
          </option>
        ))}
      </select>
      {ops.length > 1 ? (
        <select
          aria-label={`Condition ${n} match`}
          className={cn(selectClass, "w-40")}
          value={p.op}
          onChange={(e) => onChange({ ...p, op: e.target.value as ForwardPredicate["op"] })}
        >
          {ops.map((o) => (
            <option key={o} value={o}>
              {opLabels[p.field][o]}
            </option>
          ))}
        </select>
      ) : (
        opLabels[p.field][p.op] && <span className="px-1 text-sm text-muted-foreground">{opLabels[p.field][p.op]}</span>
      )}
      {p.field === "has_attachments" ? (
        <select
          aria-label={`Condition ${n} value`}
          className={cn(selectClass, "min-w-[10rem] flex-1")}
          value={p.value ? "yes" : "no"}
          onChange={(e) => onChange({ ...p, value: e.target.value === "yes" })}
        >
          <option value="yes">Has attachments</option>
          <option value="no">Has no attachments</option>
        </select>
      ) : p.field === "category_slug" ? (
        <select
          aria-label={`Condition ${n} value`}
          className={cn(selectClass, "min-w-[10rem] flex-1")}
          value={String(p.value)}
          onChange={(e) => onChange({ ...p, value: e.target.value })}
        >
          {categories.map((c) => (
            <option key={c.slug} value={c.slug}>
              {c.display_name}
            </option>
          ))}
        </select>
      ) : (
        <Input
          className="min-w-[10rem] flex-1"
          aria-label={`Condition ${n} value`}
          value={String(p.value)}
          onChange={(e) => onChange({ ...p, value: e.target.value })}
          placeholder={p.field === "from" && p.op === "domain" ? "vendor.com" : p.field === "from" ? "billing@vendor.com" : "invoice"}
        />
      )}
      {onRemove ? (
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Remove condition ${n}`}
          className="inline-flex h-10 w-10 items-center justify-center rounded-md text-muted-foreground hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <X aria-hidden="true" className="h-4 w-4" />
        </button>
      ) : null}
    </div>
  );
}
