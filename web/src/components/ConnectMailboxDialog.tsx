import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import {
  ApiError,
  connectImapAccount,
  startMailboxConnect,
  type MailProviderId,
  type MailProviderOption,
  type MailServerSettings,
} from "@/lib/auth";
import { mailPresets, presetForEmail } from "@/lib/mailPresets";

/** Pre-selects a provider, e.g. to reconnect an expired account. */
export type ConnectTarget = {
  provider: MailProviderId;
  kind?: "work" | "personal";
  email?: string;
};

const providerCopy: Record<MailProviderId, { title: string; blurb: string }> = {
  m365: { title: "Microsoft", blurb: "Microsoft 365, Outlook.com, Hotmail, Live" },
  google: { title: "Google", blurb: "Gmail and Google Workspace" },
  imap: { title: "Other (IMAP)", blurb: "Fastmail, iCloud, Zoho, Yahoo, your own server" },
};

export function ConnectMailboxDialog({
  open,
  onOpenChange,
  providers,
  target,
  onConnected,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providers: MailProviderOption[];
  target?: ConnectTarget;
  onConnected: (accountID: string) => void;
}) {
  const [provider, setProvider] = useState<MailProviderId | null>(null);

  useEffect(() => {
    if (open) setProvider(target?.provider ?? null);
  }, [open, target]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        {provider === null ? (
          <>
            <DialogHeader>
              <DialogTitle className="font-display text-xl">Connect a mailbox</DialogTitle>
              <DialogDescription>
                Each mailbox is its own account. Credentials are stored encrypted, per account.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-3 pt-2">
              {providers.map((p) => (
                <button
                  key={p.provider}
                  onClick={() => setProvider(p.provider)}
                  className="rounded-lg border border-border p-4 text-left transition hover:border-foreground/40"
                >
                  <p className="font-display text-base font-medium">{providerCopy[p.provider]?.title ?? p.provider}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{providerCopy[p.provider]?.blurb}</p>
                </button>
              ))}
              {providers.length === 0 && (
                <p className="text-sm text-muted-foreground">No mailbox providers are configured on this server.</p>
              )}
            </div>
          </>
        ) : (
          <>
            {!target && (
              <button
                onClick={() => setProvider(null)}
                className="inline-flex w-fit items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
              >
                <ArrowLeft className="h-3 w-3" /> All providers
              </button>
            )}
            {provider === "m365" && <MicrosoftStep initialKind={target?.kind} />}
            {provider === "google" && <GoogleStep />}
            {provider === "imap" && <ImapStep initialEmail={target?.email} onConnected={onConnected} />}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function useOAuthStart(title: string) {
  const { accessToken } = useAuth();
  return useMutation({
    mutationFn: async (args: { provider: "m365" | "google"; kind?: "work" | "personal" }) => {
      if (!accessToken) throw new Error("Not authenticated");
      return startMailboxConnect(accessToken, args.provider, args.kind);
    },
    onSuccess: (res) => window.location.assign(res.authorization_url),
    onError: (err) =>
      toast({
        title,
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      }),
  });
}

function RedirectLabel({ pending, idle }: { pending: boolean; idle: string }) {
  return pending ? (
    <>
      <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
      Redirecting...
    </>
  ) : (
    <>{idle}</>
  );
}

function MicrosoftStep({ initialKind }: { initialKind?: "work" | "personal" }) {
  const [kind, setKind] = useState<"work" | "personal" | null>(initialKind ?? null);
  const start = useOAuthStart("Could not start Microsoft connect");
  return (
    <>
      <DialogHeader>
        <DialogTitle className="font-display text-xl">Connect a Microsoft mailbox</DialogTitle>
        <DialogDescription>
          We'll redirect you to Microsoft to sign in and grant Mail.Read, Mail.Send, and offline access.
        </DialogDescription>
      </DialogHeader>
      <div className="grid grid-cols-2 gap-3 pt-2">
        {(["work", "personal"] as const).map((k) => (
          <button
            key={k}
            onClick={() => setKind(k)}
            className={cn(
              "rounded-lg border p-4 text-left transition",
              kind === k ? "border-foreground bg-secondary" : "border-border hover:border-foreground/40",
            )}
          >
            <p className="font-display text-base font-medium capitalize">{k}</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {k === "work" ? "Microsoft 365 / Entra (organizations)" : "Outlook.com, Hotmail, Live (consumers)"}
            </p>
          </button>
        ))}
      </div>
      <Button
        disabled={!kind || start.isPending}
        onClick={() => kind && start.mutate({ provider: "m365", kind })}
        className="mt-2 w-full bg-foreground text-background hover:bg-foreground/90"
      >
        <RedirectLabel pending={start.isPending} idle="Continue to Microsoft sign-in" />
      </Button>
    </>
  );
}

function GoogleStep() {
  const start = useOAuthStart("Could not start Google connect");
  return (
    <>
      <DialogHeader>
        <DialogTitle className="font-display text-xl">Connect a Google mailbox</DialogTitle>
        <DialogDescription>
          We'll redirect you to Google to grant read and send access to Gmail. This is separate from how you sign in
          to Automata, so you can connect several Google mailboxes.
        </DialogDescription>
      </DialogHeader>
      <p className="text-xs text-muted-foreground">
        While the Google app is limited to one Workspace organisation, mailboxes outside it will be refused at the
        consent screen. For a personal Gmail address, go back and choose Other (IMAP) with an app password.
      </p>
      <Button
        disabled={start.isPending}
        onClick={() => start.mutate({ provider: "google" })}
        className="mt-2 w-full bg-foreground text-background hover:bg-foreground/90"
      >
        <RedirectLabel pending={start.isPending} idle="Continue to Google sign-in" />
      </Button>
    </>
  );
}

const emptyServer = (port: number, security: MailServerSettings["security"]): MailServerSettings => ({
  host: "",
  port,
  security,
});

function ServerFields({
  name,
  value,
  onChange,
}: {
  name: "IMAP" | "SMTP";
  value: MailServerSettings;
  onChange: (v: MailServerSettings) => void;
}) {
  const id = name.toLowerCase();
  return (
    <div className="grid grid-cols-[1fr_5rem_7rem] gap-2">
      <div className="space-y-1">
        <Label htmlFor={`${id}-host`}>{name} server</Label>
        <Input
          id={`${id}-host`}
          value={value.host}
          onChange={(e) => onChange({ ...value, host: e.target.value })}
          placeholder={name === "IMAP" ? "imap.example.com" : "smtp.example.com"}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor={`${id}-port`}>Port</Label>
        <Input
          id={`${id}-port`}
          inputMode="numeric"
          value={String(value.port || "")}
          onChange={(e) => onChange({ ...value, port: Number(e.target.value.replace(/\D/g, "")) || 0 })}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor={`${id}-security`}>Security</Label>
        <select
          id={`${id}-security`}
          className="h-10 w-full rounded-md border border-input bg-background px-2 text-sm"
          value={value.security}
          onChange={(e) => onChange({ ...value, security: e.target.value as MailServerSettings["security"] })}
        >
          <option value="tls">SSL/TLS</option>
          <option value="starttls">STARTTLS</option>
        </select>
      </div>
    </div>
  );
}

function ImapStep({ initialEmail, onConnected }: { initialEmail?: string; onConnected: (accountID: string) => void }) {
  const { accessToken } = useAuth();
  const [email, setEmail] = useState(initialEmail ?? "");
  const [password, setPassword] = useState("");
  const [username, setUsername] = useState("");
  const [presetID, setPresetID] = useState(() => presetForEmail(initialEmail ?? "")?.id ?? "");
  const [imap, setImap] = useState<MailServerSettings>(
    () => presetForEmail(initialEmail ?? "")?.imap ?? emptyServer(993, "tls"),
  );
  const [smtp, setSmtp] = useState<MailServerSettings>(
    () => presetForEmail(initialEmail ?? "")?.smtp ?? emptyServer(465, "tls"),
  );
  const [error, setError] = useState<string | null>(null);
  const preset = mailPresets.find((p) => p.id === presetID);

  const applyPreset = (id: string) => {
    setPresetID(id);
    const p = mailPresets.find((x) => x.id === id);
    if (p) {
      setImap(p.imap);
      setSmtp(p.smtp);
    }
  };

  const connect = useMutation({
    mutationFn: async () => {
      if (!accessToken) throw new Error("Not authenticated");
      return connectImapAccount(accessToken, {
        email: email.trim(),
        username: username.trim() || undefined,
        password,
        imap: { ...imap, host: imap.host.trim() },
        smtp: { ...smtp, host: smtp.host.trim() },
      });
    },
    onSuccess: (res) => {
      setError(null);
      onConnected(res.account_id);
    },
    // The server names the cause — wrong password, unreachable host — so it
    // is shown as is, next to the fields that need fixing.
    onError: (err) => setError(err instanceof ApiError ? err.message : "Could not connect. Please try again."),
  });

  const ready = email.includes("@") && password !== "" && imap.host.trim() !== "" && smtp.host.trim() !== "";

  return (
    <>
      <DialogHeader>
        <DialogTitle className="font-display text-xl">Connect with IMAP</DialogTitle>
        <DialogDescription>
          We log in to both servers before saving anything, so a wrong password or server shows up now.
        </DialogDescription>
      </DialogHeader>
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (ready) connect.mutate();
        }}
      >
        <div className="space-y-1">
          <Label htmlFor="imap-email">Email address</Label>
          <Input
            id="imap-email"
            type="email"
            value={email}
            onChange={(e) => {
              setEmail(e.target.value);
              const found = presetForEmail(e.target.value);
              if (found && found.id !== presetID) applyPreset(found.id);
            }}
            placeholder="you@example.com"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="imap-preset">Mail provider</Label>
          <select
            id="imap-preset"
            className="h-10 w-full rounded-md border border-input bg-background px-2 text-sm"
            value={presetID}
            onChange={(e) => applyPreset(e.target.value)}
          >
            <option value="">Custom server</option>
            {mailPresets.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label htmlFor="imap-password">Password</Label>
          <Input
            id="imap-password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {preset?.passwordHint && <p className="text-xs text-muted-foreground">{preset.passwordHint}</p>}
        </div>
        {!preset && (
          <>
            <div className="space-y-1">
              <Label htmlFor="imap-username">Username (if not your email address)</Label>
              <Input id="imap-username" value={username} onChange={(e) => setUsername(e.target.value)} />
            </div>
            <ServerFields name="IMAP" value={imap} onChange={setImap} />
            <ServerFields name="SMTP" value={smtp} onChange={setSmtp} />
          </>
        )}
        {error && (
          <div role="alert" className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}
        <Button
          type="submit"
          disabled={!ready || connect.isPending}
          className="w-full bg-foreground text-background hover:bg-foreground/90"
        >
          {connect.isPending ? (
            <>
              <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
              Checking servers...
            </>
          ) : (
            "Connect mailbox"
          )}
        </Button>
      </form>
    </>
  );
}
