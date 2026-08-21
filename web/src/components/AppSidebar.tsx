import { NavLink, useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Sparkles,
  Inbox,
  Forward,
  PenLine,
  Plug,
  History,
  Settings as SettingsIcon,
  MessageSquare,
  Users,
  FolderKanban,
  CircleHelp,
  Mail,
  Slack,
  Zap,
  Server,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { getSummary, getUnassignedSummary, listDraftSuggestions } from "@/lib/auth";

const assistantNav = [
  { title: "Assistant", url: "/", icon: MessageSquare, end: true },
];

const primaryNav = [
  { title: "Today", url: "/today", icon: Sparkles },
  { title: "Inbox", url: "/inbox", icon: Inbox },
  { title: "Projects", url: "/projects", icon: FolderKanban },
  { title: "Unassigned", url: "/unassigned", icon: CircleHelp },
  { title: "People", url: "/people", icon: Users },
  { title: "Drafts", url: "/drafts", icon: PenLine },
  { title: "Rules", url: "/rules", icon: Forward },
];

const connectorsNav = [
  { title: "Email", url: "/accounts", icon: Mail, status: "live" as const },
  { title: "Slack", url: "/accounts", icon: Slack, status: "soon" as const },
  { title: "Linear", url: "/accounts", icon: Zap, status: "soon" as const },
  { title: "MCP servers", url: "/accounts", icon: Server, status: "soon" as const },
];

const systemNav = [
  { title: "Accounts", url: "/accounts", icon: Plug },
  { title: "Runs", url: "/runs", icon: History },
  { title: "Settings", url: "/settings", icon: SettingsIcon },
];

export function AppSidebar() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";
  const location = useLocation();
  const { user, signOut, accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const connectedAccounts = accounts.filter((a) => a.status === "connected");

  const summaryBadgeQuery = useQuery({
    queryKey: ["sidebar", "summary-badge", accessToken],
    queryFn: () => getSummary(accessToken!),
    enabled: Boolean(accessToken) && connectedAccounts.length > 0,
  });
  const draftsBadgeQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, "all"],
    queryFn: () => listDraftSuggestions(accessToken!),
    enabled: Boolean(accessToken) && connectedAccounts.length > 0,
  });
  const unassignedBadgeQuery = useQuery({
    queryKey: ["unassigned-summary", accessToken],
    queryFn: () => getUnassignedSummary(accessToken!),
    enabled: Boolean(accessToken),
  });
  const todayBadge = summaryBadgeQuery.data?.action_items?.length ?? 0;
  const draftsBadge = draftsBadgeQuery.data?.length ?? 0;
  const unassignedBadge =
    (unassignedBadgeQuery.data?.unassigned ?? 0) +
    (unassignedBadgeQuery.data?.provisional ?? 0);

  const isActive = (url: string, end?: boolean) =>
    end ? location.pathname === url : location.pathname.startsWith(url) && url !== "/";

  const linkClass = (active: boolean) =>
    cn(
      "flex items-center gap-3 rounded-md px-2 py-1.5 text-sm transition-colors",
      active
        ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
        : "text-sidebar-foreground/80 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground"
    );

  return (
    <Sidebar collapsible="icon" className="border-r border-sidebar-border">
      <SidebarHeader className="px-3 pt-4 pb-2">
        <div className="flex items-center gap-2">
          <div className="h-full w-9 overflow-hidden rounded-md">
            <img
              src="/logo.png"
              alt="Automata"
              className="h-full w-auto object-contain"
            />
          </div>
          {!collapsed && (
            <div className="leading-tight">
              <p className="font-display text-base font-semibold">Automata</p>
              <p className="text-[10px] uppercase tracking-widest text-muted-foreground">
                workspace intelligence
              </p>
            </div>
          )}
        </div>
      </SidebarHeader>

      <SidebarContent className="px-2">
        <SidebarGroup>
          {!collapsed && (
            <SidebarGroupLabel className="px-2 text-[10px] uppercase tracking-widest text-muted-foreground">
              Assistant
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {assistantNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url} end={item.end}>
                      {({ isActive: a }) => (
                        <span className={linkClass(a)}>
                          <item.icon className="h-4 w-4 shrink-0" />
                          {!collapsed && <span>{item.title}</span>}
                        </span>
                      )}
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          {!collapsed && (
            <SidebarGroupLabel className="px-2 text-[10px] uppercase tracking-widest text-muted-foreground">
              Workspace
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {primaryNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url}>
                      {({ isActive: a }) => (
                        <span className={linkClass(a)}>
                          <item.icon className="h-4 w-4 shrink-0" />
                          {!collapsed && (
                            <>
                              <span className="flex-1">{item.title}</span>
                              {item.title === "Today" && (
                                <span className="rounded-full border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium leading-none">
                                  {todayBadge}
                                </span>
                              )}
                              {item.title === "Drafts" && (
                                <span className="rounded-full border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium leading-none">
                                  {draftsBadge}
                                </span>
                              )}
                              {item.title === "Unassigned" && unassignedBadge > 0 && (
                                <span className="rounded-full border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium leading-none">
                                  {unassignedBadge}
                                </span>
                              )}
                            </>
                          )}
                        </span>
                      )}
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          {!collapsed && (
            <SidebarGroupLabel className="px-2 text-[10px] uppercase tracking-widest text-muted-foreground">
              Connectors
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {connectorsNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url}>
                      <span className={linkClass(false)}>
                        <item.icon className="h-4 w-4 shrink-0" />
                        {!collapsed && (
                          <>
                            <span className="flex-1">{item.title}</span>
                            {item.status === "soon" && (
                              <span className="text-[9px] uppercase tracking-wider text-muted-foreground">
                                soon
                              </span>
                            )}
                          </>
                        )}
                      </span>
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        <SidebarGroup>
          {!collapsed && (
            <SidebarGroupLabel className="px-2 text-[10px] uppercase tracking-widest text-muted-foreground">
              System
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {systemNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url}>
                      {({ isActive: a }) => (
                        <span className={linkClass(a || isActive(item.url))}>
                          <item.icon className="h-4 w-4 shrink-0" />
                          {!collapsed && <span>{item.title}</span>}
                        </span>
                      )}
                    </NavLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {!collapsed && (
          <SidebarGroup>
            <SidebarGroupLabel className="px-2 text-[10px] uppercase tracking-widest text-muted-foreground">
              Connected
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <ul className="space-y-1 px-2">
                {accounts.map((a) => (
                  <li
                    key={a.id}
                    className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-sidebar-foreground/80"
                  >
                    <span
                      className="acct-dot"
                      style={{ background: `hsl(var(--${a.colorVar}))` }}
                    />
                    <span className="flex-1 truncate">{a.label}</span>
                    {a.status !== "connected" && (
                      <span className="text-[10px] uppercase tracking-wider text-destructive">
                        {a.status}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </SidebarGroupContent>
          </SidebarGroup>
        )}
      </SidebarContent>

      <SidebarFooter className="px-3 py-3 space-y-2">
        {!collapsed && (
          <>
            <div className="rounded-md border border-sidebar-border px-2 py-2">
              <p className="truncate text-xs font-medium text-sidebar-foreground">
                {user?.email ?? "Signed in"}
              </p>
              <button
                type="button"
                onClick={signOut}
                className="mt-1 text-xs text-sidebar-foreground/70 transition-colors hover:text-sidebar-foreground"
              >
                Sign out
              </button>
            </div>
            <p className="text-[10px] text-muted-foreground text-center">
              Local LLM · LM Studio
            </p>
          </>
        )}
        {collapsed && (
          <button
            type="button"
            onClick={signOut}
            className="rounded-md border border-sidebar-border px-2 py-1.5 text-center text-xs font-medium text-sidebar-foreground hover:bg-sidebar-accent transition-colors"
            title={`Sign out${user?.email ? ` ${user.email}` : ""}`}
          >
            Out
          </button>
        )}
      </SidebarFooter>
    </Sidebar>
  );
}
