package autonomysvc

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeaseWedgeWatcher_ShouldScanOnEveryTickAndJoinOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	scanned := make(chan time.Time, 2)
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	StartLeaseWedgeWatcher(ctx, tick, &wg, func(now time.Time) { scanned <- now })

	for _, at := range []time.Time{time.Unix(1, 0), time.Unix(2, 0)} {
		select {
		case tick <- at:
		case <-time.After(5 * time.Second):
			require.Fail(t, "the watcher never took the tick")
		}
		select {
		case got := <-scanned:
			assert.Equal(t, at, got)
		case <-time.After(5 * time.Second):
			require.Fail(t, "the tick was never scanned")
		}
	}
	cancel()
	wg.Wait() // joined: the goroutine exits on cancel
}
