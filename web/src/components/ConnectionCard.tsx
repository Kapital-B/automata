import { useState, type ReactNode } from "react";
import { AlertTriangle, type LucideIcon } from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// Building blocks shared by every kind of connection on the Accounts page, so
// a mailbox and a Slack workspace read as the same kind of thing.

/** A section of the Accounts page: heading, one line of context, its add action. */
export function ConnectionSection({
  id,
  title,
  description,
  action,
  children,
}: {
  id: string;
  title: string;
  description: string;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section aria-labelledby={id} className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="space-y-1">
          <h2 id={id} className="font-display text-2xl font-medium">
            {title}
          </h2>
          <p className="max-w-2xl text-sm text-muted-foreground">{description}</p>
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

/**
 * The leading tile of a card. dotColor marks a mailbox with the colour that
 * identifies it everywhere else in the app.
 */
export function ConnectionIcon({ icon: Icon, dotColor }: { icon: LucideIcon; dotColor?: string }) {
  return (
    <span
      aria-hidden="true"
      className="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-border bg-secondary"
    >
      <Icon className="h-5 w-5" />
      {dotColor && (
        <span
          className="absolute -right-1 -top-1 h-3 w-3 rounded-full ring-2 ring-card"
          style={{ background: dotColor }}
        />
      )}
    </span>
  );
}

/** A small neutral label: provider, account kind, test workspace. */
export function Tag({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-full border border-border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
      {children}
    </span>
  );
}

export function StatusPill({ status }: { status: string }) {
  if (status === "connected") {
    return (
      <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 text-xs font-medium text-success">
        <span aria-hidden="true" className="h-1.5 w-1.5 rounded-full bg-success" /> Connected
      </span>
    );
  }
  const label = status.replace(/_/g, " ");
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-destructive/10 px-2.5 py-1 text-xs font-medium text-destructive">
      <AlertTriangle aria-hidden="true" className="h-3 w-3" />
      {label.charAt(0).toUpperCase() + label.slice(1)}
    </span>
  );
}

/**
 * The card every connection uses: identity and status on top, an optional
 * problem, the connection's own content, and its actions in a footer.
 */
export function ConnectionCard({
  icon,
  title,
  tags,
  meta,
  status,
  problem,
  highlighted,
  children,
  actions,
}: {
  icon: ReactNode;
  title: ReactNode;
  tags?: ReactNode;
  meta: ReactNode;
  status: string;
  problem?: ReactNode;
  highlighted?: boolean;
  children?: ReactNode;
  actions: ReactNode;
}) {
  return (
    <li className={cn("surface-card overflow-hidden", highlighted && "ring-2 ring-success/50 transition-shadow")}>
      <div className="flex flex-wrap items-start justify-between gap-4 p-5">
        <div className="flex min-w-0 items-start gap-3">
          {icon}
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="truncate font-display text-lg font-medium">{title}</h3>
              {tags}
            </div>
            <div className="mt-0.5 text-sm text-muted-foreground">{meta}</div>
          </div>
        </div>
        <StatusPill status={status} />
      </div>
      {problem && (
        <div
          role="alert"
          className="mx-5 mb-4 flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"
        >
          <AlertTriangle aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{problem}</span>
        </div>
      )}
      {children}
      <div className="flex flex-wrap items-center gap-2 border-t border-border/70 bg-muted/30 px-5 py-3">{actions}</div>
    </li>
  );
}

/** The body strip of a card: padded, separated from the header by a rule. */
export function ConnectionCardBody({ label, children }: { label?: string; children: ReactNode }) {
  return (
    <section aria-label={label} className="border-t border-border/70">
      {children}
    </section>
  );
}

export function ConnectionEmpty({
  icon,
  message,
  action,
}: {
  icon: ReactNode;
  message: string;
  action: ReactNode;
}) {
  return (
    <div className="surface-card flex flex-col items-start gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-3">
        {icon}
        <p className="text-sm text-muted-foreground">{message}</p>
      </div>
      {action}
    </div>
  );
}

export function ConnectionSkeleton({ label }: { label: string }) {
  return (
    <div aria-label={label} className="surface-card flex items-center gap-3 p-5">
      <Skeleton className="h-10 w-10 rounded-md" />
      <div className="space-y-2">
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-4 w-64" />
      </div>
    </div>
  );
}

export function ConnectionError({ children }: { children: ReactNode }) {
  return (
    <div role="alert" className="surface-card p-5 text-sm text-destructive">
      {children}
    </div>
  );
}

/**
 * A button that asks before it acts. Used for anything slow or destructive
 * on this page, so each asks the same way.
 */
export function ConfirmAction({
  trigger,
  title,
  description,
  confirmLabel,
  destructive,
  onConfirm,
}: {
  trigger: (open: () => void) => ReactNode;
  title: string;
  description: string;
  confirmLabel: string;
  destructive?: boolean;
  onConfirm: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {trigger(() => setOpen(true))}
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className={cn(destructive && "bg-destructive text-destructive-foreground hover:bg-destructive/90")}
              onClick={onConfirm}
            >
              {confirmLabel}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
