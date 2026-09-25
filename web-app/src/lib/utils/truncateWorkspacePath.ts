import { truncateMiddle } from "./truncateMiddle";

/**
 * Whether a path segment is opaque (a hash/UUID with no human-readable
 * meaning) or semantic (a readable name). Used by `truncateWorkspacePath` to
 * decide which segments are safe to collapse to a single ellipsis.
 */
export type SegmentKind = "semantic" | "opaque";

const HEX_HASH_RE = /^[0-9a-f]{7,40}$/i;
// Copied verbatim from a canonical UUID regex reference, per build-vs-buy.md's
// "don't hand-roll a UUID pattern" guidance.
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const TRAILING_OPAQUE_RE = /[-_][0-9a-f]{8,}$/i;
const HOME_DIR_RE = /^\/(?:home|Users)\/[^/]+/;

/**
 * Classifies a whole path segment as "opaque" (a bare hex hash/UUID, or one
 * with a trailing opaque hex suffix) or "semantic" (a human-readable name).
 */
export function classifySegment(segment: string): SegmentKind {
  if (
    HEX_HASH_RE.test(segment) ||
    UUID_RE.test(segment) ||
    TRAILING_OPAQUE_RE.test(segment)
  ) {
    return "opaque";
  }
  return "semantic";
}

function isWholeOpaque(segment: string): boolean {
  return HEX_HASH_RE.test(segment) || UUID_RE.test(segment);
}

/** Replaces a segment's trailing opaque hex suffix (if any) with "…". */
function collapseTrailingSuffix(segment: string): string {
  const trailingMatch = segment.match(TRAILING_OPAQUE_RE);
  if (trailingMatch && trailingMatch.index !== undefined) {
    return segment.slice(0, trailingMatch.index) + "…";
  }
  return segment;
}

/**
 * Collapses each run of one-or-more consecutive whole-opaque segments to a
 * single "…" segment, and collapses any remaining segment's trailing opaque
 * hex suffix (if present) to "…" while keeping its semantic prefix.
 */
function collapseOpaqueSegments(segments: string[]): string[] {
  const result: string[] = [];
  for (let i = 0; i < segments.length; ) {
    if (isWholeOpaque(segments[i])) {
      result.push("…");
      do {
        i++;
      } while (i < segments.length && isWholeOpaque(segments[i]));
      continue;
    }
    result.push(collapseTrailingSuffix(segments[i]));
    i++;
  }
  return result;
}

/**
 * Collapses opaque path segments (workspace hashes, worktree UUID suffixes)
 * to a single "…" while preserving semantic segments, so long
 * workspace/worktree paths stay scannable. Falls back to `truncateMiddle`'s
 * char-budget truncation when no opaque segment is found, or when segment
 * collapsing alone doesn't bring the path under `maxLen`.
 */
export function truncateWorkspacePath(path: string, maxLen: number): string {
  if (!path) return path;

  // Step 0: collapse the home-dir prefix before length-checking or
  // classifying segments — splitting first would put the username as an
  // interior segment that classifySegment would never flag as opaque.
  const homeCollapsed = path.replace(HOME_DIR_RE, "~");

  if (homeCollapsed.length <= maxLen) return homeCollapsed;

  const rejoined = collapseOpaqueSegments(homeCollapsed.split("/")).join("/");
  return rejoined.length > maxLen ? truncateMiddle(rejoined, maxLen) : rejoined;
}
