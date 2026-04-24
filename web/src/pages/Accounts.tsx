import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import { accounts, relativeTime } from "@/lib/mock-data";
import { Plus, RefreshCw, Unplug, AlertTriangle } from "lucide-react";
import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";

export default function AccountsPage() {
  const [kind, setKind] = useState<"work" | "personal" | null>(null);

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Settings · Accounts"
        title="Connected mailboxes"
        description="Each connected account is treated as its own source. Every summary, draft, rule, and run is tagged with the account it came from."
        actions={
          <Dialog>
            <DialogTrigger asChild>
              <Button size="sm" className="bg-foreground text-background hover:bg-foreground/90">
                <Plus className="mr-1.5 h-3.5 w-3.5" /> Add account
              </Button>
            </DialogTrigger>
            <DialogContent className="max-w-md">
              <DialogHeader>
                <DialogTitle className="font-display text-xl">Connect a Microsoft mailbox</DialogTitle>
                <DialogDescription>
                  We'll redirect you to Microsoft to sign in and grant Mail.Read,
                  Mail.Send, and offline access. Your tokens are stored encrypted, per account.
                </DialogDescription>
              </DialogHeader>
              <div className="grid grid-cols-2 gap-3 pt-2">
                {(["work", "personal"] as const).map((k) => (
                  <button
                    key={k}
                    onClick={() => setKind(k)}
                    className={cn(
                      "rounded-lg border p-4 text-left transition",
                      kind === k
                        ? "border-foreground bg-secondary"
                        : "border-border hover:border-foreground/40"
                    )}
                  >
                    <p className="font-display text-base font-medium capitalize">{k}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {k === "work"
                        ? "Microsoft 365 / Entra (organizations)"
                        : "Outlook.com, Hotmail, Live (consumers)"}
                    </p>
                  </button>
                ))}
              </div>
              <Button
                disabled={!kind}
                className="mt-2 w-full bg-foreground text-background hover:bg-foreground/90"
              >
                Continue to Microsoft sign-in
              </Button>
            </DialogContent>
          </Dialog>
        }
      />

      <ul className="space-y-3">
        {accounts.map((a) => (
          <li key={a.id} className="surface-card p-5">
            <div className="flex items-start justify-between gap-4">
              <div className="flex items-start gap-3">
                <span
                  className="mt-1.5 acct-dot h-3 w-3"
                  style={{ background: `hsl(var(--${a.colorVar}))` }}
                />
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="font-display text-lg font-medium">{a.label}</h3>
                    <span
                      className={cn(
                        "rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider",
                        a.kind === "work"
                          ? "border-[hsl(184_55%_22%/0.3)] bg-[hsl(184_55%_22%/0.08)] text-[hsl(184_55%_22%)]"
                          : "border-[hsl(28_70%_52%/0.3)] bg-[hsl(28_70%_52%/0.10)] text-[hsl(28_70%_38%)]"
                      )}
                    >
                      {a.kind}
                    </span>
                  </div>
                  <p className="mt-0.5 text-sm text-muted-foreground">{a.primaryEmail}</p>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Last sync {relativeTime(a.lastSyncedAt)}
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {a.status === "connected" ? (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 text-xs font-medium text-success">
                    <span className="h-1.5 w-1.5 rounded-full bg-success" /> Connected
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
                    <AlertTriangle className="h-3 w-3" /> {a.status}
                  </span>
                )}
              </div>
            </div>
            {a.status !== "connected" && (
              <div className="mt-4 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                Session expired. Sign in again to resume sync and forwarding.
              </div>
            )}
            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Button size="sm" variant="outline">
                <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                {a.status === "connected" ? "Sync now" : "Reconnect"}
              </Button>
              <Button size="sm" variant="ghost" className="text-muted-foreground hover:text-destructive">
                <Unplug className="mr-1.5 h-3.5 w-3.5" /> Disconnect
              </Button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
