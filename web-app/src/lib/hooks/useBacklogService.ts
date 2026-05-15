"use client";

import { useCallback, useRef, useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { getApiBaseUrl, createAuthInterceptor } from "@/lib/config";
import {
  BacklogService,
  BacklogItem as BacklogItemProto,
  AcCriterion as AcCriterionProto,
  ItemSession as ItemSessionProto,
} from "@/gen/session/v1/backlog_pb";

// ---------------------------------------------------------------------------
// Domain types exposed to UI (mapped from proto, but without Message<> noise)
// ---------------------------------------------------------------------------

export type BacklogItemStatus =
  | "idea"
  | "ready"
  | "in_progress"
  | "review"
  | "done"
  | "archived";

export type AcCriterionStatus = "pending" | "in_progress" | "done";

export interface AcCriterion {
  index: number;
  text: string;
  status: AcCriterionStatus;
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

  return session;
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

  // Derive triageStatus from linked sessions: running if a triage session has no endedAt.
  let triageStatus: BacklogItem["triageStatus"];
  const triageSession = linkedSessions.filter((s) => s.role === "triage").at(-1);
  if (triageSession) {
    triageStatus = triageSession.endedAt ? "completed" : "running";
  }

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
  createBacklogItem: (data: BacklogItemInput) => Promise<BacklogItem | null>;
  updateBacklogItem: (id: string, data: Partial<BacklogItemInput>) => Promise<BacklogItem | null>;
  archiveBacklogItem: (id: string) => Promise<boolean>;
  transitionStatus: (
    id: string,
    toStatus: BacklogItemStatus,
    precondition?: BacklogItemStatus
  ) => Promise<BacklogItem | null>;
  spawnSessionFromItem: (id: string) => Promise<{ sessionUuid: string } | null>;
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
    async (data: BacklogItemInput): Promise<BacklogItem | null> => {
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
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
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
      return false;
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
    async (id: string): Promise<{ sessionUuid: string } | null> => {
      if (!clientRef.current) return null;
      try {
        setLastError(null);
        const resp = await clientRef.current.spawnSessionFromItem({ itemId: id });
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
        return null;
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
      return null;
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
        return false;
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
      return false;
    }
  }, []);

  return {
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
  };
}
