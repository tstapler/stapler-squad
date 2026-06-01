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
