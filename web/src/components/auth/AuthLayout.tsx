import { Link } from "react-router-dom";
import { ReactNode } from "react";
import { Sparkles, Zap, ShieldCheck } from "lucide-react";

interface AuthLayoutProps {
  eyebrow: string;
  title: string;
  subtitle: string;
  children: ReactNode;
}

export function AuthLayout({ eyebrow, title, subtitle, children }: AuthLayoutProps) {
  return (
    <div className="min-h-screen grid lg:grid-cols-2 bg-background">
      {/* Left: form */}
      <div className="flex flex-col">
        <header className="px-6 lg:px-12 py-6">
          <Link to="/" className="inline-flex items-center gap-2">
            <div
              className="grid h-8 w-8 place-items-center rounded-md text-primary-foreground"
              style={{ background: "var(--gradient-ink)" }}
            >
              <span className="font-display text-sm font-semibold">P</span>
            </div>
            <div className="leading-tight">
              <p className="font-display text-base font-semibold">Postern</p>
              <p className="text-[10px] uppercase tracking-widest text-muted-foreground">
                workspace intelligence
              </p>
            </div>
          </Link>
        </header>

        <main className="flex-1 flex items-center justify-center px-6 lg:px-12 py-8">
          <div className="w-full max-w-md">
            <div className="mb-8">
              <p className="text-[10px] uppercase tracking-widest text-muted-foreground mb-2">
                {eyebrow}
              </p>
              <h1 className="font-display text-3xl lg:text-4xl font-semibold tracking-tight text-foreground">
                {title}
              </h1>
              <p className="mt-2 text-muted-foreground">{subtitle}</p>
            </div>
            <div className="rounded-xl border border-border bg-card p-6 lg:p-8 shadow-sm">
              {children}
            </div>
          </div>
        </main>

        <footer className="px-6 lg:px-12 py-6 text-xs text-muted-foreground">
          © {new Date().getFullYear()} Postern · Privacy · Terms
        </footer>
      </div>

      {/* Right: editorial panel */}
      <aside
        className="hidden lg:flex relative overflow-hidden"
        style={{ background: "var(--gradient-ink)" }}
      >
        <div className="absolute inset-0 opacity-[0.07]" aria-hidden>
          <div
            className="absolute inset-0"
            style={{
              backgroundImage:
                "radial-gradient(circle at 20% 20%, hsl(var(--primary-foreground)) 0, transparent 40%), radial-gradient(circle at 80% 60%, hsl(var(--accent)) 0, transparent 40%)",
            }}
          />
        </div>

        <div className="relative z-10 flex flex-col justify-between p-12 text-primary-foreground w-full">
          <div>
            <p className="text-[10px] uppercase tracking-widest opacity-70 mb-3">
              The dispatch
            </p>
            <p className="font-display text-3xl xl:text-4xl leading-tight max-w-md">
              "Reads everything. Surfaces only what matters. Drafts the reply before you ask."
            </p>
          </div>

          <ul className="space-y-5 max-w-md">
            <Feature
              icon={<Sparkles className="h-4 w-4" />}
              title="Conversational by default"
              body="Ask anything across your inbox, Slack, Linear and more."
            />
            <Feature
              icon={<Zap className="h-4 w-4" />}
              title="Action, not noise"
              body="Daily summaries, prioritized threads, drafts ready to send."
            />
            <Feature
              icon={<ShieldCheck className="h-4 w-4" />}
              title="Yours alone"
              body="Bring your own model. Local LLM and MCP-server ready."
            />
          </ul>
        </div>
      </aside>
    </div>
  );
}

function Feature({ icon, title, body }: { icon: ReactNode; title: string; body: string }) {
  return (
    <li className="flex gap-4">
      <span className="grid h-9 w-9 shrink-0 place-items-center rounded-md bg-primary-foreground/10 ring-1 ring-primary-foreground/15">
        {icon}
      </span>
      <div>
        <p className="font-medium">{title}</p>
        <p className="text-sm opacity-75">{body}</p>
      </div>
    </li>
  );
}
