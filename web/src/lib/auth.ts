const API_BASE_URL = (import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080").replace(/\/$/, "");

const ACCESS_TOKEN_KEY = "postern.access_token";
const REFRESH_TOKEN_KEY = "postern.refresh_token";

export type AuthTokens = {
  accessToken: string;
  refreshToken: string;
};

export type AuthUser = {
  userId: string;
  email: string;
};

type TokenResponse = {
  access_token: string;
  refresh_token: string;
  user_id?: string;
};

type MeResponse = {
  user_id: string;
  email: string;
};

type OAuthStartResponse = {
  authorization_url: string;
};

export class ApiError extends Error {
  status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function parseApiError(response: Response) {
  let message = response.statusText || "Request failed";
  try {
    const body = (await response.json()) as { error?: string };
    if (body.error) {
      message = body.error;
    }
  } catch {
    // Keep the HTTP status text when the response is not JSON.
  }
  throw new ApiError(message, response.status);
}

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers,
    },
  });

  if (!response.ok) {
    await parseApiError(response);
  }

  return response.json() as Promise<T>;
}

export function getStoredTokens(): AuthTokens | null {
  const accessToken = window.localStorage.getItem(ACCESS_TOKEN_KEY);
  const refreshToken = window.localStorage.getItem(REFRESH_TOKEN_KEY);
  if (!accessToken || !refreshToken) {
    return null;
  }
  return { accessToken, refreshToken };
}

export function storeTokens(tokens: AuthTokens) {
  window.localStorage.setItem(ACCESS_TOKEN_KEY, tokens.accessToken);
  window.localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refreshToken);
}

export function clearTokens() {
  window.localStorage.removeItem(ACCESS_TOKEN_KEY);
  window.localStorage.removeItem(REFRESH_TOKEN_KEY);
}

function normalizeTokens(response: TokenResponse): AuthTokens {
  return {
    accessToken: response.access_token,
    refreshToken: response.refresh_token,
  };
}

export async function registerWithPassword(email: string, password: string) {
  const response = await apiRequest<TokenResponse>("/api/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  return normalizeTokens(response);
}

export async function loginWithPassword(email: string, password: string) {
  const response = await apiRequest<TokenResponse>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
  return normalizeTokens(response);
}

export async function refreshAuthTokens(refreshToken: string) {
  const response = await apiRequest<TokenResponse>("/api/auth/refresh", {
    method: "POST",
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  return normalizeTokens(response);
}

export async function fetchCurrentUser(accessToken: string): Promise<AuthUser> {
  const response = await apiRequest<MeResponse>("/api/me", {
    headers: {
      Authorization: `Bearer ${accessToken}`,
    },
  });
  return {
    userId: response.user_id,
    email: response.email,
  };
}

export async function startOAuthLogin(provider: "google" | "microsoft") {
  const response = await apiRequest<OAuthStartResponse>(`/api/auth/${provider}`);
  window.location.assign(response.authorization_url);
}

export function readTokensFromFragment(hash: string): AuthTokens | null {
  const params = new URLSearchParams(hash.replace(/^#/, ""));
  const accessToken = params.get("access_token");
  const refreshToken = params.get("refresh_token");
  if (!accessToken || !refreshToken) {
    return null;
  }
  return { accessToken, refreshToken };
}
