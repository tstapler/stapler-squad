"use client";

import { useCallback, useRef, useEffect, useState, useMemo } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import {
  BacklogService,
  BacklogItem as BacklogItemProto,
  AcCriterion as AcCriterionProto,
  ItemSession as ItemSessionProto,
  TriageTask as TriageTaskProto,
  BacklogStatusEvent as BacklogStatusEventProto,
} from "@/gen/session/v1/backlog_pb";

// ---------------------------------------------------------------------------
// Domain types exposed to UI (mapped from proto, but without Message<> noise)
// ---------------------------------------------------------------------------

export type KnownBacklogStatus = "idea" | "refining" | "ready" | "in_progress" | "review" | "done" | "archived";
// (string & {}) preserves autocomplete for KnownBacklogStatus values while still
// accepting unknown statuses returned by newer server versions.
export type BacklogItemStatus = KnownBacklogStatus | (string & {});

export type AcCriterionStatus = "pending" | "in_progress" | "done";

export interface AcCriterion {
  index: number;
  text: string;
  status: AcCriterionStatus;
}

export interface TriageSuggestion {
  text: string;
  rationale: string; // "question" for R7-lite clarifying questions
}

export interface TriageTask {
  text: string;
  estimate: string;
  category: string;
}

export interface TriageResult {
  summary: string;
  suggestions: TriageSuggestion[];
  clarifyingQuestions: string[];
  tasks?: TriageTask[];
}

export interface LinkedSession {
  /** Entity UUID of the ItemSession record — use for overrideVerdict calls. */
  entityId: string;
  /** Tmux session UUID — use for linking to the session terminal. */
  sessionId: string;
  role: string;
  startedAt?: string;
  endedAt?: string;
  reviewVerdict?: {
    overallOutcome?: "PASS" | "PARTIAL" | "FAIL" | "PENDING";
    summary?: string;
    perCriterion?: Array<{ criterionIndex: number; outcome: string }>;
  };
  triageResult?: TriageResult;
}

export interface BacklogItem {
  id: string;
  title: string;
  description?: string;
  status: BacklogItemStatus;
  /** 1 = highest priority, 5 = lowest */
  priority: number;
  repoPath?: string;
  skipPlanning: boolean;
  skipReviewGate: boolean;
  planApproved: boolean;
  planArtifactsPath?: string;
  acCriteria: AcCriterion[];
  linkedSessions: LinkedSession[];
  notes?: string;
  createdAt?: string;
  updatedAt?: string;
  /** Gate verdict from the most recent item session (if in review status) */
  gateVerdict?: "PASS" | "PARTIAL" | "FAIL" | "PENDING";
  gateVerdictSummary?: string;
  gateCriteria?: Array<{ label: string; passed: boolean }>;
  /** Triage progress indicator: when item is in "idea" status being triaged */
  triageStatus?: "running" | "completed" | "failed";
  /** Triage result from the most recent triage session (populated when triageStatus === "completed") */
  triageResult?: TriageResult;
  /** Status transition history for this item (audit log) */
  statusEvents: StatusEvent[];
}

export interface StatusEvent {
  id: string;
  fromStatus: string;
  toStatus: string;
  triggeredBy: string;
  createdAt?: string;
}

export interface BacklogItemInput {
  title: string;
  description?: string;
  repoPath?: string;
  priority?: number;
  skipPlanning?: boolean;
  skipReviewGate?: boolean;
  acCriteria?: AcCriterion[];
  notes?: string;
  skipTriage?: boolean;
}

export interface ListBacklogItemsFilter {
  statuses?: BacklogItemStatus[];
  priorities?: number[];
  includeTerminal?: boolean;
  search?: string;
}

// ---------------------------------------------------------------------------
// Proto ↔ domain mapping helpers
// ---------------------------------------------------------------------------

function mapAcCriterion(c: AcCriterionProto): AcCriterion {
  return {
    index: c.index,
    text: c.text,
    status: (c.status || "pending") as AcCriterionStatus,
  };
}

function mapItemSession(s: ItemSessionProto): LinkedSession {
  const session: LinkedSession = {
    entityId: s.id,
    sessionId: s.sessionUuid,
    role: s.sessionRole,
    startedAt: s.startedAt ? new Date(Number(s.startedAt.seconds) * 1000).toISOString() : undefined,
    endedAt: s.endedAt ? new Date(Number(s.endedAt.seconds) * 1000).toISOString() : undefined,
  };

  // Map review verdict if present
  if (s.reviewVerdict) {
    const rv = s.reviewVerdict;
    const knownOutcomes = new Set(["PASS", "FAIL", "PARTIAL"]);
    session.reviewVerdict = {
      // Map UNVERIFIABLE → PARTIAL so GateVerdictBox always gets a known verdict
      overallOutcome: knownOutcomes.has(rv.overallOutcome)
        ? (rv.overallOutcome as "PASS" | "PARTIAL" | "FAIL" | "PENDING")
        : rv.overallOutcome
          ? "PARTIAL"
          : "PENDING",
      summary: rv.summary,
      perCriterion: (rv.perCriterion ?? []).map((c) => ({
        criterionIndex: c.criterionIndex,
        outcome: c.outcome,
      })),
    };
  }

  // Map triage result if present
  if (s.triageResult) {
    const tr = s.triageResult;
    session.triageResult = {
      summary: tr.summary,
      suggestions: (tr.suggestions ?? []).map((sg) => ({
        text: sg.text,
        rationale: sg.rationale,
      })),
      clarifyingQuestions: tr.clarifyingQuestions ?? [],
      tasks: (tr.tasks ?? []).map((t: TriageTaskProto) => ({
        text: t.text,
        estimate: t.estimate,
        category: t.category,
      })),
    };
  }

  return session;
}

function mapStatusEvent(e: BacklogStatusEventProto): StatusEvent {
  return {
    id: e.id,
    fromStatus: e.fromStatus,
    toStatus: e.toStatus,
    triggeredBy: e.triggeredBy,
    createdAt: e.createdAt ? new Date(Number(e.createdAt.seconds) * 1000).toISOString() : undefined,
  };
}

function mapBacklogItem(p: BacklogItemProto): BacklogItem {
  const linkedSessions = (p.itemSessions ?? []).map(mapItemSession);

  // Extract gate verdict from the most recent session (for review status)
  let gateVerdict: "PASS" | "PARTIAL" | "FAIL" | "PENDING" | undefined;
  let gateVerdictSummary: string | undefined;
  let gateCriteria: Array<{ label: string; passed: boolean }> | undefined;

  if (linkedSessions.length > 0) {
    const mostRecentSession = linkedSessions[linkedSessions.length - 1];
    if (mostRecentSession.reviewVerdict?.overallOutcome) {
      gateVerdict = mostRecentSession.reviewVerdict.overallOutcome;
      gateVerdictSummary = mostRecentSession.reviewVerdict.summary;

      // Map per-criterion verdicts to criteria with pass/fail
      if (mostRecentSession.reviewVerdict.perCriterion?.length) {
        gateCriteria = mostRecentSession.reviewVerdict.perCriterion.map((c) => ({
          label: `Criterion ${c.criterionIndex}: ${c.outcome}`,
          passed: c.outcome === "PASS" || c.outcome === "pass",
        }));
      }
    }
  }

  // Derive triageStatus from linked sessions.
  // P12 fix: only mark "completed" if the session ended AND has a non-empty summary.
  // Orphan detection: a triage session without endedAt is only "running" while the item
  // is in "idea" status. If the item has advanced (ready, in_progress, etc.) the session
  // died without cleanly recording its end — treat it as "failed" so the UI doesn't show
  // a loading indicator for a session that no longer exists.
  const itemStatus = (p.status || "idea") as BacklogItemStatus;
  let triageStatus: BacklogItem["triageStatus"];
  const triageSession = linkedSessions.filter((s) => s.role === "triage").at(-1);
  if (triageSession) {
    if (triageSession.endedAt) {
      triageStatus = triageSession.triageResult?.summary ? "completed" : "failed";
    } else if (itemStatus === "idea") {
      triageStatus = "running";
    } else {
      triageStatus = "failed";
    }
  }

  const triageResult = triageSession?.triageResult;

  return {
    id: p.id,
    title: p.title,
    description: p.description || undefined,
    status: (p.status || "idea") as BacklogItemStatus,
    priority: p.priority || 3,
    repoPath: p.repoPath || undefined,
    skipPlanning: p.skipPlanning,
    skipReviewGate: p.skipReviewGate,
    planApproved: p.planApproved,
    planArtifactsPath: p.planArtifactsPath || undefined,
    acCriteria: (p.acceptanceCriteria ?? []).map(mapAcCriterion),
    linkedSessions,
    notes: p.notes || undefined,
    createdAt: p.createdAt ? new Date(Number(p.createdAt.seconds) * 1000).toISOString() : undefined,
    updatedAt: p.updatedAt ? new Date(Number(p.updatedAt.seconds) * 1000).toISOString() : undefined,
    gateVerdict,
    gateVerdictSummary,
    gateCriteria,
    triageStatus,
    triageResult,
    statusEvents: (p.statusEvents ?? []).map(mapStatusEvent),
  };
}

function toProtoAcCriteria(criteria: AcCriterion[]): AcCriterionProto[] {
  return criteria.map((c) => ({
    $typeName: "session.v1.AcCriterion" as const,
    index: c.index,
    text: c.text,
    status: c.status,
  }));
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

interface UseBacklogServiceReturn {
  listBacklogItems: (filter?: ListBacklogItemsFilter) => Promise<BacklogItem[]>;
  getBacklogItem: (id: string) => Promise<BacklogItem | null>;
  createBacklogItem: (data: BacklogItemInput) => Promise<{ item: BacklogItem; triageTriggered: boolean } | null>;
  updateBacklogItem: (id: string, data: Partial<BacklogItemInput>) => Promise<BacklogItem | null>;
  archiveBacklogItem: (id: string) => Promise<boolean>;
  transitionStatus: (
    id: string,
    toStatus: BacklogItemStatus,
    precondition?: BacklogItemStatus
  ) => Promise<BacklogItem | null>;
  spawnSessionFromItem: (id: string, options?: { autonomous?: boolean }) => Promise<{ sessionUuid: string } | null>;
  triggerTriage: (id: string) => Promise<{ itemSessionId: string } | null>;
  approvePlan: (id: string) => Promise<BacklogItem | null>;
  overrideVerdict: (id: string, overrideReason: string, toStatus?: string) => Promise<boolean>;
  triggerReReview: (id: string) => Promise<boolean>;
  /** Last error from createBacklogItem, updateBacklogItem, transitionStatus, or spawnSessionFromItem. */
  lastError: Error | null;
  /** Clears the lastError state. */
  clearError: () => void;
}

export function useBacklogService(): UseBacklogServiceReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);
  const [lastError, setLastError] = useState<Error | null>(null);

  const clearError = useCallback(() => setLastError(null), []);

  useEffect(() => {
    const transport = createConnectTransport({
      baseUrl: getApiBaseUrl(),
      interceptors: [createAuthInterceptor()],
    });
    clientRef.current = createClient(BacklogService, transport);
  }, []);

  const listBacklogItems = useCallback(
    async (filter?: ListBacklogItemsFilter): Promise<BacklogItem[]> => {
      if (!clientRef.current) return [];
      try {
        const resp = await clientRef.current.listBacklogItems({
          status: filter?.statuses ?? [],
          priority: filter?.priorities ?? [],
          includeTerminal: filter?.includeTerminal ?? false,
          sortBy: "",
        });
        const items = (resp.items ?? []).map(mapBacklogItem);
        if (filter?.search) {
          const q = filter.search.toLowerCase();
          return items.filter(
            (item) =>
              item.title.toLowerCase().includes(q) ||
              item.description?.toLowerCase().includes(q)
          );
        }
        return items;
      } catch (err) {
        console.error("[useBacklogService] listBacklogItems:", err);
        return [];
      }
    },
    []
  );

  const getBacklogItem = useCallback(async (id: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.getBacklogItem({ itemId: id });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] getBacklogItem:", err);
      return null;
    }
  }, []);

  const createBacklogItem = useCallback(
    async (data: BacklogItemInput): Promise<{ item: BacklogItem; triageTriggered: boolean } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.createBacklogItem({
          title: data.title,
          description: data.description ?? "",
          repoPath: data.repoPath ?? "",
          priority: data.priority ?? 3,
          skipPlanning: data.skipPlanning ?? false,
          skipReviewGate: data.skipReviewGate ?? false,
          acceptanceCriteria: toProtoAcCriteria(data.acCriteria ?? []),
          notes: data.notes ?? "",
          skipTriage: data.skipTriage ?? false,
        });
        return resp.item
          ? { item: mapBacklogItem(resp.item), triageTriggered: resp.triageTriggered }
          : null;
      } catch (err) {
        console.error("[useBacklogService] createBacklogItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const updateBacklogItem = useCallback(
    async (id: string, data: Partial<BacklogItemInput>): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.updateBacklogItem({
          itemId: id,
          title: data.title,
          description: data.description,
          repoPath: data.repoPath,
          priority: data.priority,
          skipPlanning: data.skipPlanning,
          skipReviewGate: data.skipReviewGate,
          acceptanceCriteria: data.acCriteria ? toProtoAcCriteria(data.acCriteria) : undefined,
          notes: data.notes,
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] updateBacklogItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const archiveBacklogItem = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.archiveBacklogItem({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] archiveBacklogItem:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const transitionStatus = useCallback(
    async (
      id: string,
      toStatus: BacklogItemStatus,
      precondition?: BacklogItemStatus
    ): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.transitionBacklogItemStatus({
          itemId: id,
          targetStatus: toStatus,
          expectedStatus: precondition ?? "",
          overrideReason: "",
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] transitionStatus:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const spawnSessionFromItem = useCallback(
    async (id: string, options?: { autonomous?: boolean }): Promise<{ sessionUuid: string } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.spawnSessionFromItem({
          itemId: id,
          autonomous: options?.autonomous ?? false,
        });
        return { sessionUuid: resp.sessionUuid };
      } catch (err) {
        console.error("[useBacklogService] spawnSessionFromItem:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        return null;
      }
    },
    []
  );

  const triggerTriage = useCallback(
    async (id: string): Promise<{ itemSessionId: string } | null> => {
      if (!clientRef.current) return null;
      try {
        const resp = await clientRef.current.triggerTriage({ itemId: id });
        return { itemSessionId: resp.itemSession?.id ?? "" };
      } catch (err) {
        console.error("[useBacklogService] triggerTriage:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const approvePlan = useCallback(async (id: string): Promise<BacklogItem | null> => {
    if (!clientRef.current) return null;
    try {
      const resp = await clientRef.current.approvePlan({ itemId: id });
      return resp.item ? mapBacklogItem(resp.item) : null;
    } catch (err) {
      console.error("[useBacklogService] approvePlan:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  const overrideVerdict = useCallback(
    async (id: string, overrideReason: string, toStatus?: string): Promise<boolean> => {
      if (!clientRef.current) return false;
      try {
        await clientRef.current.overrideVerdict({
          itemSessionId: id,
          overrideReason,
          toStatus: toStatus ?? "done",
        });
        return true;
      } catch (err) {
        console.error("[useBacklogService] overrideVerdict:", err);
        setLastError(err instanceof Error ? err : new Error(String(err)));
        throw err;
      }
    },
    []
  );

  const triggerReReview = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.triggerReReview({ itemId: id });
      return true;
    } catch (err) {
      console.error("[useBacklogService] triggerReReview:", err);
      setLastError(err instanceof Error ? err : new Error(String(err)));
      throw err;
    }
  }, []);

  // Stable object reference: all methods are useCallback(fn,[]) — only lastError changes.
  // Without useMemo, every render creates a new object, making callers' useCallback deps
  // fire on every render and causing infinite reload loops.
  return useMemo(
    () => ({
      listBacklogItems,
      getBacklogItem,
      createBacklogItem,
      updateBacklogItem,
      archiveBacklogItem,
      transitionStatus,
      spawnSessionFromItem,
      triggerTriage,
      approvePlan,
      overrideVerdict,
      triggerReReview,
      lastError,
      clearError,
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [lastError]
  );
}
