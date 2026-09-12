/**
 * Builds an Error whose message carries the
 * "(reason=transient|exhausted; reset_at=<RFC3339>)" marker
 * `classifyGitHubRateLimitError` (server/services/github_service.go) appends
 * when wrapping a GitHub rate-limit error, so specs can exercise
 * `getGitHubRateLimitMessage`'s parsing without hand-copying the wire format.
 */
export function rateLimitError(reason: "transient" | "exhausted", msUntilReset: number): Error {
  const resetAt = new Date(Date.now() + msUntilReset);
  return new Error(
    `github: rate limited until ${resetAt.toISOString()}: some detail (reason=${reason}; reset_at=${resetAt.toISOString()})`
  );
}
