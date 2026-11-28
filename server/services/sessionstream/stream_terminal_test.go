package sessionstream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestWaitForTerminalResume_Cancelled(t *testing.T) {
	t.Parallel()
	inst := &instance.Instance{Status: domain.Active}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitForTerminalResume(ctx, inst), context.Canceled)
}

func TestWaitForTerminalResume_FailedStartup(t *testing.T) {
	t.Parallel()
	inst := &instance.Instance{Status: domain.Hibernated}
	require.ErrorContains(t, waitForTerminalResume(context.Background(), inst), "session startup failed")
}

// TestWaitWithTimeout pins waitWithTimeout's two branches directly, without
// depending on tmux or the e2e StreamTerminal path above.
func TestWaitWithTimeout(t *testing.T) {
	t.Parallel()
	t.Run("returns true when goroutines finish in time", func(t *testing.T) {
		t.Parallel()
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done() }()
		require.True(t, waitWithTimeout(&wg, time.Second))
	})

	t.Run("returns false when goroutines don't finish in time", func(t *testing.T) {
		t.Parallel()
		var wg sync.WaitGroup
		wg.Add(1) // deliberately never Done()
		require.False(t, waitWithTimeout(&wg, 10*time.Millisecond))
	})
}
