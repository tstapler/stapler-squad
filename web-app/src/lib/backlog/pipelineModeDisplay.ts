import type { LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";

/**
 * Epic 3.4 "what ran" surface — resolves an ItemSession's frozen
 * pipelineModeSnapshot against the currently-fetched mode list, purely for
 * display (looking up the human-readable name). The underlying stored
 * value is never re-resolved live. Case priority:
 *   1. Snapshot slug not found in the current mode list → "unrecognized"
 *      (checked first — there's no live mode to compare a hash against).
 *   2. Snapshot slug found, but its content hash has since changed →
 *      "resolved" with drifted: true.
 *   3. Snapshot slug found and unchanged (or snapshot hash is empty,
 *      meaning default mode / a pre-feature session) → "resolved" with
 *      drifted: false. Empty pipelineModeSnapshot always short-circuits to
 *      the "default" case before any lookup is attempted.
 *
 * Extracted from BacklogItemDetail.tsx (Story 3.1.4, D6) so both
 * SessionsSection's per-session breakdown and LifecycleSummary's
 * glanceable Pipeline badge share one implementation.
 */
export type PipelineModeDisplay =
  | { kind: "resolved"; name: string; drifted: boolean }
  | { kind: "unrecognized"; slug: string };

export function resolvePipelineModeDisplay(
  session: Pick<LinkedSession, "pipelineModeSnapshot" | "pipelineModeSnapshotHash">,
  modes: PipelineMode[]
): PipelineModeDisplay {
  const snapshot = session.pipelineModeSnapshot ?? "";
  if (snapshot === "") {
    return { kind: "resolved", name: "default", drifted: false };
  }

  const match = modes.find((m) => m.slug === snapshot);
  if (!match) {
    return { kind: "unrecognized", slug: snapshot };
  }

  const snapshotHash = session.pipelineModeSnapshotHash ?? "";
  const drifted = snapshotHash !== "" && snapshotHash !== match.contentHash;
  return { kind: "resolved", name: match.name, drifted };
}

/** One session's runtime program-fallback fact (Story 2.3.1's resolveHeadlessCaller). */
export interface FallbackInfo {
  configuredProgram: string;
  resolvedProgram: string;
  resolvedModel: string;
  reason: string;
}

/**
 * The two "what ran" provenance facts for one session, computed
 * INDEPENDENTLY of each other — never an if/else-if chain that returns early
 * on the first true condition. A session can be both a fallback AND drifted
 * simultaneously (e.g. it fell back from gemini to Claude at spawn time, and
 * the mode's executor config was edited again afterward); both facts must be
 * reported so the UI can render both badges at once. See plan.md Story
 * 5.2.4's UX-lens BLOCKER 2 design note.
 */
export interface ExecutorProvenance {
  fallback: FallbackInfo | null;
  drifted: boolean;
}

/**
 * Resolves a session's executor-provenance facts (fallback + drift) against
 * `mode` — the currently-fetched PipelineMode this session's
 * pipelineModeSnapshot resolves to (or undefined if unresolved/default,
 * matching resolvePipelineModeDisplay's own precedent of only comparing
 * drift when a live mode match exists).
 *
 * `drifted` compares session.executorSnapshotHash against
 * `mode.stageExecutorHashes[session.role]` — Task 5.2.4a's now-DENSE map, so
 * an unconfigured role's session (whose own hash is
 * ComputeExecutorHash("", "")) matches the mode's own
 * ComputeExecutorHash("", "") entry for that role by construction, and never
 * falsely reads as drifted. An empty snapshotHash (a session predating this
 * feature) or a missing mode entry is treated as "no signal" — never
 * drifted.
 */
export function resolveExecutorProvenance(
  session: Pick<
    LinkedSession,
    "role" | "configuredProgram" | "resolvedProgram" | "resolvedModel" | "executorFallbackReason" | "executorSnapshotHash"
  >,
  mode: Pick<PipelineMode, "stageExecutorHashes"> | undefined
): ExecutorProvenance {
  const reason = session.executorFallbackReason ?? "";
  const fallback: FallbackInfo | null = reason
    ? {
        configuredProgram: session.configuredProgram ?? "",
        resolvedProgram: session.resolvedProgram ?? "",
        resolvedModel: session.resolvedModel ?? "",
        reason,
      }
    : null;

  const snapshotHash = session.executorSnapshotHash ?? "";
  const modeHash = mode?.stageExecutorHashes?.[session.role] ?? "";
  const drifted = snapshotHash !== "" && modeHash !== "" && snapshotHash !== modeHash;

  return { fallback, drifted };
}
