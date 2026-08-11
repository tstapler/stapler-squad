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
