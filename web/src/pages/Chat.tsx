import { useEffect, useRef, useState } from "react";
import { Sparkles, ArrowUp, Paperclip, Mail, Slack, Zap, Inbox, FileText, Plug } from "lucide-react";
import { cn } from "@/lib/utils";
import { accounts } from "@/lib/mock-data";
import type { AccountFilter } from "@/components/AppShell";
import { Link } from "react-router-dom";

type Role = "user" | "assistant";
interface ChatMessage {
  id: string;
  role: Role;
  content: string;
  sources?: { label: string; colorVar?: string; href?: string }[];
}

const SUGGESTIONS = [
  {
    icon: Inbox,
    title: "What needs my attention today?",
    sub: "Across all connected inboxes",
  },
  {
    icon: Mail,
    title: "Draft a reply to the latest invoice",
    sub: "From your Finance category",
  },
  {
    icon: FileText,
    title: "Summarize this week's newsletters",
    sub: "Group by topic, skip promos",
  },
  {
    icon: Zap,
    title: "Create a rule for receipts",
    sub: "Forward to bookkeeping@",
  },
];

const CONNECTORS = [
  { name: "Email", status: "connected", icon: Mail },
  { name: "Slack", status: "soon", icon: Slack },
  { name: "Linear", status: "soon", icon: Zap },
  { name: "MCP servers", status: "soon", icon: Plug },
];

function mockReply(prompt: string): ChatMessage {
  const p = prompt.toLowerCase();
  if (p.includes("attention") || p.includes("today")) {
    return {
      id: crypto.randomUUID(),
      role: "assistant",
      content:
        "Three things need a response today:\n\n1. **Stripe** – verification documents requested for your work account (due tomorrow).\n2. **Anna Liu** – waiting on your decision about the Q2 roadmap call.\n3. **Landlord** – lease renewal needs a signature by Friday.\n\nWant me to draft replies to any of these?",
      sources: [
        { label: "work@", colorVar: "acct-1" },
        { label: "personal@", colorVar: "acct-2" },
      ],
    };
  }
  if (p.includes("draft") || p.includes("reply")) {
    return {
      id: crypto.randomUUID(),
      role: "assistant",
      content:
        "Here's a draft for the latest invoice from **Vercel ($240.00)**:\n\n> Hi team — confirming receipt of invoice #INV-8821. Payment has been queued and should clear by end of week. Let me know if anything changes.\n\nWant me to send it, or edit first?",
      sources: [{ label: "work@", colorVar: "acct-1" }],
    };
  }
  if (p.includes("rule") || p.includes("forward")) {
    return {
      id: crypto.randomUUID(),
      role: "assistant",
      content:
        "I can set this up. Proposed rule:\n\n- **When** category = Finance and subject contains \"receipt\"\n- **Then** forward to `bookkeeping@yourco.com` and tag as Archived\n\nShall I create it on your work account?",
    };
  }
  return {
    id: crypto.randomUUID(),
    role: "assistant",
    content:
      "I'm wired up to your connected accounts and can read, summarize, draft, and act across them. Try asking what needs attention, or pick a suggestion below.",
  };
}

export default function ChatPage({ accountFilter }: { accountFilter: AccountFilter }) {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState("");
  const [isThinking, setIsThinking] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);

  const current = accounts.find((a) => a.id === accountFilter);
  const scope = current ? current.label : "all accounts";

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages, isThinking]);

  const send = (text: string) => {
    const trimmed = text.trim();
    if (!trimmed) return;
    const userMsg: ChatMessage = { id: crypto.randomUUID(), role: "user", content: trimmed };
    setMessages((m) => [...m, userMsg]);
    setInput("");
    setIsThinking(true);
    setTimeout(() => {
      setMessages((m) => [...m, mockReply(trimmed)]);
      setIsThinking(false);
    }, 700);
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send(input);
    }
  };

  const empty = messages.length === 0;

  return (
    <div className="flex h-[calc(100vh-3.5rem-5rem)] min-h-[520px] flex-col">
      {/* Conversation / empty state */}
      <div ref={scrollRef} className="flex-1 overflow-y-auto">
        {empty ? (
          <div className="mx-auto flex h-full max-w-3xl flex-col items-center justify-center px-2 text-center">
            <div
              className="mb-5 grid h-12 w-12 place-items-center rounded-xl text-primary-foreground"
              style={{ background: "var(--gradient-ink)" }}
            >
              <Sparkles className="h-5 w-5" />
            </div>
            <h1 className="font-display text-4xl font-medium tracking-tight md:text-5xl">
              How can I help across <span className="italic text-primary">{scope}</span>?
            </h1>
            <p className="mt-3 max-w-xl text-sm text-muted-foreground">
              Ask anything about your connected workspaces. I can read inboxes, draft replies,
              build rules, and — soon — act across Slack, Linear, and MCP servers.
            </p>

            {/* Connector status row */}
            <div className="mt-6 flex flex-wrap items-center justify-center gap-2">
              {CONNECTORS.map((c) => (
                <span
                  key={c.name}
                  className={cn(
                    "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px]",
                    c.status === "connected"
                      ? "border-success/30 bg-success/10 text-success"
                      : "border-border bg-card text-muted-foreground"
                  )}
                >
                  <c.icon className="h-3 w-3" />
                  {c.name}
                  {c.status !== "connected" && (
                    <span className="text-[10px] uppercase tracking-wider opacity-70">soon</span>
                  )}
                </span>
              ))}
            </div>

            {/* Suggestions */}
            <div className="mt-8 grid w-full grid-cols-1 gap-2 sm:grid-cols-2">
              {SUGGESTIONS.map((s) => (
                <button
                  key={s.title}
                  onClick={() => send(s.title)}
                  className="group surface-card flex items-start gap-3 px-4 py-3 text-left transition hover:border-foreground/30"
                >
                  <span className="mt-0.5 grid h-7 w-7 place-items-center rounded-md bg-secondary text-foreground/70 transition group-hover:bg-primary group-hover:text-primary-foreground">
                    <s.icon className="h-3.5 w-3.5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium leading-tight">{s.title}</span>
                    <span className="mt-0.5 block text-xs text-muted-foreground">{s.sub}</span>
                  </span>
                </button>
              ))}
            </div>

            <p className="mt-6 text-[11px] text-muted-foreground">
              Need the classic view?{" "}
              <Link to="/today" className="underline underline-offset-2 hover:text-foreground">
                Open the dashboard
              </Link>
            </p>
          </div>
        ) : (
          <div className="mx-auto max-w-3xl space-y-6 px-2 py-6">
            {messages.map((m) => (
              <MessageBubble key={m.id} message={m} />
            ))}
            {isThinking && (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <span className="acct-dot animate-pulse bg-primary" />
                Thinking…
              </div>
            )}
          </div>
        )}
      </div>

      {/* Composer */}
      <div className="mx-auto w-full max-w-3xl pt-3">
        <div className="surface-card flex items-end gap-2 px-3 py-2.5 shadow-[var(--shadow-elevated)]">
          <button
            type="button"
            className="grid h-8 w-8 shrink-0 place-items-center rounded-md text-muted-foreground hover:bg-secondary hover:text-foreground"
            title="Attach context (coming soon)"
          >
            <Paperclip className="h-4 w-4" />
          </button>
          <textarea
            ref={taRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            rows={1}
            placeholder={`Message your assistant — scope: ${scope}`}
            className="max-h-40 min-h-[28px] flex-1 resize-none bg-transparent px-1 py-1 text-sm placeholder:text-muted-foreground focus:outline-none"
          />
          <button
            onClick={() => send(input)}
            disabled={!input.trim()}
            className={cn(
              "grid h-8 w-8 shrink-0 place-items-center rounded-md transition",
              input.trim()
                ? "bg-primary text-primary-foreground hover:opacity-90"
                : "bg-secondary text-muted-foreground"
            )}
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        </div>
        <p className="mt-2 px-1 text-[11px] text-muted-foreground">
          Replies are mocked while we wire up the backend. Provenance from each connector will
          always be cited inline.
        </p>
      </div>
    </div>
  );
}

function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user";
  return (
    <div className={cn("flex gap-3", isUser && "flex-row-reverse")}>
      <div
        className={cn(
          "grid h-7 w-7 shrink-0 place-items-center rounded-md text-[11px] font-semibold",
          isUser ? "bg-secondary text-foreground" : "text-primary-foreground"
        )}
        style={!isUser ? { background: "var(--gradient-ink)" } : undefined}
      >
        {isUser ? "You" : <Sparkles className="h-3.5 w-3.5" />}
      </div>
      <div className={cn("min-w-0 flex-1", isUser && "flex justify-end")}>
        <div
          className={cn(
            "inline-block max-w-full rounded-lg px-3.5 py-2.5 text-sm leading-relaxed",
            isUser
              ? "bg-primary text-primary-foreground"
              : "surface-card whitespace-pre-wrap"
          )}
        >
          {renderMarkdownLite(message.content)}
        </div>
        {message.sources && message.sources.length > 0 && (
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            {message.sources.map((s) => (
              <span
                key={s.label}
                className="inline-flex items-center gap-1 rounded-full border border-border bg-card px-2 py-0.5 text-[10px] text-muted-foreground"
              >
                <span
                  className="acct-dot"
                  style={s.colorVar ? { background: `hsl(var(--${s.colorVar}))` } : undefined}
                />
                {s.label}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// Tiny markdown-ish renderer: **bold**, > quotes, lists. Avoids pulling a dep.
function renderMarkdownLite(text: string) {
  return text.split("\n").map((line, i) => {
    if (line.startsWith("> ")) {
      return (
        <p key={i} className="my-1 border-l-2 border-primary/40 pl-3 italic text-foreground/80">
          {bold(line.slice(2))}
        </p>
      );
    }
    if (/^\d+\.\s/.test(line)) {
      return (
        <p key={i} className="my-0.5">
          {bold(line)}
        </p>
      );
    }
    if (line.startsWith("- ")) {
      return (
        <p key={i} className="my-0.5 pl-3">
          • {bold(line.slice(2))}
        </p>
      );
    }
    if (!line.trim()) return <div key={i} className="h-2" />;
    return (
      <p key={i} className="my-0.5">
        {bold(line)}
      </p>
    );
  });
}

function bold(s: string) {
  const parts = s.split(/(\*\*[^*]+\*\*)/g);
  return parts.map((p, i) =>
    p.startsWith("**") && p.endsWith("**") ? (
      <strong key={i} className="font-semibold">
        {p.slice(2, -2)}
      </strong>
    ) : (
      <span key={i}>{p}</span>
    )
  );
}
