import { Account } from "@/lib/mock-data";
import { cn } from "@/lib/utils";

interface AccountBadgeProps {
  account?: Account;
  size?: "sm" | "md";
  showEmail?: boolean;
  className?: string;
}

/**
 * AccountBadge — provenance chip used everywhere a row, summary line,
 * draft, or run could come from one of multiple connected accounts.
 * Per the spec: account identity must be visible on every list/detail view.
 */
export function AccountBadge({ account, size = "sm", showEmail = false, className }: AccountBadgeProps) {
  if (!account) {
    return (
      <span className={cn("inline-flex items-center gap-1.5 text-muted-foreground text-xs", className)}>
        <span className="acct-dot bg-muted-foreground/40" />
        all accounts
      </span>
    );
  }
  const dotColor = `hsl(var(--${account.colorVar}))`;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 font-medium text-foreground/80",
        size === "sm" ? "text-xs" : "text-sm",
        className
      )}
    >
      <span className="acct-dot" style={{ background: dotColor }} />
      <span>{account.label}</span>
      {showEmail && (
        <span className="text-muted-foreground font-normal">· {account.primaryEmail}</span>
      )}
    </span>
  );
}
