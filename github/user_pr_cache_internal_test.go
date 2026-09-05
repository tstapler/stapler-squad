package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestUserPRCache_Stop_WaitsForLoopToExit is a glass-box regression test for
// BUG-052: Stop() must block until loop() has actually closed c.done, not
// just cancel the context, or a caller (notably t.Cleanup(cache.Stop)) can
// return while loop() is still mid-fetch and race the next test's use of
// shared package-level state (e.g. go-keyring's mock).
func TestUserPRCache_Stop_WaitsForLoopToExit(t *testing.T) {
	t.Parallel()
	c := NewUserPRCache()
	c.Start(context.Background())
	c.Stop()

	select {
	case <-c.done:
	default:
		t.Fatal("Stop() returned before loop() closed c.done")
	}
}

// TestUserPRCache_Stop_BeforeStart_NoPanic verifies Stop is a safe no-op
// when called before Start, per Stop's documented contract.
func TestUserPRCache_Stop_BeforeStart_NoPanic(t *testing.T) {
	t.Parallel()
	c := NewUserPRCache()
	c.Stop()
}

// TestUserPRCache_Stop_NoGoroutineLeak confirms Stop() actually waits for
// loop() to exit rather than just signaling it — goleak.VerifyNone must see
// nothing new relative to the pre-Start baseline immediately after Stop()
// returns, with no polling/sleep needed. Mirrors
// TestAnalyticsStore_Stop_JoinsFlushGoroutine (server/services/analytics_store_test.go).
func TestUserPRCache_Stop_NoGoroutineLeak(t *testing.T) {
	// Not t.Parallel(): a goleak baseline can be polluted by other parallel
	// tests' own background goroutines starting mid-flight, per the same
	// rationale documented on TestAnalyticsStore_Stop_JoinsFlushGoroutine.
	c := NewUserPRCache()

	baseline := goleak.IgnoreCurrent()
	c.Start(context.Background())
	c.Stop()

	goleak.VerifyNone(t, baseline)
}

// TestUserPRCache_Stop_Idempotent confirms Stop() can be called multiple
// times (e.g. once via t.Cleanup and once explicitly) without panicking.
func TestUserPRCache_Stop_Idempotent(t *testing.T) {
	t.Parallel()
	c := NewUserPRCache()
	c.Start(context.Background())

	require.NotPanics(t, c.Stop)
	require.NotPanics(t, c.Stop, "a second Stop() call must be a no-op, not a double-close panic")
}

// TestUserPRCache_Stop_BoundedAgainstHangingEndpoint confirms Stop() returns
// well within the shared 30s production shutdown-hooks budget
// (server.go's shutdown ordering, alongside WorkflowScheduler.Stop /
// BacklogService.Shutdown / SessionService.Shutdown) even when a fetch is
// blocked on a GitHub endpoint that never responds — ctx cancellation must
// abort the in-flight HTTP request, not leave loop() waiting for it.
func TestUserPRCache_Stop_BoundedAgainstHangingEndpoint(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client cancels (mirrors a hanging GitHub endpoint),
		// with a generous fallback so the test can't hang forever if
		// cancellation doesn't propagate as expected.
		select {
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()

	c := NewUserPRCache()
	c.Start(context.Background())

	start := time.Now()
	c.Stop()
	elapsed := time.Since(start)

	const budget = 5 * time.Second
	if elapsed > budget {
		t.Fatalf("Stop() took %s against a hanging endpoint, want < %s to preserve the shared 30s shutdown budget", elapsed, budget)
	}
}
