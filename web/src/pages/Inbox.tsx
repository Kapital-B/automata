import { useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Loader2, Tags } from "lucide-react";
import { PageHeader } from "@/components/PageHeader";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useIsBelowLg } from "@/hooks/use-mobile";
import { toast } from "@/hooks/use-toast";
import {
  categorizeAccount,
  forwardMessage,
  generateDraftSuggestions,
  getForwardAllowlist,
  getMessage,
  listCategories,
  listDraftSuggestions,
  listMessages,
  listProjects,
  syncAccount,
} from "@/lib/auth";
import { cn } from "@/lib/utils";
import { ForwardDialog } from "./inbox/ForwardDialog";
import { MessageDetail } from "./inbox/MessageDetail";
import { MessageList } from "./inbox/MessageList";
import { looksLikeTextConvertedHtml } from "./inbox/format";

interface Props {
  accountFilter: AccountFilter;
}

const INBOX_PAGE_SIZE = 50;

/**
 * Mail from connected accounts. Wide screens show the list and the open
 * message side by side; narrow screens show one at a time, with a way back.
 */
export default function InboxPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const [cat, setCat] = useState<string>("all");
  const [projectFilter, setProjectFilter] = useState<string>("all");
  const [selectedId, setSelectedId] = useState<string>("");
  const [htmlRefreshAttempts, setHtmlRefreshAttempts] = useState<Set<string>>(() => new Set());
  const [refreshingHtmlMessageIds, setRefreshingHtmlMessageIds] = useState<Set<string>>(() => new Set());
  const [pendingDraftMessageKeys, setPendingDraftMessageKeys] = useState<Set<string>>(() => new Set());
  const [forwardDialogOpen, setForwardDialogOpen] = useState(false);
  const [forwardTo, setForwardTo] = useState("");
  const [forwardComment, setForwardComment] = useState("");
  /** Narrow layout: show either list or message, not both stacked. */
  const [narrowInboxPane, setNarrowInboxPane] = useState<"list" | "detail">("list");
  const [searchParams, setSearchParams] = useSearchParams();
  const isStackedInbox = useIsBelowLg();
  const wasStackedInboxRef = useRef<boolean | null>(null);
  const deepLinkedMessageID = searchParams.get("message_id");

  const categoriesQuery = useQuery({
    queryKey: ["categories", accessToken],
    queryFn: () => listCategories(accessToken!),
    enabled: Boolean(accessToken),
  });
  const messagesQuery = useInfiniteQuery({
    queryKey: ["messages", accessToken, accountFilter, cat, projectFilter],
    queryFn: ({ pageParam }) =>
      listMessages(accessToken!, {
        accountId: accountFilter === "all" ? undefined : accountFilter,
        category: cat === "all" ? undefined : cat,
        projectId: projectFilter === "all" ? undefined : projectFilter,
        limit: INBOX_PAGE_SIZE,
        offset: pageParam,
      }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) => (lastPage.length === INBOX_PAGE_SIZE ? pages.length * INBOX_PAGE_SIZE : undefined),
    enabled: Boolean(accessToken),
  });
  const selectedMessageQuery = useQuery({
    queryKey: ["message", accessToken, selectedId],
    queryFn: () => getMessage(accessToken!, selectedId),
    enabled: Boolean(accessToken && selectedId),
  });
  const projectsQuery = useQuery({
    queryKey: ["projects", accessToken],
    queryFn: () => listProjects(accessToken!),
    enabled: Boolean(accessToken),
  });
  const draftsScope = accountFilter === "all" ? "all" : accountFilter;
  const draftsQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, draftsScope],
    queryFn: () => listDraftSuggestions(accessToken!, accountFilter === "all" ? undefined : accountFilter),
    enabled: Boolean(accessToken),
  });
  const forwardAllowlistQuery = useQuery({
    queryKey: ["forward-allowlist", accessToken],
    queryFn: () => getForwardAllowlist(accessToken!),
    enabled: Boolean(accessToken && forwardDialogOpen),
  });

  const projectCodeByID = useMemo(() => {
    const map = new Map<string, string>();
    for (const p of projectsQuery.data ?? []) map.set(p.id, p.code);
    return map;
  }, [projectsQuery.data]);
  const categoryNameBySlug = useMemo(() => {
    const map = new Map<string, string>();
    for (const c of categoriesQuery.data ?? []) map.set(c.slug, c.display_name);
    return map;
  }, [categoriesQuery.data]);

  const categorizeMutation = useMutation({
    mutationFn: async ({ recategorize }: { recategorize: boolean }) => {
      if (!accessToken || accountFilter === "all") return;
      return categorizeAccount(accessToken, accountFilter, { recategorize });
    },
    onSuccess: (res, vars) => {
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: vars.recategorize ? "Re-categorising queued" : "Categorising queued",
        description: res?.job_run_id ? `Run ${res.job_run_id.slice(0, 8)} started in the background.` : undefined,
      });
    },
    onError: (err) => {
      toast({
        title: "Categorising failed",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });
  const createDraftMutation = useMutation({
    mutationFn: async ({ accountID, messageID }: { accountID: string; messageID: string }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return generateDraftSuggestions(accessToken, accountID, { messageId: messageID });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["draft-suggestions"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({ title: "Drafting a reply" });
    },
    onError: (err, vars) => {
      setPendingDraftMessageKeys((prev) => {
        const next = new Set(prev);
        next.delete(`${vars.accountID}:${vars.messageID}`);
        return next;
      });
      toast({
        title: "Could not start the draft",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const messages = useMemo(() => messagesQuery.data?.pages.flat() ?? [], [messagesQuery.data]);
  const hasCategorizedMessages = useMemo(() => messages.some((m) => Boolean(m.category_slug)), [messages]);

  useEffect(() => {
    if (!deepLinkedMessageID || messages.length === 0) return;
    const target = messages.find((m) => m.id === deepLinkedMessageID);
    if (!target) return;
    setSelectedId(target.id);
    setNarrowInboxPane("detail");
    // Clean query string after honoring the deep link to avoid reselect loops.
    const next = new URLSearchParams(searchParams);
    next.delete("message_id");
    next.delete("account_id");
    setSearchParams(next, { replace: true });
  }, [deepLinkedMessageID, messages, searchParams, setSearchParams]);

  useEffect(() => {
    if (wasStackedInboxRef.current === null) {
      wasStackedInboxRef.current = isStackedInbox;
      return;
    }
    const prev = wasStackedInboxRef.current;
    wasStackedInboxRef.current = isStackedInbox;
    if (isStackedInbox && !prev && selectedId) setNarrowInboxPane("detail");
  }, [isStackedInbox, selectedId]);

  useEffect(() => {
    // Avoid overriding deep-link selection before it is applied.
    if (deepLinkedMessageID) return;
    if (messages.length === 0) {
      setSelectedId("");
      return;
    }
    if (!selectedId || !messages.find((m) => m.id === selectedId)) setSelectedId(messages[0].id);
  }, [deepLinkedMessageID, messages, selectedId]);

  const selected = messages.find((m) => m.id === selectedId) ?? messages[0];
  const selectedBody = selectedMessageQuery.data?.body_text;

  const forwardMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !selected) throw new Error("Not authenticated");
      const trimmed = forwardTo.trim().toLowerCase();
      if (!trimmed) throw new Error("Choose a destination");
      await forwardMessage(accessToken, selected.id, { to_email: trimmed, comment: forwardComment.trim() || undefined });
    },
    onSuccess: () => {
      toast({ title: "Message forwarded" });
      setForwardDialogOpen(false);
      setForwardComment("");
    },
    onError: (err) => {
      toast({
        title: "Could not forward the message",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  // A plain-text body carrying bracketed links is HTML mail that was
  // flattened on an earlier sync; re-sync once to fetch the formatted version.
  useEffect(() => {
    if (!accessToken || !selected || !selectedBody || !looksLikeTextConvertedHtml(selectedBody)) return;
    if (htmlRefreshAttempts.has(selected.id)) return;

    setHtmlRefreshAttempts((prev) => new Set(prev).add(selected.id));
    setRefreshingHtmlMessageIds((prev) => new Set(prev).add(selected.id));
    void syncAccount(accessToken, selected.account_id)
      .then(() => {
        void queryClient.invalidateQueries({ queryKey: ["messages"] });
        void queryClient.invalidateQueries({ queryKey: ["message", accessToken, selected.id] });
      })
      .catch((err) => {
        toast({
          title: "Could not fetch the formatted email",
          description: err instanceof Error ? err.message : "Try syncing this account again.",
          variant: "destructive",
        });
      })
      .finally(() => {
        setRefreshingHtmlMessageIds((prev) => {
          const next = new Set(prev);
          next.delete(selected.id);
          return next;
        });
      });
  }, [accessToken, htmlRefreshAttempts, queryClient, selected, selectedBody]);

  const draftByMessageKey = useMemo(() => {
    const map = new Map<string, string>();
    for (const d of draftsQuery.data ?? []) {
      const key = `${d.account_id}:${d.message_id}`;
      if (!map.has(key)) map.set(key, d.id);
    }
    return map;
  }, [draftsQuery.data]);
  useEffect(() => {
    setPendingDraftMessageKeys((prev) => {
      if (prev.size === 0) return prev;
      const next = new Set(prev);
      for (const key of prev) {
        if (draftByMessageKey.has(key)) next.delete(key);
      }
      return next;
    });
  }, [draftByMessageKey]);

  useEffect(() => {
    if (!forwardDialogOpen) return;
    const emails = forwardAllowlistQuery.data?.emails ?? [];
    if (emails.length === 0) return;
    setForwardTo((prev) => (prev && emails.includes(prev) ? prev : emails[0]));
  }, [forwardDialogOpen, forwardAllowlistQuery.data?.emails]);

  const selectedMessageKey = selected ? `${selected.account_id}:${selected.id}` : "";

  const openForwardDialog = () => {
    setForwardComment("");
    const cached = queryClient.getQueryData<{ emails: string[] }>(["forward-allowlist", accessToken]);
    setForwardTo(cached?.emails?.[0] ?? "");
    setForwardDialogOpen(true);
  };

  const filtersActive = cat !== "all" || projectFilter !== "all";
  const showList = !isStackedInbox || narrowInboxPane === "list";
  const showDetail = Boolean(selected && (!isStackedInbox || narrowInboxPane === "detail"));
  const oneAccount = accountFilter !== "all";

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Correspondence"
        title="Inbox"
        description="Mail from your connected accounts, sorted into categories. File a message to a project, draft a reply or forward it from here."
        actions={
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="outline" disabled={categorizeMutation.isPending}>
                {categorizeMutation.isPending ? (
                  <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Tags aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
                )}
                Categorise
                <ChevronDown aria-hidden="true" className="ml-1 h-3.5 w-3.5 opacity-70" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-64">
              {!oneAccount && (
                <>
                  <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
                    Choose one account in the top bar to categorise its mail.
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                </>
              )}
              <DropdownMenuItem disabled={!oneAccount} onClick={() => categorizeMutation.mutate({ recategorize: false })}>
                Categorise new mail
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!oneAccount || !hasCategorizedMessages}
                onClick={() => categorizeMutation.mutate({ recategorize: true })}
              >
                Re-categorise everything
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        }
      />

      {showList && (
        <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div role="radiogroup" aria-label="Category" className="flex min-w-0 flex-wrap gap-1.5">
            {[{ slug: "all", name: "All" }, ...(categoriesQuery.data ?? []).map((c) => ({ slug: c.slug, name: c.display_name }))].map(
              (c) => (
                <button
                  key={c.slug}
                  type="button"
                  role="radio"
                  aria-checked={cat === c.slug}
                  onClick={() => setCat(c.slug)}
                  className={cn(
                    "h-8 max-w-full truncate rounded-full border px-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    cat === c.slug
                      ? "border-foreground bg-foreground text-background"
                      : "border-border bg-card text-muted-foreground hover:border-foreground/40 hover:text-foreground",
                  )}
                >
                  {c.name}
                </button>
              ),
            )}
          </div>
          <div className="flex min-w-0 items-center gap-3">
            <label htmlFor="inbox-project" className="sr-only">
              Project
            </label>
            <select
              id="inbox-project"
              className="h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring lg:w-56 lg:flex-none"
              value={projectFilter}
              onChange={(e) => setProjectFilter(e.target.value)}
            >
              <option value="all">All projects</option>
              {(projectsQuery.data ?? []).map((p) => (
                <option key={p.id} value={p.id}>
                  {p.code} — {p.name}
                </option>
              ))}
            </select>
            {!messagesQuery.isLoading && messages.length > 0 && (
              <p className="shrink-0 text-sm text-muted-foreground" aria-live="polite">
                {messages.length}
                {messagesQuery.hasNextPage ? "+" : ""} {messages.length === 1 ? "message" : "messages"}
              </p>
            )}
          </div>
        </div>
      )}

      <div className={cn("grid min-w-0 gap-6", !isStackedInbox && "lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]")}>
        {showList && (
          <MessageList
            messages={messages}
            selectedID={isStackedInbox ? undefined : selected?.id}
            onSelect={(id) => {
              setSelectedId(id);
              if (isStackedInbox) setNarrowInboxPane("detail");
            }}
            accountFor={(id) => accounts.find((a) => a.id === id)}
            showAccount={!oneAccount && accounts.length > 1}
            projectCode={(id) => (id ? projectCodeByID.get(id) : undefined)}
            categoryName={(slug) => (slug ? categoryNameBySlug.get(slug) : undefined)}
            loading={messagesQuery.isLoading}
            error={
              messagesQuery.isError && !messagesQuery.data
                ? messagesQuery.error instanceof Error
                  ? messagesQuery.error.message
                  : "unknown error"
                : undefined
            }
            filtered={filtersActive}
            hasAccounts={accounts.length > 0}
            onClearFilters={() => {
              setCat("all");
              setProjectFilter("all");
            }}
            hasMore={Boolean(messagesQuery.hasNextPage)}
            loadingMore={messagesQuery.isFetchingNextPage}
            loadMoreFailed={messagesQuery.isFetchNextPageError}
            onLoadMore={() => void messagesQuery.fetchNextPage()}
          />
        )}

        {showDetail && selected && (
          <div className="min-w-0 lg:sticky lg:top-6 lg:max-h-[calc(100vh-3rem)] lg:self-start lg:overflow-y-auto">
            <MessageDetail
              message={selected}
              account={accounts.find((a) => a.id === selected.account_id)}
              categoryName={selected.category_slug ? categoryNameBySlug.get(selected.category_slug) : undefined}
              body={selectedBody}
              bodyLoading={selectedMessageQuery.isLoading}
              refreshingHtml={refreshingHtmlMessageIds.has(selected.id)}
              draftID={draftByMessageKey.get(selectedMessageKey)}
              draftPending={pendingDraftMessageKeys.has(selectedMessageKey) || createDraftMutation.isPending}
              onCreateDraft={() => {
                setPendingDraftMessageKeys((prev) => new Set(prev).add(selectedMessageKey));
                createDraftMutation.mutate({ accountID: selected.account_id, messageID: selected.id });
              }}
              onForward={openForwardDialog}
              onBack={isStackedInbox ? () => setNarrowInboxPane("list") : undefined}
            />
          </div>
        )}
      </div>

      <ForwardDialog
        open={forwardDialogOpen}
        onOpenChange={(open) => {
          setForwardDialogOpen(open);
          if (!open) setForwardComment("");
        }}
        loading={forwardAllowlistQuery.isLoading}
        allowlist={forwardAllowlistQuery.data?.emails ?? []}
        to={forwardTo}
        onToChange={setForwardTo}
        comment={forwardComment}
        onCommentChange={setForwardComment}
        pending={forwardMutation.isPending}
        onConfirm={() => forwardMutation.mutate()}
      />
    </div>
  );
}
