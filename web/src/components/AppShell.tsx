import { ReactNode, useState } from "react";
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/AppSidebar";
import { accounts } from "@/lib/mock-data";
import { useAuth } from "@/components/auth/AuthProvider";
import { ChevronDown, Search } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

export type AccountFilter = "all" | string;

interface AppShellProps {
  children: (ctx: { accountFilter: AccountFilter }) => ReactNode;
}

/**
 * AppShell — global app chrome:
 * - Persistent sidebar (collapsible)
 * - Top bar with global account switcher (provenance-first)
 * - Always-visible SidebarTrigger
 *
 * Account filter is hoisted here so any page can react to it via
 * the render-prop ctx.
 */
export function AppShell({ children }: AppShellProps) {
  const [accountFilter, setAccountFilter] = useState<AccountFilter>("all");
  const { user } = useAuth();
  const current = accounts.find((a) => a.id === accountFilter);
  const initials =
    user?.email
      .split("@")[0]
      .split(/[._-]/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase())
      .join("") || "P";

  return (
    <SidebarProvider>
      <div className="flex min-h-screen w-full bg-background">
        <AppSidebar />
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border/70 bg-background/85 px-4 backdrop-blur">
            <SidebarTrigger className="text-muted-foreground hover:text-foreground" />
            <div className="hidden h-5 w-px bg-border md:block" />

            <DropdownMenu>
              <DropdownMenuTrigger className="group flex items-center gap-2 rounded-md border border-border bg-card px-2.5 py-1.5 text-sm shadow-sm transition hover:border-foreground/30">
                {current ? (
                  <>
                    <span
                      className="acct-dot"
                      style={{ background: `hsl(var(--${current.colorVar}))` }}
                    />
                    <span className="font-medium">{current.label}</span>
                    <span className="hidden text-muted-foreground sm:inline">
                      · {current.primaryEmail}
                    </span>
                  </>
                ) : (
                  <>
                    <span className="acct-dot bg-foreground/40" />
                    <span className="font-medium">All accounts</span>
                    <span className="hidden text-muted-foreground sm:inline">
                      · {accounts.length} connected
                    </span>
                  </>
                )}
                <ChevronDown className="h-3.5 w-3.5 text-muted-foreground transition group-data-[state=open]:rotate-180" />
              </DropdownMenuTrigger>
              <DropdownMenuContent align="start" className="w-72">
                <DropdownMenuLabel className="text-[10px] uppercase tracking-widest text-muted-foreground">
                  Filter by account
                </DropdownMenuLabel>
                <DropdownMenuItem
                  onClick={() => setAccountFilter("all")}
                  className={cn(
                    "flex items-center gap-2",
                    accountFilter === "all" && "bg-secondary"
                  )}
                >
                  <span className="acct-dot bg-foreground/40" />
                  <span className="flex-1">All accounts</span>
                  <span className="text-xs text-muted-foreground">
                    {accounts.length}
                  </span>
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                {accounts.map((a) => (
                  <DropdownMenuItem
                    key={a.id}
                    onClick={() => setAccountFilter(a.id)}
                    className={cn(
                      "flex items-center gap-2",
                      accountFilter === a.id && "bg-secondary"
                    )}
                  >
                    <span
                      className="acct-dot"
                      style={{ background: `hsl(var(--${a.colorVar}))` }}
                    />
                    <div className="flex flex-1 flex-col">
                      <span className="text-sm">{a.label}</span>
                      <span className="text-[11px] text-muted-foreground">
                        {a.primaryEmail}
                      </span>
                    </div>
                    <span
                      className={cn(
                        "text-[10px] uppercase tracking-wider",
                        a.status === "connected"
                          ? "text-success"
                          : "text-destructive"
                      )}
                    >
                      {a.status}
                    </span>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>

            <div className="ml-auto flex items-center gap-2">
              <div className="relative hidden md:block">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <input
                  type="search"
                  placeholder="Search mail, drafts, rules…"
                  className="w-72 rounded-md border border-border bg-card py-1.5 pl-8 pr-3 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
              <div
                className="hidden h-8 w-8 place-items-center rounded-full md:grid"
                style={{ background: "var(--gradient-ink)" }}
                title={user?.email ?? "Signed in"}
              >
                <span className="text-xs font-semibold text-primary-foreground">{initials}</span>
              </div>
            </div>
          </header>
          <main className="min-w-0 flex-1 px-4 py-6 md:px-8 md:py-10">
            <div className="mx-auto max-w-6xl">{children({ accountFilter })}</div>
          </main>
        </div>
      </div>
    </SidebarProvider>
  );
}
