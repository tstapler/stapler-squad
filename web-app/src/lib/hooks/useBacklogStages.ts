"use client";

import { useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogService, type BacklogStage as BacklogStageProto } from "@/gen/session/v1/backlog_pb";

export interface BacklogStageInfo {
  slug: string;
  name: string;
  enabled: boolean;
}

/**
 * The 9 built-in stages seeded by the backend on boot (session/workflow_stage_seed.go)
 * — used as the synchronous initial value so BacklogBoard/StageTracker render their
 * known columns/labels on first paint instead of flashing empty while ListStages is
 * in flight, and as the safe fallback on a fetch failure. Mirrors
 * CachingLivenessEngine/CachingPipelineEngine's cache-miss-falls-back-to-default
 * precedent (plan.md Pattern Decisions).
 */
export const BUILTIN_BACKLOG_STAGES: readonly BacklogStageInfo[] = [
  { slug: "idea", name: "Idea", enabled: true },
  { slug: "refining", name: "Refining", enabled: true },
  { slug: "ready", name: "Ready", enabled: true },
  { slug: "queued", name: "Queued", enabled: true },
  { slug: "in_progress", name: "In Progress", enabled: true },
  { slug: "review", name: "Review", enabled: true },
  { slug: "pr_pending", name: "PR Pending", enabled: true },
  { slug: "done", name: "Done", enabled: true },
  { slug: "archived", name: "Archived", enabled: true },
];

// Module-level cache shared across every hook instance in the page — once one
// mount's ListStages call succeeds, later mounts (e.g. navigating between
// board and item detail) start from that value instead of re-flashing the
// built-in defaults. Cleared only by a page reload, same lifetime as
// pipelineModeCache's in-memory cache.
let cachedStages: BacklogStageInfo[] | null = null;

function mapStage(p: BacklogStageProto): BacklogStageInfo {
  return { slug: p.slug, name: p.name, enabled: p.enabled };
}

export interface UseBacklogStagesReturn {
  stages: BacklogStageInfo[];
  isLoading: boolean;
  error: Error | null;
}

/**
 * Fetches the live, operator-configured stage set (built-in + custom) via
 * ListStages, so BacklogBoard's columns and StageTracker's unrecognized-status
 * fallback reflect what's actually configured instead of a hardcoded list.
 *
 * Never blanks the result on a fetch failure — it keeps the last-known (or
 * built-in default) list, since a stage silently disappearing from the board
 * is exactly the BUG-037 failure mode this epic exists to prevent.
 */
export function useBacklogStages(): UseBacklogStagesReturn {
  const [stages, setStages] = useState<BacklogStageInfo[]>(
    cachedStages ?? [...BUILTIN_BACKLOG_STAGES]
  );
  const [isLoading, setIsLoading] = useState(cachedStages === null);
  const [error, setError] = useState<Error | null>(null);
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (!clientRef.current) {
      clientRef.current = createClient(BacklogService, getConnectTransport());
    }

    clientRef.current
      .listStages({})
      .then((resp) => {
        if (cancelled) return;
        const fetched = (resp.items ?? []).map(mapStage);
        cachedStages = fetched;
        setStages(fetched);
        setError(null);
      })
      .catch((err) => {
        if (cancelled) return;
        console.error("[useBacklogStages] listStages:", err);
        setError(err instanceof Error ? err : new Error("Failed to load backlog stages"));
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return { stages, isLoading, error };
}

// ---------------------------------------------------------------------------
// useBacklogStagesAdmin — full CRUD surface for the Epic 2.8 settings UI.
// Separate from useBacklogStages above (a read-only, cached list for board/
// detail rendering) since the two were built concurrently for different
// consumers and have incompatible shapes (subscribe-to-a-value vs.
// imperative action functions).
// ---------------------------------------------------------------------------

import { useCallback } from "react";
import type {
  StageTransition as StageTransitionProto,
  TransitionGate as TransitionGateProto,
} from "@/gen/session/v1/backlog_pb";

// ---------------------------------------------------------------------------
// Domain types exposed to UI (mapped from proto, but without Message<> noise)
// ---------------------------------------------------------------------------

/**
 * The 9 built-in BacklogStatus-derived stages seeded by
 * session.EnsureBuiltInWorkflowStages. Neither BacklogStage nor its proto
 * carries an explicit "is built-in" flag, so the frontend derives it from
 * slug membership in this closed set — matches useBacklogService.ts's
 * KnownBacklogStatus list.
 */
export const BUILT_IN_STAGE_SLUGS: ReadonlySet<string> = new Set([
  "idea",
  "refining",
  "ready",
  "queued",
  "in_progress",
  "review",
  "pr_pending",
  "done",
  "archived",
]);

export function isBuiltInStage(slug: string): boolean {
  return BUILT_IN_STAGE_SLUGS.has(slug);
}

/** GateKind mirrors session.GateKind's 4-value sealed set (session/gate_status.go). */
export type GateKind = "human_approval" | "automated_review" | "structural" | "custom";

/** Human-readable labels for each GateKind, shared by StageForm and StageGraphDiagram. */
export const GATE_KIND_LABELS: Record<GateKind, string> = {
  human_approval: "human approval",
  automated_review: "automated review",
  structural: "structural check",
  custom: "custom check",
};

/**
 * BacklogStage is one node in the workflow graph, built-in or
 * operator-defined. Mapped from session.v1.BacklogStage.
 */
export interface BacklogStage {
  id: string;
  slug: string;
  name: string;
  description: string;
  isEntry: boolean;
  isTerminal: boolean;
  enabled: boolean;
}

export interface BacklogStageInput {
  slug: string;
  name: string;
  description?: string;
  isEntry?: boolean;
  isTerminal?: boolean;
  enabled?: boolean;
}

/** Slug is immutable after creation (mirrors PipelineModeUpdateInput). */
export type BacklogStageUpdateInput = Partial<Omit<BacklogStageInput, "slug">>;

/**
 * TransitionGate is one gate attached to a StageTransition. `config` carries
 * kind-specific key/value pairs validated server-side by
 * session.ParseGateConfig against the kind's allowlisted key set:
 *  - human_approval: {} (no config)
 *  - automated_review: { pipeline_mode, requires_diff }
 *  - structural: { check_id }
 *  - custom: { skill }
 */
export interface TransitionGate {
  id: string;
  transitionId: string;
  kind: GateKind;
  config: Record<string, string>;
  stateful: boolean;
  orderIndex: number;
  enabled: boolean;
}

export interface TransitionGateInput {
  transitionId: string;
  kind: GateKind;
  config?: Record<string, string>;
  stateful?: boolean;
  orderIndex?: number;
  enabled?: boolean;
}

/**
 * UpdateTransitionGateRequest always resubmits kind+config together (never a
 * partial kind-XOR-config update — see backlog.proto's doc comment on
 * UpdateTransitionGateRequest), so kind/config stay required here even though
 * this is otherwise an "update" shape.
 */
export interface TransitionGateUpdateInput {
  kind: GateKind;
  config?: Record<string, string>;
  stateful?: boolean;
  orderIndex?: number;
  enabled?: boolean;
}

/** StageTransition is one (from_stage, to_stage) edge, gates eager-loaded. */
export interface StageTransition {
  id: string;
  fromStageSlug: string;
  toStageSlug: string;
  enabled: boolean;
  gates: TransitionGate[];
}

export interface StageTransitionInput {
  fromStageSlug: string;
  toStageSlug: string;
  enabled?: boolean;
}

export interface StageTransitionUpdateInput {
  enabled?: boolean;
}

/** Result of a create/update that also runs Epic 2.6's graph validator. */
export interface StageTransitionSaveResult {
  item: StageTransition;
  /** Non-blocking warnings (e.g. a gate-free cycle) — never a rejection. */
  warnings: string[];
}

function mapFullStage(s: BacklogStageProto): BacklogStage {
  return {
    id: s.id,
    slug: s.slug,
    name: s.name,
    description: s.description,
    isEntry: s.isEntry,
    isTerminal: s.isTerminal,
    enabled: s.enabled,
  };
}

function mapGate(g: TransitionGateProto): TransitionGate {
  return {
    id: g.id,
    transitionId: g.transitionId,
    kind: g.kind as GateKind,
    config: g.config ?? {},
    stateful: g.stateful,
    orderIndex: g.orderIndex,
    enabled: g.enabled,
  };
}

function mapTransition(t: StageTransitionProto): StageTransition {
  return {
    id: t.id,
    fromStageSlug: t.fromStageSlug,
    toStageSlug: t.toStageSlug,
    enabled: t.enabled,
    gates: (t.gates ?? []).map(mapGate),
  };
}

export interface UseBacklogStagesAdminReturn {
  listStages: () => Promise<BacklogStage[]>;
  createStage: (data: BacklogStageInput) => Promise<BacklogStage>;
  /** Partial-updates an existing stage (e.g. just `{enabled}` for the list-row toggle). */
  updateStage: (id: string, data: BacklogStageUpdateInput) => Promise<BacklogStage>;
  /** force bypasses the live-item-count guard — never set true from this settings UI. */
  deleteStage: (id: string, force?: boolean) => Promise<boolean>;

  listStageTransitions: (fromStageSlug?: string) => Promise<StageTransition[]>;
  createStageTransition: (data: StageTransitionInput) => Promise<StageTransitionSaveResult>;
  updateStageTransition: (id: string, data: StageTransitionUpdateInput) => Promise<StageTransitionSaveResult>;
  deleteStageTransition: (id: string) => Promise<boolean>;

  createTransitionGate: (data: TransitionGateInput) => Promise<TransitionGate>;
  updateTransitionGate: (id: string, data: TransitionGateUpdateInput) => Promise<TransitionGate>;
  deleteTransitionGate: (id: string) => Promise<boolean>;
}

/**
 * RPC-backed CRUD for the Epic 2.7 stage/transition/gate surface, feeding the
 * Epic 2.8 "Backlog Stages" settings UI (and, per plan.md's Epic 2.9, the
 * board/detail dynamic-stage rendering). Self-contained (its own ConnectRPC
 * client) rather than folded into useBacklogService.ts, so this and other
 * concurrently-built epics don't collide on the same shared hook file.
 */
export function useBacklogStagesAdmin(): UseBacklogStagesAdminReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(BacklogService, getConnectTransport());
  }, []);

  const listStages = useCallback(async (): Promise<BacklogStage[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.listStages({});
      return (resp.items ?? []).map(mapStage);
    } catch (err) {
      console.error("[useBacklogStages] listStages:", err);
      throw err;
    }
  }, []);

  const createStage = useCallback(async (data: BacklogStageInput): Promise<BacklogStage> => {
    if (!clientRef.current) throw new Error("Backlog service not connected");
    try {
      const resp = await clientRef.current.createStage({
        slug: data.slug,
        name: data.name,
        description: data.description ?? "",
        isEntry: data.isEntry ?? false,
        isTerminal: data.isTerminal ?? false,
        enabled: data.enabled ?? true,
      });
      if (!resp.item) throw new Error("createStage: server returned no item");
      return mapFullStage(resp.item);
    } catch (err) {
      console.error("[useBacklogStages] createStage:", err);
      throw err;
    }
  }, []);

  const updateStage = useCallback(async (id: string, data: BacklogStageUpdateInput): Promise<BacklogStage> => {
    if (!clientRef.current) throw new Error("Backlog service not connected");
    try {
      const resp = await clientRef.current.updateStage({
        id,
        name: data.name,
        description: data.description,
        isEntry: data.isEntry,
        isTerminal: data.isTerminal,
        enabled: data.enabled,
      });
      if (!resp.item) throw new Error("updateStage: server returned no item");
      return mapFullStage(resp.item);
    } catch (err) {
      console.error("[useBacklogStages] updateStage:", err);
      throw err;
    }
  }, []);

  const deleteStage = useCallback(async (id: string, force?: boolean): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deleteStage({ id, force: force ?? false });
      return true;
    } catch (err) {
      console.error("[useBacklogStages] deleteStage:", err);
      throw err;
    }
  }, []);

  const listStageTransitions = useCallback(async (fromStageSlug?: string): Promise<StageTransition[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.listStageTransitions({ fromStageSlug });
      return (resp.items ?? []).map(mapTransition);
    } catch (err) {
      console.error("[useBacklogStages] listStageTransitions:", err);
      throw err;
    }
  }, []);

  const createStageTransition = useCallback(
    async (data: StageTransitionInput): Promise<StageTransitionSaveResult> => {
      if (!clientRef.current) throw new Error("Backlog service not connected");
      try {
        const resp = await clientRef.current.createStageTransition({
          fromStageSlug: data.fromStageSlug,
          toStageSlug: data.toStageSlug,
          enabled: data.enabled ?? true,
        });
        if (!resp.item) throw new Error("createStageTransition: server returned no item");
        return { item: mapTransition(resp.item), warnings: resp.warnings ?? [] };
      } catch (err) {
        console.error("[useBacklogStages] createStageTransition:", err);
        throw err;
      }
    },
    []
  );

  const updateStageTransition = useCallback(
    async (id: string, data: StageTransitionUpdateInput): Promise<StageTransitionSaveResult> => {
      if (!clientRef.current) throw new Error("Backlog service not connected");
      try {
        const resp = await clientRef.current.updateStageTransition({ id, enabled: data.enabled });
        if (!resp.item) throw new Error("updateStageTransition: server returned no item");
        return { item: mapTransition(resp.item), warnings: resp.warnings ?? [] };
      } catch (err) {
        console.error("[useBacklogStages] updateStageTransition:", err);
        throw err;
      }
    },
    []
  );

  const deleteStageTransition = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deleteStageTransition({ id });
      return true;
    } catch (err) {
      console.error("[useBacklogStages] deleteStageTransition:", err);
      throw err;
    }
  }, []);

  const createTransitionGate = useCallback(async (data: TransitionGateInput): Promise<TransitionGate> => {
    if (!clientRef.current) throw new Error("Backlog service not connected");
    try {
      const resp = await clientRef.current.createTransitionGate({
        transitionId: data.transitionId,
        kind: data.kind,
        config: data.config ?? {},
        stateful: data.stateful ?? false,
        orderIndex: data.orderIndex ?? 0,
        enabled: data.enabled ?? true,
      });
      if (!resp.item) throw new Error("createTransitionGate: server returned no item");
      return mapGate(resp.item);
    } catch (err) {
      console.error("[useBacklogStages] createTransitionGate:", err);
      throw err;
    }
  }, []);

  const updateTransitionGate = useCallback(
    async (id: string, data: TransitionGateUpdateInput): Promise<TransitionGate> => {
      if (!clientRef.current) throw new Error("Backlog service not connected");
      try {
        const resp = await clientRef.current.updateTransitionGate({
          id,
          kind: data.kind,
          config: data.config ?? {},
          stateful: data.stateful,
          orderIndex: data.orderIndex,
          enabled: data.enabled,
        });
        if (!resp.item) throw new Error("updateTransitionGate: server returned no item");
        return mapGate(resp.item);
      } catch (err) {
        console.error("[useBacklogStages] updateTransitionGate:", err);
        throw err;
      }
    },
    []
  );

  const deleteTransitionGate = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deleteTransitionGate({ id });
      return true;
    } catch (err) {
      console.error("[useBacklogStages] deleteTransitionGate:", err);
      throw err;
    }
  }, []);

  return {
    listStages,
    createStage,
    updateStage,
    deleteStage,
    listStageTransitions,
    createStageTransition,
    updateStageTransition,
    deleteStageTransition,
    createTransitionGate,
    updateTransitionGate,
    deleteTransitionGate,
  };
}
