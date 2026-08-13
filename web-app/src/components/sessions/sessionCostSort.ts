import type { Session } from "@/gen/session/v1/types_pb";

/**
 * Compares two sessions by cost, always sorting sessions with no resolved
 * cost (not yet loaded, or genuinely unpriced) last — in BOTH sort
 * directions. The early return happens before the sortDir flip: a sentinel
 * value fed into a generic comparator would invert position when sortDir
 * flips, which is the specific bug this shape avoids.
 */
export function compareSessionsByCost(
  a: Session,
  b: Session,
  costById: Map<string, number>,
  sortDir: "asc" | "desc"
): number {
  const aCost = costById.get(a.id);
  const bCost = costById.get(b.id);
  const aMissing = aCost === undefined;
  const bMissing = bCost === undefined;
  if (aMissing !== bMissing) return aMissing ? 1 : -1;
  if (aMissing && bMissing) return 0;
  const cmp = aCost! - bCost!;
  return sortDir === "asc" ? cmp : -cmp;
}
