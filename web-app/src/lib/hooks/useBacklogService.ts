"use client";

import { useCallback, useRef, useEffect } from "react";
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
  sessionId: string;
  role: string;
  startedAt?: string;
  endedAt?: string;
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
  return {
    sessionId: s.sessionUuid,
    role: s.sessionRole,
    startedAt: s.startedAt ? new Date(Number(s.startedAt.seconds) * 1000).toISOString() : undefined,
    endedAt: s.endedAt ? new Date(Number(s.endedAt.seconds) * 1000).toISOString() : undefined,
  };
}

function mapBacklogItem(p: BacklogItemProto): BacklogItem {
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
    linkedSessions: (p.itemSessions ?? []).map(mapItemSession),
    notes: p.notes || undefined,
    createdAt: p.createdAt ? new Date(Number(p.createdAt.seconds) * 1000).toISOString() : undefined,
    updatedAt: p.updatedAt ? new Date(Number(p.updatedAt.seconds) * 1000).toISOString() : undefined,
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
}

export function useBacklogService(): UseBacklogServiceReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

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
        return (resp.items ?? []).map(mapBacklogItem);
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
        return null;
      }
    },
    []
  );

  const updateBacklogItem = useCallback(
    async (id: string, data: Partial<BacklogItemInput>): Promise<BacklogItem | null> => {
      if (!clientRef.current) return null;
      try {
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
        const resp = await clientRef.current.transitionBacklogItemStatus({
          itemId: id,
          targetStatus: toStatus,
          expectedStatus: precondition ?? "",
          overrideReason: "",
        });
        return resp.item ? mapBacklogItem(resp.item) : null;
      } catch (err) {
        console.error("[useBacklogService] transitionStatus:", err);
        return null;
      }
    },
    []
  );

  const spawnSessionFromItem = useCallback(
    async (id: string): Promise<{ sessionUuid: string } | null> => {
      if (!clientRef.current) return null;
      try {
        const resp = await clientRef.current.spawnSessionFromItem({ itemId: id });
        return { sessionUuid: resp.sessionUuid };
      } catch (err) {
        console.error("[useBacklogService] spawnSessionFromItem:", err);
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
  };
}
