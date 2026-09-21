import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/components/auth/AuthProvider";
import { toast } from "@/hooks/use-toast";
import {
  ApiError,
  extractProject,
  getApiHealth,
  getCurrentPosition,
  getProject,
  getProjectAttention,
  getProjectTimeline,
  listProjectContradictions,
  listProjectDecisions,
  listProjectFacts,
  listProjectIssues,
  type Contradiction,
  type Decision,
  type FactDetail,
  type FactVersion,
  type IssueListItem,
} from "@/lib/auth";

export type TimelineFilters = {
  source: "all" | "mail" | "manual" | "slack";
  unassignedToIssue: boolean;
};

/**
 * A single row in the "Needs your confirmation" panel (spec §8.2). Proposed
 * fact versions, proposed decisions and open contradictions are different
 * objects with the same job, so the panel flattens them into one shape:
 * what it is, what it would change, and what it came from.
 */
export type ConfirmationRow = {
  id: string;
  kind: "fact" | "decision" | "contradiction";
  /** What it is, in the operator's words. */
  what: string;
  /** What confirming it would change. */
  change: string;
  evidenceCount: number;
  source: string;
  fact?: FactDetail;
  version?: FactVersion;
  activeVersionID?: string;
  decision?: Decision;
  contradiction?: Contradiction;
};

/**
 * How long "Reviewing…" can persist before the run is called stalled. Without
 * a ceiling a chain that dies leaves a spinner forever — but falling silently
 * back to the watermark is worse, because a stuck pipeline then looks exactly
 * like an idle one. Past this the status line says so.
 */
const REVIEWING_CEILING_MS = 2 * 60 * 1000;

function buildConfirmationRows(
  facts: FactDetail[],
  decisions: Decision[],
  contradictions: Contradiction[],
): ConfirmationRow[] {
  const rows: ConfirmationRow[] = [];

  for (const fact of facts) {
    const active = fact.versions.find((v) => v.status === "active");
    for (const version of fact.versions.filter((v) => v.status === "proposed")) {
      const next = `${version.value_text}${version.unit ? ` ${version.unit}` : ""}`;
      const current = active
        ? `${active.value_text}${active.unit ? ` ${active.unit}` : ""}`
        : null;
      rows.push({
        id: version.id,
        kind: "fact",
        what: fact.label,
        change: current ? `${current} → ${next}` : `Would become ${next}`,
        evidenceCount: version.evidence?.length ?? 0,
        source: version.source,
        fact,
        version,
        activeVersionID: active?.id,
      });
    }
  }

  for (const decision of decisions.filter((d) => d.status === "proposed")) {
    rows.push({
      id: decision.id,
      kind: "decision",
      what: decision.statement,
      change: "Would be recorded as a decision this project has made",
      evidenceCount: decision.evidence?.length ?? 0,
      source: decision.source,
      decision,
    });
  }

  for (const contradiction of contradictions) {
    rows.push({
      id: contradiction.id,
      kind: "contradiction",
      what: contradiction.summary,
      change: "Two sources disagree — nothing changes until you pick one",
      evidenceCount: contradiction.sides?.length ?? 0,
      source: "llm",
      contradiction,
    });
  }

  return rows;
}

export function useProjectDetailData(projectID: string | undefined, filters: TimelineFilters) {
  const { accessToken } = useAuth();
  const queryClient = useQueryClient();
  const enabled = Boolean(accessToken && projectID);

  // Set while an extraction chain we queued is still in flight, so the status
  // line can say "Reviewing…" rather than showing a stale watermark.
  const [reviewingSince, setReviewingSince] = useState<number | null>(null);
  // Set when a queued chain passed the ceiling without the watermark moving.
  const [stalled, setStalled] = useState(false);

  const projectQuery = useQuery({
    queryKey: ["project", accessToken, projectID],
    queryFn: () => getProject(accessToken!, projectID!),
    enabled,
    refetchInterval: reviewingSince === null ? false : 3000,
  });
  const timelineQuery = useQuery({
    queryKey: [
      "project-timeline",
      accessToken,
      projectID,
      filters.source,
      filters.unassignedToIssue,
    ],
    queryFn: () =>
      getProjectTimeline(accessToken!, projectID!, {
        source: filters.source,
        unassigned_to_issue: filters.unassignedToIssue,
        limit: 100,
      }),
    enabled,
  });
  const issuesQuery = useQuery({
    queryKey: ["project-issues", accessToken, projectID],
    queryFn: () => listProjectIssues(accessToken!, projectID!),
    enabled,
  });
  const currentPositionQuery = useQuery({
    queryKey: ["project-current-position", accessToken, projectID],
    queryFn: () => getCurrentPosition(accessToken!, projectID!),
    enabled,
  });
  const factsQuery = useQuery({
    queryKey: ["project-facts", accessToken, projectID],
    queryFn: () =>
      listProjectFacts(accessToken!, projectID!, { include: ["proposed", "history"] }),
    enabled,
  });
  const contradictionsQuery = useQuery({
    queryKey: ["project-contradictions", accessToken, projectID],
    queryFn: () => listProjectContradictions(accessToken!, projectID!, "open"),
    enabled,
  });
  const decisionsQuery = useQuery({
    queryKey: ["project-decisions", accessToken, projectID],
    queryFn: () => listProjectDecisions(accessToken!, projectID!),
    enabled,
  });
  const attentionQuery = useQuery({
    queryKey: ["project-attention", accessToken, projectID],
    queryFn: () => getProjectAttention(accessToken!, projectID!),
    enabled,
  });
  const healthQuery = useQuery({
    queryKey: ["api-health"],
    queryFn: () => getApiHealth(),
    staleTime: 60_000,
  });

  const lastExtractedAt = projectQuery.data?.last_extracted_at;

  // Extraction finishing is what makes everything else on this page stale, so
  // the watermark advancing is the signal to refetch — not the click.
  //
  // The ceiling has to be a real timer rather than a comparison done inside
  // this effect: the case it exists for is the watermark never moving, and in
  // that case nothing in the dependency list ever changes, so an inline check
  // would never run again and the spinner would never clear.
  useEffect(() => {
    if (reviewingSince === null) return;
    const watermark = lastExtractedAt ? new Date(lastExtractedAt).getTime() : 0;
    if (watermark >= reviewingSince) {
      setReviewingSince(null);
      for (const key of [
        "project-facts",
        "project-decisions",
        "project-contradictions",
        "project-current-position",
        "project-issues",
        "project-attention",
      ]) {
        void queryClient.invalidateQueries({ queryKey: [key] });
      }
      return;
    }
    const remaining = Math.max(REVIEWING_CEILING_MS - (Date.now() - reviewingSince), 0);
    const timer = setTimeout(() => {
      setReviewingSince(null);
      setStalled(true);
    }, remaining);
    return () => clearTimeout(timer);
  }, [lastExtractedAt, reviewingSince, queryClient]);

  const checkNowMutation = useMutation({
    mutationFn: async () => {
      if (!accessToken || !projectID) throw new Error("Not authenticated");
      return extractProject(accessToken, projectID);
    },
    onSuccess: () => {
      setStalled(false);
      setReviewingSince(Date.now());
    },
    onError: (err) => {
      toast({
        title: "Could not check for updates",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  const allIssues = useMemo<IssueListItem[]>(() => issuesQuery.data ?? [], [issuesQuery.data]);
  // A discarded issue should never have been raised, so it leaves the open
  // list exactly as a resolved one does (spec §7.3).
  const openIssues = useMemo(
    () => allIssues.filter((iss) => iss.status !== "resolved" && !iss.discarded_at),
    [allIssues],
  );

  const confirmationRows = useMemo(
    () =>
      buildConfirmationRows(
        factsQuery.data ?? [],
        decisionsQuery.data ?? [],
        contradictionsQuery.data ?? [],
      ),
    [factsQuery.data, decisionsQuery.data, contradictionsQuery.data],
  );

  return {
    project: projectQuery.data,
    projectQuery,
    timeline: timelineQuery.data ?? [],
    timelineQuery,
    issues: allIssues,
    openIssues,
    issuesQuery,
    currentPosition: currentPositionQuery.data,
    currentPositionQuery,
    facts: factsQuery.data ?? [],
    factsQuery,
    contradictions: contradictionsQuery.data ?? [],
    contradictionsQuery,
    decisions: decisionsQuery.data ?? [],
    decisionsQuery,
    attention: attentionQuery.data,
    attentionQuery,
    confirmationRows,
    llmEnabled: healthQuery.data?.llm === true,
    extraction: {
      lastExtractedAt,
      reviewing: reviewingSince !== null,
      stalled,
      checkNow: () => checkNowMutation.mutate(),
      pending: checkNowMutation.isPending,
    },
  };
}
