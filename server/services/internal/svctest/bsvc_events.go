package svctest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// requireCleanReturn cancels cancel and asserts watchBacklogItems's
// goroutine returns nil within a bounded deadline (proving the live loop's
// ctx.Done() branch actually fires rather than hanging forever).
func RequireCleanReturn(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("watchBacklogItems did not return after context cancellation")
	}
}
