package prs

import (
	"fmt"
	"regexp"
	"time"
)

// githubRateLimitErrorPattern matches rateLimitTransport's fail-fast error
// text (github/http_client.go's RoundTrip: "github: rate limited until
// <RFC3339>, skipping request to avoid another guaranteed failure") so it can
// be reclassified into a `reason=transient|exhausted` marker before it
// reaches an RPC caller.
var githubRateLimitErrorPattern = regexp.MustCompile(`rate limited until (\S+),`)

// secondaryRateLimitMaxWait mirrors github/rate_limit.go's
// maxRetryAfterSleep -- the hard cap that RateLimiter.Update applies to a
// secondary (Retry-After-driven) rate-limit wait. A reset further out than
// this can only be the primary hourly limit, since the secondary path never
// produces a longer one; that's the classification signal, not a guess.
const secondaryRateLimitMaxWait = 60 * time.Second

// ClassifyGitHubRateLimitError appends a reason/reset-time marker to err's
// message when it recognizes rateLimitTransport's fail-fast text, so the
// frontend's getGitHubRateLimitMessage (web-app/src/lib/vcs/githubRateLimit.ts)
// can classify it into friendly copy instead of showing the raw string.
// Errors that don't match (network failures, auth errors, etc.) pass through
// unchanged.
//
// No connect.ErrorDetail/typed-error convention exists yet in this codebase
// for RPC error classification (verified: no connect.NewErrorDetail/WithDetails
// usage anywhere under server/) — a plain-text marker is the simplest
// convention that survives fmt.Errorf's %w wrapping through to the
// ConnectError message the frontend actually reads.
func ClassifyGitHubRateLimitError(err error) error {
	if err == nil {
		return nil
	}
	match := githubRateLimitErrorPattern.FindStringSubmatch(err.Error())
	if match == nil {
		return err
	}
	resetAt, parseErr := time.Parse(time.RFC3339, match[1])
	if parseErr != nil {
		return err
	}
	reason := "exhausted"
	if time.Until(resetAt) <= secondaryRateLimitMaxWait {
		reason = "transient"
	}
	return fmt.Errorf("%w (reason=%s; reset_at=%s)", err, reason, resetAt.Format(time.RFC3339))
}
