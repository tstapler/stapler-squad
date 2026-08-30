package github

import (
	"context"
	"testing"
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
