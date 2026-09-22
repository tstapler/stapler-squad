// +feature: insights-dashboard
// Shared formatting helpers for all insights components.

/** Format a USD cost with adaptive decimal precision. */
export function fmtCost(usd: number): string {
  if (usd < 0.01) return `$${usd.toFixed(4)}`;
  if (usd < 1) return `$${usd.toFixed(3)}`;
  return `$${usd.toFixed(2)}`;
}

/** Format a token count with M/K abbreviations. */
export function fmtTokens(n: bigint): string {
  const num = Number(n);
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`;
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`;
  return num.toString();
}

/** Format a cache-hit rate (0–1) as a percentage with 1 decimal place. */
export function fmtPct(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`;
}

/** Cache-hit rate: cacheRead / (input + cacheRead), 0-guarded. */
export function computeCacheHitRate(input: number, cacheRead: number): number {
  const denom = input + cacheRead;
  return denom === 0 ? 0 : cacheRead / denom;
}

/** Format a protobuf Timestamp as a short human-readable date. */
export function fmtDate(ts: { seconds: bigint } | undefined): string {
  if (!ts) return "—";
  return new Date(Number(ts.seconds) * 1000).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

/** Return the first 8 characters of an ID followed by an ellipsis. */
export function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) + "…" : id;
}

// Session worktree dirs are `<title>_<16-hex id>`; the transcript's project path
// decoding turns every "_" and "-" into "/", so the id is the segment after the title.
const WORKTREE_PATH = /\/worktrees\/(.+?)\/[0-9a-f]{16}(?:\/|$)/;

/**
 * Return a project path's display name: the session title for a stapler-squad
 * worktree path (whose last segment is just the opaque id), else its final path
 * segment. The title is approximate: original "-", "_" and "." are indistinguishable.
 */
export function pathBasename(p: string): string {
  const title = WORKTREE_PATH.exec(p)?.[1];
  return title ? title.replace(/\//g, "-") : p.split("/").pop() || p;
}
