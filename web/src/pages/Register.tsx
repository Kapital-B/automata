import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ArrowRight, Loader2, Mail, User } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "@/hooks/use-toast";
import { GoogleIcon, MicrosoftIcon } from "@/components/auth/SocialIcons";
import { AuthLayout } from "@/components/auth/AuthLayout";
import { useAuth } from "@/components/auth/AuthProvider";
import { ApiError, startOAuthLogin } from "@/lib/auth";

export default function RegisterPage() {
  const navigate = useNavigate();
  const { register } = useAuth();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState<"email" | "google" | "microsoft" | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading("email");
    try {
      await register(email, password);
      toast({
        title: "Account created",
        description: `Welcome to Postern, ${name || email}.`,
      });
      navigate("/", { replace: true });
    } catch (error) {
      toast({
        title: "Registration failed",
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
        title: `${provider === "google" ? "Google" : "Microsoft"} sign-up failed`,
        description: error instanceof ApiError ? error.message : "Could not start the OAuth flow.",
        variant: "destructive",
      });
    }
  };

  return (
    <AuthLayout
      eyebrow="Get started"
      title="Create your workspace"
      subtitle="Connect your tools. Let the assistant do the rest."
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
          Sign up with Google
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
          Sign up with Microsoft
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
          <Label htmlFor="name">Full name</Label>
          <div className="relative">
            <User className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="name"
              type="text"
              placeholder="Ada Lovelace"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="pl-9 h-11"
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="email">Work email</Label>
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
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            placeholder="At least 8 characters"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={8}
            className="h-11"
          />
          <p className="text-xs text-muted-foreground">
            Use 8+ characters with a mix of letters, numbers and symbols.
          </p>
        </div>

        <Button type="submit" className="w-full h-11 gap-2" disabled={loading !== null}>
          {loading === "email" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <>
              Create account <ArrowRight className="h-4 w-4" />
            </>
          )}
        </Button>

        <p className="text-center text-xs text-muted-foreground">
          By creating an account you agree to our{" "}
          <a href="#" className="underline hover:text-foreground">Terms</a> and{" "}
          <a href="#" className="underline hover:text-foreground">Privacy Policy</a>.
        </p>
      </form>

      <p className="mt-6 text-center text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link to="/login" className="font-medium text-primary hover:underline">
          Sign in
        </Link>
      </p>
    </AuthLayout>
  );
}
