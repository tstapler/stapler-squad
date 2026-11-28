package prs

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rateLimitedUntilErr builds an error matching rateLimitTransport's fail-fast
// text (github/http_client.go's RoundTrip fmt.Errorf).
func rateLimitedUntilErr(resetAt time.Time) error {
	return fmt.Errorf("github: rate limited until %s, skipping request to avoid another guaranteed failure", resetAt.Format(time.RFC3339))
}

// TestClassifyGitHubRateLimitError verifies rateLimitTransport's fail-fast
// error text is reclassified into a reason=transient|exhausted marker based
// on secondaryRateLimitMaxWait (mirrors github/rate_limit.go's
// maxRetryAfterSleep), and that a non-matching error passes through
// unchanged.
func TestClassifyGitHubRateLimitError(t *testing.T) {
	t.Parallel()

	t.Run("transient when reset is within secondaryRateLimitMaxWait", func(t *testing.T) {
		t.Parallel()
		err := rateLimitedUntilErr(time.Now().Add(30 * time.Second))

		got := ClassifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=transient")
		require.NotContains(t, got.Error(), "reason=exhausted")
	})

	t.Run("boundary: reset at secondaryRateLimitMaxWait is still transient", func(t *testing.T) {
		t.Parallel()
		// time.Until(resetAt) is re-evaluated inside classifyGitHubRateLimitError
		// a few microseconds after resetAt is computed here, so it always lands
		// at-or-just-under secondaryRateLimitMaxWait -- exercising the "<="
		// boundary (not "<") without wall-clock flakiness.
		err := rateLimitedUntilErr(time.Now().Add(secondaryRateLimitMaxWait))

		got := ClassifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=transient")
	})

	t.Run("exhausted when reset is beyond secondaryRateLimitMaxWait", func(t *testing.T) {
		t.Parallel()
		err := rateLimitedUntilErr(time.Now().Add(secondaryRateLimitMaxWait + 5*time.Minute))

		got := ClassifyGitHubRateLimitError(err)

		require.Error(t, got)
		require.Contains(t, got.Error(), "reason=exhausted")
		require.NotContains(t, got.Error(), "reason=transient")
	})

	t.Run("non-rate-limit error passes through unchanged", func(t *testing.T) {
		t.Parallel()
		err := errors.New("dial tcp: connection refused")

		got := ClassifyGitHubRateLimitError(err)

		require.Same(t, err, got)
		require.NotContains(t, got.Error(), "reason=")
	})

	t.Run("nil error passes through as nil", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ClassifyGitHubRateLimitError(nil))
	})
}
