import { Link, useSearchParams } from "react-router-dom";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";

const errorMessages: Record<string, string> = {
  access_denied: "You cancelled sign-in or denied consent in Microsoft.",
  admin_consent_required: "Your Microsoft tenant requires admin consent before this app can connect mail.",
  invalid_state: "The connect request expired or could not be validated. Please try again.",
  token_exchange_failed: "Microsoft returned to the app, but token exchange failed.",
};

export default function AccountsErrorPage() {
  const [searchParams] = useSearchParams();
  const code = searchParams.get("code") ?? "unknown";
  const message = errorMessages[code] ?? `Could not connect account (${code}).`;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="surface-card max-w-md p-6 text-center">
        <div className="mx-auto grid h-10 w-10 place-items-center rounded-full bg-destructive/15 text-destructive">
          <AlertTriangle className="h-5 w-5" />
        </div>
        <h1 className="mt-4 font-display text-2xl font-semibold">Connection failed</h1>
        <p className="mt-2 text-sm text-muted-foreground">{message}</p>
        <Button asChild className="mt-6">
          <Link to="/accounts">Back to Accounts</Link>
        </Button>
      </div>
    </div>
  );
}
