import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { AccountFilter } from "@/components/AppShell";
import { useAuth } from "@/components/auth/AuthProvider";
import { useAccountsData } from "@/hooks/useAccountsData";
import {
  type DraftSuggestion,
  type JobRun,
  type SummaryActionItem,
  type SummaryFYI,
  type SummaryPayload,
  getSummary,
  getUnassignedSummary,
  listDraftSuggestions,
  listRuns,
} from "@/lib/auth";
import type { UiAccount } from "@/lib/accounts";

export type AssistantSuggestion = {
  id: string;
  title: string;
  description: string;
  href?: string;
};

export type AssistantHomeState = {
  activeAccountID?: string;
  accounts: UiAccount[];
  connectedAccounts: UiAccount[];
  erroredAccounts: UiAccount[];
  summary: SummaryPayload | null;
  actionItems: SummaryActionItem[];
  fyi: SummaryFYI[];
  draftSuggestions?: DraftSuggestion[];
  draftsReady?: number;
  runs?: JobRun[];
  latestRun?: JobRun;
  failedRuns: JobRun[];
  unassignedCount?: number;
  suggestions: AssistantSuggestion[];
};

export function buildAssistantSuggestions(state: AssistantHomeState): AssistantSuggestion[] {
  if (state.connectedAccounts.length === 0) {
    return [
      {
        id: "connect-first-account",
        title: "Connect your first Microsoft account",
        description: "Add Work or Personal Outlook in Accounts.",
        href: "/accounts",
      },
    ];
  }

  const out: AssistantSuggestion[] = [];
  if (state.actionItems.length > 0) {
    out.push({
      id: "review-action-items",
      title: `Review ${state.actionItems.length} open action item${state.actionItems.length === 1 ? "" : "s"}`,
      description: "See the full list on Home.",
      href: "/",
    });
    out.push({
      id: "open-source-messages",
      title: "Open source messages in Inbox",
      description: "Jump to the messages behind your current action items.",
      href: "/inbox",
    });
  }

  if ((state.unassignedCount ?? 0) > 0) {
    out.push({
      id: "work-unassigned",
      title: `Clear ${state.unassignedCount} unassigned item${state.unassignedCount === 1 ? "" : "s"}`,
      description: "Assign mail and pastes to a project.",
      href: "/triage",
    });
  }

  if (state.actionItems.length === 0 && (state.unassignedCount ?? 0) === 0) {
    if (state.fyi.length > 0) {
      out.push({
        id: "review-fyi",
        title: `Review ${state.fyi.length} FYI item${state.fyi.length === 1 ? "" : "s"}`,
        description: "Catch up on awareness items.",
        href: "/",
      });
    } else if (!state.summary?.snapshot) {
      out.push({
        id: "refresh-summary",
        title: "Refresh today's summary",
        description: "Generate a new summary snapshot.",
        href: "/",
      });
    }
  }

  if ((state.draftsReady ?? 0) > 0) {
    out.push({
      id: "review-drafts",
      title: `Review ${state.draftsReady} ready draft${state.draftsReady === 1 ? "" : "s"}`,
      description: "Open Drafts to edit or send.",
      href: "/drafts",
    });
  }

  if (state.failedRuns.length > 0) {
    out.push({
      id: "inspect-failed-runs",
      title: "Inspect failed runs",
      description: "Check run errors and retry from Runs.",
      href: "/runs",
    });
  }

  if (out.length === 0) {
    out.push(
      {
        id: "open-inbox",
        title: "Open Inbox",
        description: "Review recent messages by account.",
        href: "/inbox",
      },
      {
        id: "open-settings",
        title: "Manage categories and schedules",
        description: "Tune categorization and automation settings.",
        href: "/settings",
      },
    );
  }
  return out.slice(0, 6);
}

export function useAssistantHomeData(accountFilter: AccountFilter) {
  const { accessToken } = useAuth();
  const accountsQuery = useAccountsData();
  const accounts = accountsQuery.accounts;
  const activeAccountID = accountFilter === "all" ? undefined : accountFilter;
  const connectedAccounts = useMemo(
    () => accounts.filter((a) => a.status === "connected"),
    [accounts],
  );
  const erroredAccounts = useMemo(
    () => accounts.filter((a) => a.status !== "connected"),
    [accounts],
  );
  const canLoadSecondary = Boolean(accessToken) && connectedAccounts.length > 0;

  const summaryQuery = useQuery({
    queryKey: ["summary", accessToken, activeAccountID],
    queryFn: () => getSummary(accessToken!, activeAccountID),
    enabled: canLoadSecondary,
  });

  const draftsScope = activeAccountID ?? "all";
  const draftsQuery = useQuery({
    queryKey: ["draft-suggestions", accessToken, draftsScope],
    queryFn: () => listDraftSuggestions(accessToken!, activeAccountID),
    enabled: canLoadSecondary,
  });

  const runsQuery = useQuery({
    queryKey: ["assistant-home", "runs", accessToken, activeAccountID],
    queryFn: () => listRuns(accessToken!, { accountId: activeAccountID, limit: 10 }),
    enabled: canLoadSecondary,
  });

  const unassignedQuery = useQuery({
    queryKey: ["unassigned-summary", accessToken],
    queryFn: () => getUnassignedSummary(accessToken!),
    enabled: Boolean(accessToken),
  });

  const state = useMemo<AssistantHomeState>(() => {
    const summary = summaryQuery.data ?? null;
    const actionItems = summary?.action_items ?? [];
    const fyi = summary?.fyi ?? [];
    const runs = runsQuery.data;
    const failedRuns = (runs ?? []).filter((r) => r.status === "failed");
    const latestRun = (runs ?? [])[0];
    const unassignedCount =
      (unassignedQuery.data?.unassigned ?? 0) + (unassignedQuery.data?.provisional ?? 0);

    const base: AssistantHomeState = {
      activeAccountID,
      accounts,
      connectedAccounts,
      erroredAccounts,
      summary,
      actionItems,
      fyi,
      draftSuggestions: draftsQuery.data,
      draftsReady: draftsQuery.data?.length,
      runs,
      latestRun,
      failedRuns,
      unassignedCount,
      suggestions: [],
    };
    return { ...base, suggestions: buildAssistantSuggestions(base) };
  }, [
    activeAccountID,
    accounts,
    connectedAccounts,
    erroredAccounts,
    summaryQuery.data,
    draftsQuery.data,
    runsQuery.data,
    unassignedQuery.data,
  ]);

  return {
    ...state,
    isLoading:
      accountsQuery.isPending || (canLoadSecondary && summaryQuery.isPending),
    accountsError: accountsQuery.error,
    summaryError: summaryQuery.error,
    draftsError: draftsQuery.error,
    runsError: runsQuery.error,
  };
}
