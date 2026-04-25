import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import type { Location as RouterLocation } from "react-router-dom";
import { ArrowRight, Loader2, Mail } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/hooks/use-toast";
import { GoogleIcon, MicrosoftIcon } from "@/components/auth/SocialIcons";
import { AuthLayout } from "@/components/auth/AuthLayout";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, startOAuthLogin } from "@/lib/auth";

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { signIn } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState<"email" | "google" | "microsoft" | null>(null);
  const redirectTo = (location.state as { from?: RouterLocation } | null)?.from?.pathname ?? "/";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading("email");
    try {
      await signIn(email, password);
      toast({
        title: "Signed in",
        description: `Welcome back, ${email}.`,
      });
      navigate(redirectTo, { replace: true });
    } catch (error) {
      toast({
        title: "Sign-in failed",
        description: error instanceof ApiError ? error.message : "Please check your details and try again.",
        variant: "destructive",
      });
    } finally {
      setLoading(null);
    }
  };

  const handleOAuth = async (provider: "google" | "microsoft") => {
    setLoading(provider);
    try {
      await startOAuthLogin(provider);
    } catch (error) {
      setLoading(null);
      toast({
        title: `${provider === "google" ? "Google" : "Microsoft"} sign-in failed`,
        description: error instanceof ApiError ? error.message : "Could not start the OAuth flow.",
        variant: "destructive",
      });
    }
  };

  return (
    <AuthLayout
      eyebrow="Welcome back"
      title="Sign in to Postern"
      subtitle="Your workspace intelligence, waiting."
    >
      <div className="space-y-3">
        <Button
          type="button"
          variant="outline"
          className="w-full justify-center gap-3 h-11"
          onClick={() => handleOAuth("google")}
          disabled={loading !== null}
        >
          {loading === "google" ? <Loader2 className="h-4 w-4 animate-spin" /> : <GoogleIcon className="h-4 w-4" />}
          Continue with Google
        </Button>
        <Button
          type="button"
          variant="outline"
          className="w-full justify-center gap-3 h-11"
          onClick={() => handleOAuth("microsoft")}
          disabled={loading !== null}
        >
          {loading === "microsoft" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <MicrosoftIcon className="h-4 w-4" />
          )}
          Continue with Microsoft
        </Button>
      </div>

      <div className="relative my-6">
        <div className="absolute inset-0 flex items-center">
          <div className="w-full border-t border-border" />
        </div>
        <div className="relative flex justify-center text-xs uppercase tracking-widest">
          <span className="bg-card px-3 text-muted-foreground">or with email</span>
        </div>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="email">Email</Label>
          <div className="relative">
            <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="email"
              type="email"
              placeholder="you@company.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              className="pl-9 h-11"
            />
          </div>
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="password">Password</Label>
            <Link to="/forgot-password" className="text-xs text-primary hover:underline">
              Forgot?
            </Link>
          </div>
          <Input
            id="password"
            type="password"
            placeholder="••••••••"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={6}
            className="h-11"
          />
        </div>

        <Button type="submit" className="w-full h-11 gap-2" disabled={loading !== null}>
          {loading === "email" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <>
              Sign in <ArrowRight className="h-4 w-4" />
            </>
          )}
        </Button>
      </form>

      <p className="mt-6 text-center text-sm text-muted-foreground">
        New to Postern?{" "}
        <Link to="/register" className="font-medium text-primary hover:underline">
          Create an account
        </Link>
      </p>
    </AuthLayout>
  );
}
