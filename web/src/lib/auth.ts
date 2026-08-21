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
  homeOrganisationId?: string;
};

type TokenResponse = {
  access_token: string;
  refresh_token: string;
  user_id?: string;
};

type MeResponse = {
  user_id: string;
  email: string;
  home_organisation_id?: string;
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
  created_at: string;
  due_at?: string;
  is_overdue: boolean;
};

export type SummaryFYI = {
  id: string;
  account_id: string;
  message_id: string;
  text: string;
  created_at: string;
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
  conversation_id?: string;
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
    homeOrganisationId: response.home_organisation_id,
  };
}

export type ContactListItem = {
  id: string;
  organisation_id: string;
  display_name: string;
  company?: string;
  created_at: string;
  updated_at: string;
};

export type ContactIdentity = {
  id: string;
  kind: string;
  value_normalized: string;
  value_raw: string;
  created_at: string;
};

export type ContactDetail = ContactListItem & {
  identities: ContactIdentity[];
  recent_messages: { message_id: string; account_id: string }[];
  suggested_merges: { id: string; display_name: string }[];
};

export async function listContacts(
  accessToken: string,
  opts?: { q?: string; limit?: number; offset?: number },
) {
  const params = new URLSearchParams();
  if (opts?.q) params.set("q", opts.q);
  if (typeof opts?.limit === "number") params.set("limit", String(opts.limit));
  if (typeof opts?.offset === "number") params.set("offset", String(opts.offset));
  const qs = params.toString();
  return apiRequest<ContactListItem[]>(`/api/contacts${qs ? `?${qs}` : ""}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function getContact(accessToken: string, contactID: string) {
  return apiRequest<ContactDetail>(`/api/contacts/${contactID}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function mergeContacts(
  accessToken: string,
  survivorID: string,
  sourceContactID: string,
) {
  return apiRequest<{ ok: boolean }>(`/api/contacts/${survivorID}/merge`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify({ source_contact_id: sourceContactID }),
  });
}

export type ProjectListItem = {
  id: string;
  organisation_id: string;
  name: string;
  code: string;
  description?: string;
  client?: string;
  keywords: string[];
  archived_at?: string;
  created_at: string;
  updated_at: string;
};

export type ProjectMember = {
  id: string;
  project_id: string;
  user_id: string;
  role: string;
  discipline?: string;
  responsibilities?: string;
  current_scope?: string;
  approval_authority?: string;
  out_of_scope?: string;
  created_at: string;
  updated_at: string;
};

export type ProjectDetail = ProjectListItem & {
  member?: ProjectMember;
};

export type UnassignedSummary = {
  unassigned: number;
  provisional: number;
};

export type UnassignedItem = {
  kind: "message" | "manual";
  message_id?: string;
  manual_item_id?: string;
  account_id?: string;
  account_label?: string;
  subject?: string;
  title?: string;
  channel?: string;
  from_json?: { name?: string; address?: string };
  conversation_id?: string;
  received_at?: string;
  occurred_at?: string;
  status: "unassigned" | "provisional";
  reason?: string;
  source?: string;
  project_id?: string;
};

export type TimelineContact = {
  id: string;
  display_name: string;
  role: string;
};

export type TimelineItem = {
  source: "mail" | "manual";
  occurred_at: string;
  title: string;
  snippet: string;
  contacts: TimelineContact[];
  account_id?: string;
  account_label?: string;
  message_id?: string;
  manual_item_id?: string;
  channel?: string;
  body_text?: string;
};

export type ManualItem = {
  id: string;
  organisation_id: string;
  channel: string;
  occurred_at: string;
  title: string;
  body_text: string;
  project_id?: string;
  assignment_status: string;
  created_at: string;
};

export type EffectiveAssignment = {
  status: string;
  reason?: string;
  source?: string;
  scope?: string;
  account_id?: string;
  message_id?: string;
  project_id?: string;
  conversation_id?: string;
};

export async function listProjects(accessToken: string, includeArchived = false) {
  const qs = includeArchived ? "?include_archived=true" : "";
  return apiRequest<ProjectListItem[]>(`/api/projects${qs}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function getProject(accessToken: string, projectID: string) {
  return apiRequest<ProjectDetail>(`/api/projects/${projectID}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function createProject(
  accessToken: string,
  body: { name: string; code: string; description?: string; keywords?: string[] },
) {
  return apiRequest<ProjectListItem>("/api/projects", {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
}

export async function updateProject(
  accessToken: string,
  projectID: string,
  body: Record<string, unknown>,
) {
  return apiRequest<ProjectListItem>(`/api/projects/${projectID}`, {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
}

export async function updateProjectMember(
  accessToken: string,
  projectID: string,
  body: Record<string, unknown>,
) {
  return apiRequest<ProjectMember>(`/api/projects/${projectID}/member`, {
    method: "PATCH",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
}

export async function getUnassignedSummary(accessToken: string) {
  return apiRequest<UnassignedSummary>("/api/unassigned/summary", {
    headers: toAuthHeader(accessToken),
  });
}

export async function listUnassigned(
  accessToken: string,
  opts?: { status?: string; limit?: number; offset?: number },
) {
  const params = new URLSearchParams();
  if (opts?.status) params.set("status", opts.status);
  if (typeof opts?.limit === "number") params.set("limit", String(opts.limit));
  if (typeof opts?.offset === "number") params.set("offset", String(opts.offset));
  const qs = params.toString();
  return apiRequest<UnassignedItem[]>(`/api/unassigned${qs ? `?${qs}` : ""}`, {
    headers: toAuthHeader(accessToken),
  });
}

export async function assignMessageProject(
  accessToken: string,
  messageID: string,
  body: { project_id?: string | null; scope?: "thread" | "message"; status?: string },
) {
  return apiRequest<EffectiveAssignment>(`/api/messages/${messageID}/project-assignment`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
}

export async function clearMessageProjectOverride(accessToken: string, messageID: string) {
  return apiRequest<EffectiveAssignment>(
    `/api/messages/${messageID}/project-assignment/override`,
    {
      method: "DELETE",
      headers: toAuthHeader(accessToken),
    },
  );
}

export async function getProjectTimeline(
  accessToken: string,
  projectID: string,
  opts?: { source?: string; limit?: number; offset?: number },
) {
  const params = new URLSearchParams();
  if (opts?.source) params.set("source", opts.source);
  if (typeof opts?.limit === "number") params.set("limit", String(opts.limit));
  if (typeof opts?.offset === "number") params.set("offset", String(opts.offset));
  const qs = params.toString();
  return apiRequest<TimelineItem[]>(
    `/api/projects/${projectID}/timeline${qs ? `?${qs}` : ""}`,
    { headers: toAuthHeader(accessToken) },
  );
}

export async function createManualItem(
  accessToken: string,
  body: {
    channel: string;
    occurred_at: string;
    title: string;
    body_text: string;
    project_id?: string;
    participant_contact_ids?: string[];
  },
) {
  return apiRequest<ManualItem>("/api/manual-items", {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
}

export async function assignManualItem(
  accessToken: string,
  manualItemID: string,
  body: { project_id?: string | null },
) {
  return apiRequest<ManualItem>(`/api/manual-items/${manualItemID}/project-assignment`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(body),
  });
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

export async function generateDraftSuggestions(
  accessToken: string,
  accountID: string,
  options?: { messageId?: string },
) {
  const body = options?.messageId ? JSON.stringify({ message_id: options.messageId }) : undefined;
  return apiRequest<{ job_run_id: string; status: string; drafts_generated?: number; action_items_seen?: number }>(
    `/api/accounts/${accountID}/drafts/generate`,
    {
      method: "POST",
      headers: toAuthHeader(accessToken),
      body,
    },
  );
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

export async function forwardMessage(accessToken: string, messageID: string, payload: { to_email: string; comment?: string }) {
  return apiRequest<{ status: string }>(`/api/messages/${messageID}/forward`, {
    method: "POST",
    headers: toAuthHeader(accessToken),
    body: JSON.stringify(payload),
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
