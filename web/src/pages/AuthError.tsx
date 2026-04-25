import { Link, useSearchParams } from "react-router-dom";
import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";

const errorMessages: Record<string, string> = {
  access_denied: "You cancelled sign-in or did not grant access.",
  invalid_state: "The sign-in request expired or could not be verified.",
  token_exchange_failed: "The provider accepted sign-in, but the API could not exchange the code for tokens.",
};

export default function AuthErrorPage() {
  const [searchParams] = useSearchParams();
  const code = searchParams.get("code") ?? "unknown";
  const message = errorMessages[code] ?? `Authentication failed (${code}).`;

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="surface-card max-w-md p-6 text-center">
        <div className="mx-auto grid h-10 w-10 place-items-center rounded-full bg-destructive/10 text-destructive">
          <AlertTriangle className="h-5 w-5" />
        </div>
        <h1 className="mt-4 font-display text-2xl font-semibold">Could not sign in</h1>
        <p className="mt-2 text-sm text-muted-foreground">{message}</p>
        <Button asChild className="mt-6">
          <Link to="/login">Try again</Link>
        </Button>
      </div>
    </div>
  );
}
