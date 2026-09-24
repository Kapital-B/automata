import { Info, KeyRound, Loader2, Mail, RefreshCw, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  ConfirmAction,
  ConnectionCard,
  ConnectionCardBody,
  ConnectionIcon,
  Tag,
} from "@/components/ConnectionCard";
import { capabilityNotes, providerLabel, relativeTime, type UiAccount } from "@/lib/accounts";

export function MailboxCard({
  account: a,
  highlighted,
  syncing,
  busy,
  onSync,
  onReconnect,
  onDisconnect,
}: {
  account: UiAccount;
  highlighted: boolean;
  /** This card's sync is in flight. */
  syncing: boolean;
  /** Something on the page is mid-change; hold off on actions. */
  busy: boolean;
  onSync: (force?: boolean) => void;
  onReconnect: () => void;
  onDisconnect: () => void;
}) {
  const notes = capabilityNotes(a.capabilities);
  const problem =
    a.status === "connected"
      ? undefined
      : (a.lastError ??
        (a.status === "expired"
          ? "Sign-in expired. Reconnect to resume sync and forwarding."
          : "The last sync failed."));

  return (
    <ConnectionCard
      icon={<ConnectionIcon icon={Mail} dotColor={`hsl(var(--${a.colorVar}))`} />}
      title={a.label}
      tags={
        <>
          <Tag>{providerLabel(a.provider)}</Tag>
          {a.provider === "m365" && (a.kind === "work" || a.kind === "personal") && <Tag>{a.kind}</Tag>}
        </>
      }
      meta={
        <>
          {a.label !== a.primaryEmail && <span className="break-all">{a.primaryEmail} · </span>}
          Last sync {relativeTime(a.lastSyncedAt)}
        </>
      }
      status={a.status}
      problem={problem}
      highlighted={highlighted}
      actions={
        <>
          {a.status === "expired" ? (
            <Button size="sm" variant="outline" onClick={onReconnect} disabled={busy}>
              <KeyRound aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Reconnect
            </Button>
          ) : (
            <Button size="sm" variant="outline" onClick={() => onSync()} disabled={busy || syncing}>
              {syncing ? (
                <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
              )}
              {a.status === "connected" ? "Sync now" : "Retry sync"}
            </Button>
          )}
          {a.status === "connected" && (
            <ConfirmAction
              title={`Refetch all of ${a.label}?`}
              description="An ordinary sync collects only what changed since the last one. A full resync fetches every message again, which repairs messages missing their sender, subject or body. It takes longer."
              confirmLabel="Full resync"
              onConfirm={() => onSync(true)}
              trigger={(open) => (
                <Button size="sm" variant="ghost" className="text-muted-foreground" onClick={open} disabled={busy || syncing}>
                  Full resync
                </Button>
              )}
            />
          )}
          <ConfirmAction
            title={`Disconnect ${a.label}?`}
            description="This deletes everything synced from this mailbox — messages, summaries, drafts and forwarding rules — along with its stored credentials. Connecting it again starts from a fresh sync."
            confirmLabel="Disconnect"
            destructive
            onConfirm={onDisconnect}
            trigger={(open) => (
              <Button
                size="sm"
                variant="ghost"
                className="ml-auto text-muted-foreground hover:text-destructive"
                onClick={open}
                disabled={busy}
              >
                <Unplug aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" /> Disconnect
              </Button>
            )}
          />
        </>
      }
    >
      {notes.length > 0 && (
        <ConnectionCardBody label={`What ${a.label} can do`}>
          <ul className="space-y-1.5 px-5 py-3">
            {notes.map((note) => (
              <li key={note} className="flex items-start gap-2 text-sm text-muted-foreground">
                <Info aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0" /> {note}
              </li>
            ))}
          </ul>
        </ConnectionCardBody>
      )}
    </ConnectionCard>
  );
}
