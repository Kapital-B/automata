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
  messages_upserted?: number;
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
  definition: string;
  sort_order: number;
};

export type SummarySnapshot = {
  id: string;
  account_id?: string;
  run_id: string;
  window_start: string;
  window_end: string;
  general_summary: string;
  created_at: string;
};

export type SummaryActionItem = {
  id: string;
  account_id: string;
  message_id: string;
  text: string;
  due_at?: string;
  is_overdue: boolean;
};

export type SummaryFYI = {
  id: string;
  account_id: string;
  message_id: string;
  text: string;
};

export type SummaryPayload = {
  snapshot: SummarySnapshot | null;
  action_items: SummaryActionItem[];
  fyi: SummaryFYI[];
};

export type SummarySettings = {
  include_category_slugs: string[];
  exclude_category_slugs: string[];
  chunk_size: number;
  updated_at?: string;
};

export type ScheduleChain = {
  id: string;
  name: string;
  account_id?: string;
  jobs: string[];
  interval_minutes: number;
  enabled: boolean;
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

export type DraftSuggestion = {
  id: string;
  account_id: string;
  message_id: string;
  action_item_id: string;
  run_id: string;
  subject: string;
  body: string;
  model: string;
  status?: "ready" | "sent" | "discarded";
  to_name: string;
  to_email: string;
  created_at: string;
};

export type DraftSendAttempt = {
  id: string;
  draft_id: string;
  account_id: string;
  message_id: string;
  status: "success" | "failed";
  provider_message_id?: string;
  error_message?: string;
  created_at: string;
};

export type ForwardRule = {
  id: string;
  account_id: string;
  name: string;
  mode: "logic" | "llm";
  condition_json: Record<string, unknown>;
  forward_to: string;
  enabled: boolean;
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

export async function getRun(accessToken: string, id: string) {
  return apiRequest<JobRun>(`/api/runs/${id}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function listCategories(accessToken: string) {
  return apiRequest<CategoryDefinition[]>("/api/categories", {
    headers: toAuthHeader(accessToken),
  });
}

export async function getSummary(accessToken: string, accountID?: string) {
  const suffix = accountID ? `?account_id=${encodeURIComponent(accountID)}` : "";
  return apiRequest<SummaryPayload>(`/api/summaries${suffix}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function refreshSummary(accessToken: string, accountID: string) {
  return apiRequest<{ job_run_id: string; status: string }>(`/api/accounts/${accountID}/summaries/refresh`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
}

export async function markActionItemDone(accessToken: string, itemID: string) {
  return apiRequest<{ status: string }>(`/api/action-items/${itemID}/done`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
}

export async function dismissFYI(accessToken: string, fyiID: string) {
  return apiRequest<{ status: string }>(`/api/fyi/${fyiID}/dismiss`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
}

export async function getSummarySettings(accessToken: string) {
  return apiRequest<SummarySettings>("/api/settings/summaries", {
    headers: toAuthHeader(accessToken),
  });
}

export async function updateSummarySettings(accessToken: string, payload: SummarySettings) {
  return apiRequest<{ status: string }>("/api/settings/summaries", {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(payload),
  });
}

export async function getScheduleSettings(accessToken: string) {
  return apiRequest<{ chains: ScheduleChain[] }>("/api/settings/schedules", {
    headers: toAuthHeader(accessToken),
  });
}

export async function updateScheduleSettings(accessToken: string, chains: ScheduleChain[]) {
  return apiRequest<{ status: string }>("/api/settings/schedules", {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify({ chains }),
  });
}

export type UpsertCategoryInput = {
  slug: string;
  display_name: string;
  definition: string;
  sort_order: number;
};

export async function createCategory(accessToken: string, input: UpsertCategoryInput) {
  return apiRequest<CategoryDefinition>("/api/categories", {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(input),
  });
}

export async function updateCategory(accessToken: string, id: string, input: UpsertCategoryInput) {
  return apiRequest<CategoryDefinition>(`/api/categories/${id}`, {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(input),
  });
}

export async function deleteCategory(accessToken: string, id: string, replacementID?: string) {
  const suffix = replacementID ? `?replacement_id=${encodeURIComponent(replacementID)}` : "";
  return apiRequest<{ status: string }>(`/api/categories/${id}${suffix}`, {
    method: "DELETE",
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

export async function listDraftSuggestions(accessToken: string, accountID?: string) {
  const suffix = accountID ? `?account_id=${encodeURIComponent(accountID)}` : "";
  return apiRequest<DraftSuggestion[]>(`/api/drafts${suffix}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function listDraftSendAttempts(accessToken: string, draftID: string) {
  return apiRequest<DraftSendAttempt[]>(`/api/drafts/${draftID}/attempts`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function saveDraftSuggestion(
  accessToken: string,
  draftID: string,
  payload: { subject: string; body: string },
) {
  return apiRequest<{ ok: boolean }>(`/api/drafts/${draftID}`, {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(payload),
  });
}

export async function sendDraftSuggestion(accessToken: string, draftID: string) {
  return apiRequest<{ ok: boolean }>(`/api/drafts/${draftID}/send`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
}

export async function discardDraftSuggestion(accessToken: string, draftID: string) {
  const response = await fetch(`${API_BASE_URL}/api/drafts/${draftID}`, {
    method: "DELETE",
    headers: toAuthHeader(accessToken),
  });
  if (!response.ok) {
    await parseApiError(response);
  }
}

export async function categorizeAccount(
  accessToken: string,
  accountID: string,
  options: { recategorize?: boolean } = {},
) {
  return apiRequest<{ job_run_id: string; messages_categorized?: number; recategorize?: boolean; status: string }>(
    `/api/accounts/${accountID}/categorize`,
    {
      method: "POST",
      headers: toAuthHeader(accessToken),
      body: JSON.stringify({ recategorize: Boolean(options.recategorize) }),
    },
  );
}

export async function getForwardAllowlist(accessToken: string) {
  return apiRequest<{ emails: string[] }>("/api/forward-allowlist", {
    headers: toAuthHeader(accessToken),
  });
}

export async function putForwardAllowlist(accessToken: string, emails: string[]) {
  return apiRequest<{ status: string }>("/api/forward-allowlist", {
    method: "PUT",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify({ emails }),
  });
}

export async function listForwardRules(accessToken: string, accountID: string) {
  return apiRequest<ForwardRule[]>(`/api/accounts/${accountID}/forward-rules`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function createForwardRule(
  accessToken: string,
  accountID: string,
  payload: Omit<ForwardRule, "id" | "account_id">,
) {
  return apiRequest<{ id: string }>(`/api/accounts/${accountID}/forward-rules`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(payload),
  });
}

export async function updateForwardRule(
  accessToken: string,
  ruleID: string,
  payload: Omit<ForwardRule, "id" | "account_id">,
) {
  return apiRequest<{ status: string }>(`/api/forward-rules/${ruleID}`, {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(payload),
  });
}

export async function deleteForwardRule(accessToken: string, ruleID: string) {
  const response = await fetch(`${API_BASE_URL}/api/forward-rules/${ruleID}`, {
    method: "DELETE",
    headers: toAuthHeader(accessToken),
  });
  if (!response.ok) {
    await parseApiError(response);
  }
}

export async function runForwardRules(accessToken: string, accountID: string) {
  return apiRequest<{ job_run_id: string; status: string }>(`/api/accounts/${accountID}/forward-rules/run`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
  });
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
