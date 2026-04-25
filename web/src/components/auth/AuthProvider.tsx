import { createContext, ReactNode, useCallback, useContext, useEffect, useMemo, useState } from "react";
import {
  AuthTokens,
  AuthUser,
  clearTokens,
  fetchCurrentUser,
  getStoredTokens,
  loginWithPassword,
  refreshAuthTokens,
  registerWithPassword,
  storeTokens,
} from "@/lib/auth";

type AuthContextValue = {
  accessToken: string | null;
  refreshToken: string | null;
  user: AuthUser | null;
  initializing: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  completeOAuth: (tokens: AuthTokens) => Promise<void>;
  signOut: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [tokens, setTokens] = useState<AuthTokens | null>(() => getStoredTokens());
  const [user, setUser] = useState<AuthUser | null>(null);
  const [initializing, setInitializing] = useState(true);

  const persistTokens = useCallback((nextTokens: AuthTokens) => {
    storeTokens(nextTokens);
    setTokens(nextTokens);
  }, []);

  const signOut = useCallback(() => {
    clearTokens();
    setTokens(null);
    setUser(null);
  }, []);

  const hydrateUser = useCallback(
    async (activeTokens: AuthTokens) => {
      try {
        const nextUser = await fetchCurrentUser(activeTokens.accessToken);
        setUser(nextUser);
      } catch (error) {
        try {
          const rotated = await refreshAuthTokens(activeTokens.refreshToken);
          persistTokens(rotated);
          const nextUser = await fetchCurrentUser(rotated.accessToken);
          setUser(nextUser);
        } catch {
          signOut();
          throw error;
        }
      }
    },
    [persistTokens, signOut],
  );

  useEffect(() => {
    let cancelled = false;

    async function load() {
      if (!tokens) {
        setInitializing(false);
        return;
      }
      try {
        await hydrateUser(tokens);
      } catch {
        // The user will be redirected by protected routes when auth cannot be restored.
      } finally {
        if (!cancelled) {
          setInitializing(false);
        }
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, [hydrateUser, tokens]);

  const signIn = useCallback(
    async (email: string, password: string) => {
      const nextTokens = await loginWithPassword(email, password);
      persistTokens(nextTokens);
      await hydrateUser(nextTokens);
    },
    [hydrateUser, persistTokens],
  );

  const register = useCallback(
    async (email: string, password: string) => {
      const nextTokens = await registerWithPassword(email, password);
      persistTokens(nextTokens);
      await hydrateUser(nextTokens);
    },
    [hydrateUser, persistTokens],
  );

  const completeOAuth = useCallback(
    async (nextTokens: AuthTokens) => {
      persistTokens(nextTokens);
      await hydrateUser(nextTokens);
    },
    [hydrateUser, persistTokens],
  );

  const value = useMemo<AuthContextValue>(
    () => ({
      accessToken: tokens?.accessToken ?? null,
      refreshToken: tokens?.refreshToken ?? null,
      user,
      initializing,
      signIn,
      register,
      completeOAuth,
      signOut,
    }),
    [completeOAuth, initializing, register, signIn, signOut, tokens, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return context;
}
