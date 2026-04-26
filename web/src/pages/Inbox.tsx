import { useEffect, useMemo, useState, type SyntheticEvent } from "react";
import { PageHeader } from "@/components/PageHeader";
import { AccountBadge } from "@/components/AccountBadge";
import { CategoryPill } from "@/components/CategoryPill";
import { relativeTime } from "@/lib/accounts";
import { categorizeAccount, listCategories, listMessages, syncAccount, type MessageItem } from "@/lib/auth";
import { Paperclip, Reply, Forward, Archive, Loader2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "@/hooks/use-toast";

interface Props {
  accountFilter: AccountFilter;
}

const HTML_TAG_RE = /<\/?[a-z][\s\S]*>/i;
const HTML_DOCUMENT_RE = /<(?:!doctype|html|head|body)\b/i;
const TEXT_BODY_URL_RE = /\[https?:\/\/[^\]]+\]/i;
const EMAIL_CSP =
  "default-src 'none'; img-src http: https: data: cid: blob:; style-src 'unsafe-inline' http: https:; font-src http: https: data:; connect-src 'none'; script-src 'none'; form-action 'none'; frame-ancestors 'none';";

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

  const categorizeMutation = useMutation({
    mutationFn: async ({ recategorize }: { recategorize: boolean }) => {
      if (!accessToken || accountFilter === "all") return;
      return categorizeAccount(accessToken, accountFilter, { recategorize });
    },
    onSuccess: (_, vars) => {
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      toast({ title: vars.recategorize ? "Re-categorization completed" : "Categorization completed" });
    },
    onError: (err) => {
      toast({
        title: "Categorization failed",
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
    if (filtered.length === 0) {
      setSelectedId("");
      return;
    }
    if (!selectedId || !filtered.find((m) => m.id === selectedId)) {
      setSelectedId(filtered[0].id);
    }
  }, [filtered, selectedId]);

  const selected = filtered.find((m) => m.id === selectedId) ?? filtered[0];
  const selAccount = selected ? accounts.find((a) => a.id === selected.account_id) : undefined;

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

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.2fr)]">
        {/* List */}
        <ul className="surface-card divide-y divide-border/70 overflow-hidden">
          {filtered.map((m) => {
            const acct = accounts.find((a) => a.id === m.account_id);
            const isSel = selected?.id === m.id;
            return (
              <li
                key={m.id}
                onClick={() => setSelectedId(m.id)}
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
                  {m.from_json?.name ?? m.from_json?.address ?? "Unknown sender"}
                </p>
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

        {/* Detail */}
        {selected && (
          <article className="surface-card flex flex-col">
            <div className="flex items-center justify-between border-b border-border/70 px-5 py-3">
              <AccountBadge account={selAccount} showEmail />
              <div className="flex items-center gap-1">
                <Button size="sm" variant="ghost"><Reply className="mr-1.5 h-3.5 w-3.5" />Reply</Button>
                <Button size="sm" variant="ghost"><Forward className="mr-1.5 h-3.5 w-3.5" />Forward</Button>
                <Button size="sm" variant="ghost"><Archive className="h-3.5 w-3.5" /></Button>
              </div>
            </div>
            <div className="px-6 py-5">
              <div className="flex items-start justify-between gap-3">
                <h2 className="font-display text-2xl font-medium leading-snug">
                  {selected.subject}
                </h2>
                <CategoryPill category={selected.category_slug ?? "uncategorized"} />
              </div>
              <p className="mt-2 text-sm text-muted-foreground">
                <span className="font-medium text-foreground/80">
                  {selected.from_json?.name ?? selected.from_json?.address ?? "Unknown sender"}
                </span>{" "}
                &lt;{selected.from_json?.address ?? "unknown"}&gt; · {relativeTime(selected.received_at)}
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
    </div>
  );
}
