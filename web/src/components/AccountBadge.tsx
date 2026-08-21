import type { UiAccount } from "@/lib/accounts";
import { cn } from "@/lib/utils";

interface AccountBadgeProps {
  account?: UiAccount;
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
  const email = (account.primaryEmail ?? "").trim();
  const label = (account.label ?? "").trim();
  const emailLower = email.toLowerCase();
  const labelLower = label.toLowerCase();
  const labelMatchesEmail = labelLower !== "" && emailLower !== "" && labelLower === emailLower;

  const primaryText = labelMatchesEmail ? email : label || email;

  return (
    <span
      className={cn(
        "inline-flex min-w-0 max-w-full items-center gap-1.5 font-medium text-foreground/80",
        size === "sm" ? "text-xs" : "text-sm",
        className
      )}
    >
      <span className="acct-dot shrink-0" style={{ background: dotColor }} />
      <span className="min-w-0 truncate">{primaryText || "Account"}</span>
      {showEmail && email && !labelMatchesEmail && label !== "" && (
        <span className="shrink-0 font-normal text-muted-foreground">· {email}</span>
      )}
    </span>
  );
}
