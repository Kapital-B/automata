import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/components/auth/AuthProvider";
import { readTokensFromFragment } from "@/lib/auth";

export default function AuthCallbackPage() {
  const navigate = useNavigate();
  const { completeOAuth } = useAuth();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function finishLogin() {
      const tokens = readTokensFromFragment(window.location.hash);
      if (!tokens) {
        setError("The authentication response did not include tokens.");
        return;
      }

      try {
        await completeOAuth(tokens);
        window.history.replaceState(null, document.title, window.location.pathname);
        navigate("/", { replace: true });
      } catch {
        setError("We could not complete sign-in. Please try again.");
      }
    }

    void finishLogin();
  }, [completeOAuth, navigate]);

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-4">
        <div className="surface-card max-w-md p-6 text-center">
          <h1 className="font-display text-2xl font-semibold">Sign-in failed</h1>
          <p className="mt-2 text-sm text-muted-foreground">{error}</p>
          <Button asChild className="mt-6">
            <Link to="/login">Back to sign in</Link>
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background text-muted-foreground">
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
      Completing sign-in...
    </div>
  );
}
