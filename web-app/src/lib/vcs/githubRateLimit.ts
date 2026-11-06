import { formatRelativeTime } from "@/lib/utils/datetime";

/**
 * The two buckets `server/services/github_service.go`'s
 * `classifyGitHubRateLimitError` sorts a GitHub rate-limit error into before
 * it reaches the frontend as an Error/ConnectError message.
 */
export type GitHubRateLimitReason = "transient" | "exhausted";

// Matches the "(reason=transient|exhausted; reset_at=<RFC3339>)" marker
// classifyGitHubRateLimitError appends to the wrapped error's message.
const RATE_LIMIT_MARKER_PATTERN = /reason=(transient|exhausted); reset_at=(\S+)\)/;

export interface GitHubRateLimitMessageOptions {
  /**
   * Whether this surface has a mechanism that automatically retries the
   * failed fetch (VcsPanel.tsx's 60s fallback poller does; VcsWidgetComments.tsx's
   * fetch-once-per-mount does not — see design/ux.md State A). Defaults to
   * true, matching VcsPanel.tsx's real behavior.
   */
  autoRetries?: boolean;
}

function extractMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  return "";
}

/**
 * Reuses formatRelativeTime's own duration-bucketing thresholds (1m/1h/1d)
 * for a *future* timestamp by mirroring it onto the past side of "now" and
 * stripping the resulting " ago" suffix, instead of duplicating that
 * threshold logic here. formatRelativeTime falls back to a bare calendar
 * date (no " ago" suffix) once its input is >=7 days old, which the mirror
 * trick can't reuse — that would render the mirrored *past* date instead of
 * the real future reset day — so that range is handled directly below.
 */
function relativeSpanUntil(resetAt: Date): string {
  const now = Date.now();
  const msUntil = Math.max(resetAt.getTime() - now, 0);
  const daysUntil = Math.floor(msUntil / 86_400_000);
  if (daysUntil >= 7) return `${daysUntil}d`;
  const mirrored = now - msUntil;
  const formatted = formatRelativeTime(mirrored);
  if (formatted === "Just now") return "a moment";
  return formatted.endsWith(" ago") ? formatted.slice(0, -" ago".length) : formatted;
}

/**
 * Classifies a GitHub-rate-limit error into friendly copy with a relative
 * time, or returns `fallback` unchanged for any error without the backend's
 * reason marker (network error, auth failure, etc.) — see design/ux.md's
 * State C, which this must not misclassify.
 */
export function getGitHubRateLimitMessage(
  error: unknown,
  fallback: string,
  options: GitHubRateLimitMessageOptions = {}
): string {
  const { autoRetries = true } = options;
  const match = RATE_LIMIT_MARKER_PATTERN.exec(extractMessage(error));
  if (!match) return fallback;

  const [, reason, resetAtRaw] = match;
  const resetAt = new Date(resetAtRaw);
  if (Number.isNaN(resetAt.getTime())) return fallback;

  if (reason === "transient") {
    return autoRetries
      ? `GitHub is temporarily rate-limited — this should clear up in about ${relativeSpanUntil(resetAt)}, retrying automatically.`
      : "GitHub is rate-limited right now — tap Retry to try again.";
  }
  return `GitHub rate limit reached — try again in ~${relativeSpanUntil(resetAt)}.`;
}
