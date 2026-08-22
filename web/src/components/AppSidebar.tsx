import { NavLink, useLocation } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  Inbox,
  Forward,
  PenLine,
  Plug,
  History,
  Settings as SettingsIcon,
  Home,
  Users,
  FolderKanban,
  CircleHelp,
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
import { getUnassignedSummary, listDraftSuggestions, getAttention } from "@/lib/auth";

const primaryNav = [
  { title: "Home", url: "/", icon: Home, end: true, badge: "needsMe" as const },
  { title: "Projects", url: "/projects", icon: FolderKanban },
  { title: "Triage", url: "/triage", icon: CircleHelp, badge: "triage" as const },
  { title: "People", url: "/people", icon: Users },
];

const moreNav = [
  { title: "Inbox", url: "/inbox", icon: Inbox },
  { title: "Drafts", url: "/drafts", icon: PenLine, badge: "drafts" as const },
  { title: "Rules", url: "/rules", icon: Forward },
  { title: "Connectors", url: "/accounts", icon: Plug },
  { title: "Runs", url: "/runs", icon: History },
  { title: "Settings", url: "/settings", icon: SettingsIcon },
];

export function AppSidebar() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";
  const location = useLocation();
  const { user, signOut, accessToken } = useAuth();
  const { accounts } = useAccountsData();

  const draftsBadgeQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, "all"],
    queryFn: () => listDraftSuggestions(accessToken!),
    enabled: Boolean(accessToken) && accounts.some((a) => a.status === "connected"),
  });
  const unassignedBadgeQuery = useQuery({
    queryKey: ["unassigned-summary", accessToken],
    queryFn: () => getUnassignedSummary(accessToken!),
    enabled: Boolean(accessToken),
  });
  const attentionBadgeQuery = useQuery({
    queryKey: ["attention", accessToken],
    queryFn: () => getAttention(accessToken!),
    enabled: Boolean(accessToken),
  });
  const draftsBadge = draftsBadgeQuery.data?.length ?? 0;
  const triageBadge =
    (unassignedBadgeQuery.data?.unassigned ?? 0) +
    (unassignedBadgeQuery.data?.provisional ?? 0);
  const needsMeBadge = attentionBadgeQuery.data?.counts?.total ?? 0;

  const isActive = (url: string, end?: boolean) =>
    end ? location.pathname === url : location.pathname.startsWith(url) && url !== "/";

  const linkClass = (active: boolean) =>
    cn(
      "flex items-center gap-3 rounded-md px-2 py-1.5 text-sm transition-colors",
      active
        ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
        : "text-sidebar-foreground/80 hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
    );

  const badgeFor = (badge?: "needsMe" | "triage" | "drafts") => {
    if (badge === "needsMe" && needsMeBadge > 0) return needsMeBadge;
    if (badge === "triage" && triageBadge > 0) return triageBadge;
    if (badge === "drafts" && draftsBadge > 0) return draftsBadge;
    return null;
  };

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
              Primary
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {primaryNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url} end={item.end}>
                      {({ isActive: a }) => {
                        const count = badgeFor(item.badge);
                        return (
                          <span className={linkClass(a || isActive(item.url, item.end))}>
                            <item.icon className="h-4 w-4 shrink-0" />
                            {!collapsed && (
                              <>
                                <span className="flex-1">{item.title}</span>
                                {count != null && (
                                  <span
                                    className="rounded-full border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium leading-none"
                                    title={
                                      item.badge === "needsMe"
                                        ? "Needs my input"
                                        : item.badge === "triage"
                                          ? "Triage queue"
                                          : undefined
                                    }
                                  >
                                    {count}
                                  </span>
                                )}
                              </>
                            )}
                          </span>
                        );
                      }}
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
              More
            </SidebarGroupLabel>
          )}
          <SidebarGroupContent>
            <SidebarMenu>
              {moreNav.map((item) => (
                <SidebarMenuItem key={item.title}>
                  <SidebarMenuButton asChild>
                    <NavLink to={item.url}>
                      {({ isActive: a }) => {
                        const count = badgeFor(item.badge);
                        return (
                          <span className={linkClass(a || isActive(item.url))}>
                            <item.icon className="h-4 w-4 shrink-0" />
                            {!collapsed && (
                              <>
                                <span className="flex-1">{item.title}</span>
                                {count != null && (
                                  <span className="rounded-full border border-sidebar-border px-1.5 py-0.5 text-[10px] font-medium leading-none">
                                    {count}
                                  </span>
                                )}
                              </>
                            )}
                          </span>
                        );
                      }}
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
                {accounts.length === 0 && (
                  <li className="px-2 py-1.5 text-xs text-muted-foreground">
                    <NavLink to="/accounts" className="underline-offset-2 hover:underline">
                      Connect email
                    </NavLink>
                  </li>
                )}
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
