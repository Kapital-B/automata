import { useEffect, useMemo, useRef, useState, type SyntheticEvent } from "react";
import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { CategoryPill } from "@/components/CategoryPill";
import { relativeTime } from "@/lib/accounts";
import {
  categorizeAccount,
  forwardMessage,
  generateDraftSuggestions,
  getForwardAllowlist,
  listCategories,
  listDraftSuggestions,
  listMessages,
  syncAccount,
  type MessageItem,
} from "@/lib/auth";
import { Paperclip, Reply, Forward, Loader2, ChevronDown, ChevronLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useIsBelowLg } from "@/hooks/use-mobile";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/hooks/use-toast";
import { Link, useSearchParams } from "react-router-dom";

interface Props {
  accountFilter: AccountFilter;
}

const HTML_TAG_RE = /<\/?[a-z][\s\S]*>/i;
const HTML_DOCUMENT_RE = /<(?:!doctype|html|head|body)\b/i;
const TEXT_BODY_URL_RE = /\[https?:\/\/[^\]]+\]/i;
const EMAIL_CSP =
  "default-src 'none'; img-src http: https: data: cid: blob:; style-src 'unsafe-inline' http: https:; font-src http: https: data:; connect-src 'none'; script-src 'none'; form-action 'none'; frame-ancestors 'none';";

/** Primary line is display name or address; secondary is the other part when both exist. */
function senderLines(from: MessageItem["from_json"]): { primary: string; secondary?: string } {
  const name = from?.name?.trim() ?? "";
  const addr = from?.address?.trim() ?? "";
  if (name && addr) {
    const same =
      name.toLowerCase().replace(/^mailto:/i, "") === addr.toLowerCase().replace(/^mailto:/i, "");
    if (same) return { primary: name };
    return { primary: name, secondary: addr };
  }
  if (addr) return { primary: addr };
  if (name) return { primary: name };
  return { primary: "Unknown sender" };
}

function isProbablyHtml(body: string) {
  return HTML_TAG_RE.test(body);
}

function looksLikeTextConvertedHtml(body: string) {
  return !isProbablyHtml(body) && TEXT_BODY_URL_RE.test(body);
}

function buildEmailSrcDoc(html: string) {
  const securityHead = `<base target="_blank"><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${EMAIL_CSP}">`;

  if (HTML_DOCUMENT_RE.test(html)) {
    if (/<head\b[^>]*>/i.test(html)) {
      return html.replace(/<head\b([^>]*)>/i, `<head$1>${securityHead}`);
    }
    if (/<html\b[^>]*>/i.test(html)) {
      return html.replace(/<html\b([^>]*)>/i, `<html$1><head>${securityHead}</head>`);
    }
    return `<!doctype html><html><head>${securityHead}</head>${html}</html>`;
  }

  return `<!doctype html>
<html>
  <head>
    ${securityHead}
  </head>
  <body>${html}</body>
</html>`;
}

function EmailBody({ body }: { body: string }) {
  const [height, setHeight] = useState(320);
  const html = useMemo(() => isProbablyHtml(body), [body]);
  const srcDoc = useMemo(() => buildEmailSrcDoc(body), [body]);

  if (!html) {
    return (
      <div className="prose prose-sm max-w-none whitespace-pre-wrap px-6 py-5 text-foreground/90">
        {body}
      </div>
    );
  }

  const resizeFrame = (event: SyntheticEvent<HTMLIFrameElement>) => {
    const documentHeight = event.currentTarget.contentDocument?.documentElement.scrollHeight;
    if (documentHeight) {
      setHeight(Math.min(Math.max(documentHeight, 320), 6000));
    }
  };

  return (
    <div className="px-6 py-5">
      <iframe
        title="Email body"
        srcDoc={srcDoc}
        sandbox="allow-popups allow-popups-to-escape-sandbox allow-same-origin"
        onLoad={resizeFrame}
        className="w-full rounded-md border border-border bg-white"
        style={{ height }}
      />
    </div>
  );
}

export default function InboxPage({ accountFilter }: Props) {
  const { accessToken } = useAuth();
  const { accounts } = useAccountsData();
  const queryClient = useQueryClient();
  const [cat, setCat] = useState<string>("all");
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

  const messagesQuery = useQuery({
    queryKey: ["messages", accessToken, accountFilter, cat],
    queryFn: () =>
      listMessages(accessToken!, {
        accountId: accountFilter === "all" ? undefined : accountFilter,
        category: cat === "all" ? undefined : cat,
        limit: 200,
      }),
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

  const categorizeMutation = useMutation({
    mutationFn: async ({ recategorize }: { recategorize: boolean }) => {
      if (!accessToken || accountFilter === "all") return;
      return categorizeAccount(accessToken, accountFilter, { recategorize });
    },
    onSuccess: (res, vars) => {
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({
        title: vars.recategorize ? "Re-categorization queued" : "Categorization queued",
        description: res?.job_run_id ? `Run ${res.job_run_id.slice(0, 8)} started in background.` : undefined,
      });
    },
    onError: (err) => {
      toast({
        title: "Categorization failed",
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
      toast({ title: "Draft generation queued" });
    },
    onError: (err) => {
      toast({
        title: "Could not queue draft generation",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });
  const forwardMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !selected) throw new Error("Not authenticated");
      const trimmed = forwardTo.trim().toLowerCase();
      if (!trimmed) throw new Error("Choose a destination");
      await forwardMessage(accessToken, selected.id, {
        to_email: trimmed,
        comment: forwardComment.trim() || undefined,
      });
    },
    onSuccess: () => {
      toast({ title: "Message forwarded" });
      setForwardDialogOpen(false);
      setForwardComment("");
    },
    onError: (err) => {
      toast({
        title: "Could not forward message",
        description: err instanceof Error ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const categoryFilters = useMemo(() => {
    const slugs = categoriesQuery.data?.map((c) => c.slug) ?? [];
    return ["all", ...slugs];
  }, [categoriesQuery.data]);

  const filtered = useMemo(
    () => messagesQuery.data ?? [],
    [messagesQuery.data]
  );

  const hasCategorizedMessages = useMemo(
    () => (messagesQuery.data ?? []).some((m: MessageItem) => Boolean(m.category_slug)),
    [messagesQuery.data]
  );

  useEffect(() => {
    if (!deepLinkedMessageID || filtered.length === 0) {
      return;
    }
    const target = filtered.find((m) => m.id === deepLinkedMessageID);
    if (!target) {
      return;
    }
    setSelectedId(target.id);
    setNarrowInboxPane("detail");
    // Clean query string after honoring the deep link to avoid reselect loops.
    const next = new URLSearchParams(searchParams);
    next.delete("message_id");
    next.delete("account_id");
    setSearchParams(next, { replace: true });
  }, [deepLinkedMessageID, filtered, searchParams, setSearchParams]);

  useEffect(() => {
    if (wasStackedInboxRef.current === null) {
      wasStackedInboxRef.current = isStackedInbox;
      return;
    }
    const prev = wasStackedInboxRef.current;
    wasStackedInboxRef.current = isStackedInbox;
    if (isStackedInbox && !prev && selectedId) {
      setNarrowInboxPane("detail");
    }
  }, [isStackedInbox, selectedId]);

  useEffect(() => {
    // Avoid overriding deep-link selection before it is applied.
    if (deepLinkedMessageID) {
      return;
    }
    if (filtered.length === 0) {
      setSelectedId("");
      return;
    }
    if (!selectedId || !filtered.find((m) => m.id === selectedId)) {
      setSelectedId(filtered[0].id);
    }
  }, [deepLinkedMessageID, filtered, selectedId]);

  const selected = filtered.find((m) => m.id === selectedId) ?? filtered[0];
  const selAccount = selected ? accounts.find((a) => a.id === selected.account_id) : undefined;
  const selectedFromLines = selected ? senderLines(selected.from_json) : null;

  useEffect(() => {
    if (!accessToken || !selected?.body_text || !looksLikeTextConvertedHtml(selected.body_text)) {
      return;
    }
    if (htmlRefreshAttempts.has(selected.id)) {
      return;
    }

    setHtmlRefreshAttempts((prev) => new Set(prev).add(selected.id));
    setRefreshingHtmlMessageIds((prev) => new Set(prev).add(selected.id));
    void syncAccount(accessToken, selected.account_id)
      .then(() => queryClient.invalidateQueries({ queryKey: ["messages"] }))
      .catch((err) => {
        toast({
          title: "Could not refresh HTML email",
          description: err instanceof Error ? err.message : "Please try syncing this account again.",
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
  }, [accessToken, htmlRefreshAttempts, queryClient, selected]);

  const isRefreshingSelectedHtml = selected ? refreshingHtmlMessageIds.has(selected.id) : false;
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
  const selectedDraftID = selectedMessageKey ? draftByMessageKey.get(selectedMessageKey) : undefined;
  const selectedDraftPending = selectedMessageKey !== "" && pendingDraftMessageKeys.has(selectedMessageKey);

  const openForwardDialog = () => {
    setForwardComment("");
    const cached = queryClient.getQueryData<{ emails: string[] }>(["forward-allowlist", accessToken]);
    setForwardTo(cached?.emails?.[0] ?? "");
    setForwardDialogOpen(true);
  };

  const showMessageList = !isStackedInbox || narrowInboxPane === "list";
  const showMessageDetail = Boolean(selected && (!isStackedInbox || narrowInboxPane === "detail"));

  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Mailbox"
        title="Inbox"
        description="Per-account messages with LLM-assigned categories. Provenance preserved on every row."
        actions={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={accountFilter === "all" || categorizeMutation.isPending}
              onClick={() => categorizeMutation.mutate({ recategorize: false })}
            >
              {categorizeMutation.isPending ? (
                <>
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  Categorizing...
                </>
              ) : (
                "Categorize new"
              )}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={accountFilter === "all" || categorizeMutation.isPending || !hasCategorizedMessages}
              onClick={() => categorizeMutation.mutate({ recategorize: true })}
            >
              Re-categorize all
            </Button>
          </div>
        }
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-2">
        {categoryFilters.map((c) => (
          <button
            key={c}
            onClick={() => setCat(c)}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition",
              cat === c
                ? "border-foreground bg-foreground text-background"
                : "border-border bg-card text-muted-foreground hover:border-foreground/40 hover:text-foreground"
            )}
          >
            {c}
          </button>
        ))}
        <span className="ml-auto text-xs text-muted-foreground">
          {filtered.length} {filtered.length === 1 ? "message" : "messages"}
        </span>
      </div>

      {messagesQuery.isLoading && (
        <div className="surface-card px-4 py-3 text-sm text-muted-foreground">Loading messages...</div>
      )}
      {messagesQuery.isError && (
        <div className="surface-card px-4 py-3 text-sm text-destructive">
          Could not load messages: {messagesQuery.error instanceof Error ? messagesQuery.error.message : "unknown error"}
        </div>
      )}

      <div
        className={cn(
          "grid gap-6",
          !isStackedInbox && "lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]",
        )}
      >
        {/* List — full width on narrow when browsing messages */}
        {showMessageList && (
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {filtered.map((m) => {
            const acct = accounts.find((a) => a.id === m.account_id);
            const isSel = selected?.id === m.id;
            const from = senderLines(m.from_json);
            return (
              <li
                key={m.id}
                onClick={() => {
                  setSelectedId(m.id);
                  if (isStackedInbox) setNarrowInboxPane("detail");
                }}
                className={cn(
                  "cursor-pointer px-4 py-3 transition",
                  isSel ? "bg-secondary/70" : "hover:bg-secondary/40"
                )}
              >
                <div className="flex items-center justify-between gap-2">
                  <AccountBadge account={acct} />
                  <span className="text-[11px] text-muted-foreground">
                    {relativeTime(m.received_at)}
                  </span>
                </div>
                <p
                  className={cn(
                    "mt-1.5 truncate text-sm",
                    "text-foreground/90"
                  )}
                >
                  {from.primary}
                </p>
                {from.secondary && (
                  <p className="truncate text-xs text-muted-foreground">{from.secondary}</p>
                )}
                <p className="truncate text-sm text-foreground/85">{m.subject}</p>
                <p className="mt-0.5 truncate text-xs text-muted-foreground">{m.preview}</p>
                <div className="mt-2 flex items-center gap-2">
                  <CategoryPill category={m.category_slug ?? "uncategorized"} />
                  {m.has_attachments && (
                    <Paperclip className="h-3 w-3 text-muted-foreground" />
                  )}
                </div>
              </li>
            );
          })}
        </ul>
        )}

        {/* Detail — on narrow widths, replaces the list until user goes back */}
        {showMessageDetail && selected && (
          <article className="surface-card flex flex-col">
            {isStackedInbox && (
              <nav
                aria-label="Inbox navigation"
                className="flex border-b border-border/70 px-3 py-2"
              >
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="-ml-1 h-8 gap-1 px-2 text-muted-foreground hover:text-foreground"
                  onClick={() => setNarrowInboxPane("list")}
                >
                  <ChevronLeft className="h-4 w-4" />
                  Messages
                </Button>
              </nav>
            )}
            <div className="flex min-w-0 items-center justify-between gap-3 border-b border-border/70 px-5 py-3">
              <div className="min-w-0 flex-1">
                <AccountBadge account={selAccount} showEmail className="max-w-full" />
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button size="sm" variant="outline" className="shrink-0 gap-1">
                    Actions
                    <ChevronDown className="h-3.5 w-3.5 opacity-70" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  {selectedDraftID ? (
                    <DropdownMenuItem asChild>
                      <Link
                        to={`/drafts?draft_id=${encodeURIComponent(selectedDraftID)}`}
                        className="flex cursor-pointer items-center"
                      >
                        <Reply className="mr-2 h-3.5 w-3.5" />
                        Open draft
                      </Link>
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem
                      disabled={selectedDraftPending || createDraftMutation.isPending}
                      onClick={() => {
                        if (!selected) return;
                        const key = `${selected.account_id}:${selected.id}`;
                        setPendingDraftMessageKeys((prev) => new Set(prev).add(key));
                        createDraftMutation.mutate({ accountID: selected.account_id, messageID: selected.id });
                      }}
                    >
                      <Reply className="mr-2 h-3.5 w-3.5" />
                      {selectedDraftPending ? "Draft queued…" : "Create draft"}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem disabled={!selected} onClick={() => openForwardDialog()}>
                    <Forward className="mr-2 h-3.5 w-3.5" />
                    Forward…
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            <div className="px-6 py-5">
              <div className="flex items-start justify-between gap-3">
                <h2 className="font-display text-2xl font-medium leading-snug">
                  {selected.subject}
                </h2>
                <CategoryPill category={selected.category_slug ?? "uncategorized"} />
              </div>
              <p className="mt-2 flex flex-wrap items-baseline gap-x-1 text-sm text-muted-foreground">
                <span>From</span>
                <span className="font-medium text-foreground/90">{selectedFromLines?.primary}</span>
                {selectedFromLines?.secondary && (
                  <>
                    <span className="text-muted-foreground">·</span>
                    <span className="font-mono text-xs text-foreground/85">{selectedFromLines.secondary}</span>
                  </>
                )}
                <span className="text-muted-foreground">·</span>
                <span>{relativeTime(selected.received_at)}</span>
              </p>
            </div>
            <div className="hairline" />
            {isRefreshingSelectedHtml && (
              <div className="border-b border-border/70 bg-secondary/40 px-6 py-2 text-xs text-muted-foreground">
                Refreshing the HTML version of this email...
              </div>
            )}
            <EmailBody body={selected.body_text ?? ""} />
          </article>
        )}
      </div>

      <Dialog
        open={forwardDialogOpen}
        onOpenChange={(open) => {
          setForwardDialogOpen(open);
          if (!open) {
            setForwardComment("");
          }
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Forward message</DialogTitle>
            <DialogDescription>
              Only addresses on your forwarding allowlist can receive this message. The original is forwarded via Microsoft Graph
              (including attachments when supported).
            </DialogDescription>
          </DialogHeader>
          {forwardAllowlistQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">Loading allowlist…</p>
          ) : (forwardAllowlistQuery.data?.emails?.length ?? 0) === 0 ? (
            <p className="text-sm text-muted-foreground">
              Add at least one destination under{" "}
              <Link
                to="/rules"
                className="font-medium text-foreground underline-offset-4 hover:underline"
                onClick={() => setForwardDialogOpen(false)}
              >
                Forwarding rules
              </Link>{" "}
              before forwarding.
            </p>
          ) : (
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground" htmlFor="forward-to-select">
                  Destination
                </label>
                <Select
                  value={forwardTo}
                  onValueChange={(v) => {
                    setForwardTo(v);
                  }}
                >
                  <SelectTrigger id="forward-to-select">
                    <SelectValue placeholder="Select allowlisted email" />
                  </SelectTrigger>
                  <SelectContent>
                    {(forwardAllowlistQuery.data?.emails ?? []).map((email) => (
                      <SelectItem key={email} value={email}>
                        {email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground" htmlFor="forward-comment">
                  Comment to recipients (optional)
                </label>
                <Textarea
                  id="forward-comment"
                  value={forwardComment}
                  onChange={(e) => setForwardComment(e.target.value)}
                  placeholder="Shown above the forwarded message in Outlook"
                  rows={3}
                />
              </div>
            </div>
          )}
          <DialogFooter className="gap-2 sm:gap-0">
            <Button type="button" variant="outline" onClick={() => setForwardDialogOpen(false)}>
              Cancel
            </Button>
            <Button
              type="button"
              disabled={
                forwardMutation.isPending ||
                forwardAllowlistQuery.isLoading ||
                (forwardAllowlistQuery.data?.emails?.length ?? 0) === 0 ||
                !forwardTo.trim()
              }
              onClick={() => forwardMutation.mutate()}
            >
              {forwardMutation.isPending ? (
                <>
                  <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  Forwarding…
                </>
              ) : (
                "Confirm forward"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
