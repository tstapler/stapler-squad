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
