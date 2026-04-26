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

export type MailAccount = {
  id: string;
  label: string;
  provider: string;
  ms_account_kind: "work" | "personal" | "common";
  primary_email: string;
  connection_status: "connected" | "error" | "expired";
  last_error?: string;
  last_synced_at?: string;
};

export type SyncAccountResponse = {
  job_run_id: string;
  messages_upserted: number;
  status: string;
};

export type JobRun = {
  id: string;
  account_id?: string;
  account_label?: string;
  job_type: string;
  trigger: "api" | "schedule";
  status: "pending" | "running" | "success" | "failed" | "cancelled";
  time_window_start?: string;
  time_window_end?: string;
  started_at: string;
  finished_at?: string;
  error_message?: string;
  meta_json: Record<string, unknown>;
};

export type ListRunsFilter = {
  accountId?: string;
  jobType?: string;
  limit?: number;
  offset?: number;
};

export type CategoryDefinition = {
  id: string;
  slug: string;
  display_name: string;
  sort_order: number;
};

export type MessageItem = {
  id: string;
  account_id: string;
  provider_message_id: string;
  subject: string;
  received_at: string;
  has_attachments: boolean;
  from_json: { name?: string; address?: string };
  body_text?: string;
  preview: string;
  category_slug?: string;
  category_confidence?: number;
};

export type ListMessagesFilter = {
  accountId?: string;
  category?: string;
  since?: string;
  limit?: number;
  offset?: number;
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

export async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
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

function toAuthHeader(accessToken: string) {
  return {
    Authorization: `Bearer ${accessToken}`,
  };
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
    headers: toAuthHeader(accessToken),
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

export async function listAccounts(accessToken: string) {
  return apiRequest<MailAccount[]>("/api/accounts", {
    headers: toAuthHeader(accessToken),
  });
}

export async function startMailboxConnect(accessToken: string, kind: "work" | "personal") {
  return apiRequest<OAuthStartResponse>("/api/accounts", {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify({ provider: "m365", ms_account_kind: kind }),
  });
}

export async function deleteAccount(accessToken: string, accountID: string) {
  const response = await fetch(`${API_BASE_URL}/api/accounts/${accountID}`, {
    method: "DELETE",
    headers: {
      "Content-Type": "application/json",
      ...toAuthHeader(accessToken),
    },
  });
  if (!response.ok) {
    await parseApiError(response);
  }
}

export async function syncAccount(accessToken: string, accountID: string) {
  return apiRequest<SyncAccountResponse>(`/api/accounts/${accountID}/sync`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
}

export async function listRuns(accessToken: string, filter: ListRunsFilter = {}) {
  const params = new URLSearchParams();
  if (filter.accountId) {
    params.set("account_id", filter.accountId);
  }
  if (filter.jobType) {
    params.set("job_type", filter.jobType);
  }
  if (typeof filter.limit === "number") {
    params.set("limit", String(filter.limit));
  }
  if (typeof filter.offset === "number") {
    params.set("offset", String(filter.offset));
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return apiRequest<JobRun[]>(`/api/runs${suffix}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function listCategories(accessToken: string) {
  return apiRequest<CategoryDefinition[]>("/api/categories", {
    headers: toAuthHeader(accessToken),
  });
}

export async function listMessages(accessToken: string, filter: ListMessagesFilter = {}) {
  const params = new URLSearchParams();
  if (filter.accountId) {
    params.set("account_id", filter.accountId);
  }
  if (filter.category) {
    params.set("category", filter.category);
  }
  if (filter.since) {
    params.set("since", filter.since);
  }
  if (typeof filter.limit === "number") {
    params.set("limit", String(filter.limit));
  }
  if (typeof filter.offset === "number") {
    params.set("offset", String(filter.offset));
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return apiRequest<MessageItem[]>(`/api/messages${suffix}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function getMessage(accessToken: string, id: string) {
  return apiRequest<MessageItem>(`/api/messages/${id}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function categorizeAccount(
  accessToken: string,
  accountID: string,
  options: { recategorize?: boolean } = {},
) {
  return apiRequest<{ job_run_id: string; messages_categorized: number; recategorize?: boolean; status: string }>(
    `/api/accounts/${accountID}/categorize`,
    {
      method: "POST",
      headers: toAuthHeader(accessToken),
      body: JSON.stringify({ recategorize: Boolean(options.recategorize) }),
    },
  );
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
