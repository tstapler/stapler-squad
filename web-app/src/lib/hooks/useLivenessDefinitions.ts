"use client";

import { useCallback, useEffect, useRef } from "react";
import { createClient } from "@connectrpc/connect";
import { getConnectTransport } from "@/lib/api/transport";
import { BacklogService, type LivenessDefinition as LivenessDefinitionProto } from "@/gen/session/v1/backlog_pb";

/** Mirrors session.LivenessKind's 3-value sealed set (session/liveness_definition.go). */
export type LivenessKind = "duration_budget" | "heartbeat" | "cycle_frequency";

/**
 * LivenessDefinition is a stage/pipeline-mode-scoped override for how long a
 * backlog item may sit in a stage without progress before a reconcile sweep
 * flags it stuck. Mapped from session.v1.LivenessDefinition (see
 * backlog_pb.ts) — a tagged union over `kind`, where only the fields for that
 * kind are meaningful (the rest are zero).
 */
export interface LivenessDefinition {
  id: string;
  stageSlug: string;
  /** undefined = mode-less row, applies to any pipeline mode without its own override. */
  pipelineMode?: string;
  kind: LivenessKind;
  expectedDurationMs: number;
  stalenessMarginMs: number;
  maxNoProgressDurationMs: number;
  cycleThreshold: number;
  cycleLookbackMs: number;
  enabled: boolean;
}

export interface LivenessDefinitionInput {
  stageSlug: string;
  pipelineMode?: string;
  kind: LivenessKind;
  expectedDurationMs?: number;
  stalenessMarginMs?: number;
  maxNoProgressDurationMs?: number;
  cycleThreshold?: number;
  cycleLookbackMs?: number;
  enabled?: boolean;
}

/** stage_slug/pipeline_mode/kind are immutable after creation — not part of the update shape. */
export type LivenessDefinitionUpdateInput = Partial<
  Pick<
    LivenessDefinitionInput,
    "expectedDurationMs" | "stalenessMarginMs" | "maxNoProgressDurationMs" | "cycleThreshold" | "cycleLookbackMs" | "enabled"
  >
>;

function mapLiveness(p: LivenessDefinitionProto): LivenessDefinition {
  return {
    id: p.id,
    stageSlug: p.stageSlug,
    pipelineMode: p.pipelineMode,
    kind: p.kind as LivenessKind,
    expectedDurationMs: Number(p.expectedDurationMs),
    stalenessMarginMs: Number(p.stalenessMarginMs),
    maxNoProgressDurationMs: Number(p.maxNoProgressDurationMs),
    cycleThreshold: p.cycleThreshold,
    cycleLookbackMs: Number(p.cycleLookbackMs),
    enabled: p.enabled,
  };
}

export interface UseLivenessDefinitionsReturn {
  listLivenessDefinitions: () => Promise<LivenessDefinition[]>;
  createLivenessDefinition: (data: LivenessDefinitionInput) => Promise<LivenessDefinition>;
  updateLivenessDefinition: (id: string, data: LivenessDefinitionUpdateInput) => Promise<LivenessDefinition>;
  deleteLivenessDefinition: (id: string) => Promise<boolean>;
}

/**
 * RPC-backed CRUD for the 5 LivenessDefinition RPCs (Epic 1.3 of
 * backlog-custom-workflow-stages), feeding the settings UI's "Liveness
 * overrides" section (nested in StageForm). Self-contained (its own
 * ConnectRPC client) rather than folded into useBacklogStagesAdmin — same
 * precedent as that hook's own PipelineMode CRUD split in
 * useBacklogService.ts.
 */
export function useLivenessDefinitions(): UseLivenessDefinitionsReturn {
  const clientRef = useRef<ReturnType<typeof createClient<typeof BacklogService>> | null>(null);

  useEffect(() => {
    clientRef.current = createClient(BacklogService, getConnectTransport());
  }, []);

  const listLivenessDefinitions = useCallback(async (): Promise<LivenessDefinition[]> => {
    if (!clientRef.current) return [];
    try {
      const resp = await clientRef.current.listLivenessDefinitions({});
      return (resp.items ?? []).map(mapLiveness);
    } catch (err) {
      console.error("[useLivenessDefinitions] listLivenessDefinitions:", err);
      throw err;
    }
  }, []);

  const createLivenessDefinition = useCallback(async (data: LivenessDefinitionInput): Promise<LivenessDefinition> => {
    if (!clientRef.current) throw new Error("Backlog service not connected");
    try {
      const resp = await clientRef.current.createLivenessDefinition({
        stageSlug: data.stageSlug,
        pipelineMode: data.pipelineMode || undefined,
        kind: data.kind,
        expectedDurationMs: BigInt(Math.trunc(data.expectedDurationMs ?? 0)),
        stalenessMarginMs: BigInt(Math.trunc(data.stalenessMarginMs ?? 0)),
        maxNoProgressDurationMs: BigInt(Math.trunc(data.maxNoProgressDurationMs ?? 0)),
        cycleThreshold: Math.trunc(data.cycleThreshold ?? 0),
        cycleLookbackMs: BigInt(Math.trunc(data.cycleLookbackMs ?? 0)),
        enabled: data.enabled ?? true,
      });
      if (!resp.item) throw new Error("createLivenessDefinition: server returned no item");
      return mapLiveness(resp.item);
    } catch (err) {
      console.error("[useLivenessDefinitions] createLivenessDefinition:", err);
      throw err;
    }
  }, []);

  const updateLivenessDefinition = useCallback(
    async (id: string, data: LivenessDefinitionUpdateInput): Promise<LivenessDefinition> => {
      if (!clientRef.current) throw new Error("Backlog service not connected");
      try {
        const resp = await clientRef.current.updateLivenessDefinition({
          id,
          expectedDurationMs: data.expectedDurationMs !== undefined ? BigInt(Math.trunc(data.expectedDurationMs)) : undefined,
          stalenessMarginMs: data.stalenessMarginMs !== undefined ? BigInt(Math.trunc(data.stalenessMarginMs)) : undefined,
          maxNoProgressDurationMs:
            data.maxNoProgressDurationMs !== undefined ? BigInt(Math.trunc(data.maxNoProgressDurationMs)) : undefined,
          cycleThreshold: data.cycleThreshold !== undefined ? Math.trunc(data.cycleThreshold) : undefined,
          cycleLookbackMs: data.cycleLookbackMs !== undefined ? BigInt(Math.trunc(data.cycleLookbackMs)) : undefined,
          enabled: data.enabled,
        });
        if (!resp.item) throw new Error("updateLivenessDefinition: server returned no item");
        return mapLiveness(resp.item);
      } catch (err) {
        console.error("[useLivenessDefinitions] updateLivenessDefinition:", err);
        throw err;
      }
    },
    []
  );

  const deleteLivenessDefinition = useCallback(async (id: string): Promise<boolean> => {
    if (!clientRef.current) return false;
    try {
      await clientRef.current.deleteLivenessDefinition({ id });
      return true;
    } catch (err) {
      console.error("[useLivenessDefinitions] deleteLivenessDefinition:", err);
      throw err;
    }
  }, []);

  return { listLivenessDefinitions, createLivenessDefinition, updateLivenessDefinition, deleteLivenessDefinition };
}
