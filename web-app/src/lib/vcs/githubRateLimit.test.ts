import { getGitHubRateLimitMessage } from "./githubRateLimit";

const ISO_TIMESTAMP_PATTERN = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/;

function backendError(reason: "transient" | "exhausted", msUntilReset: number): Error {
  const resetAt = new Date(Date.now() + msUntilReset);
  return new Error(
    `failed to refresh PR info: github: rate limited until ${resetAt.toISOString()}, skipping request to avoid another guaranteed failure (reason=${reason}; reset_at=${resetAt.toISOString()})`
  );
}

describe("getGitHubRateLimitMessage", () => {
  it("returns transient copy with a relative time and no raw ISO timestamp", () => {
    const err = backendError("transient", 20_000);
    const message = getGitHubRateLimitMessage(err, "fallback");
    expect(message).toMatch(
      /^GitHub is temporarily rate-limited — this should clear up in about .+, retrying automatically\.$/
    );
    expect(message).not.toMatch(ISO_TIMESTAMP_PATTERN);
  });

  it("returns the no-auto-retry transient copy when autoRetries is false", () => {
    const err = backendError("transient", 20_000);
    const message = getGitHubRateLimitMessage(err, "fallback", { autoRetries: false });
    expect(message).toBe("GitHub is rate-limited right now — tap Retry to try again.");
    expect(message).not.toMatch(ISO_TIMESTAMP_PATTERN);
  });

  it("returns exhausted copy with a relative time and no raw ISO timestamp", () => {
    const err = backendError("exhausted", 4 * 60_000);
    const message = getGitHubRateLimitMessage(err, "fallback");
    expect(message).toMatch(/^GitHub rate limit reached — try again in ~.+\.$/);
    expect(message).not.toMatch(ISO_TIMESTAMP_PATTERN);
  });

  it("falls back to the generic message for a non-rate-limit error", () => {
    const err = new Error("network error: connection refused");
    expect(getGitHubRateLimitMessage(err, "Failed to load comments")).toBe("Failed to load comments");
  });

  it("falls back to the generic message when error is not an Error instance", () => {
    expect(getGitHubRateLimitMessage(null, "Failed to load comments")).toBe("Failed to load comments");
    expect(getGitHubRateLimitMessage(undefined, "Failed to load comments")).toBe("Failed to load comments");
  });
});
