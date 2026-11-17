package instance

import (
	"context"
	"errors"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recheckHarness runs a probe whose first call blocks until released and
// returns EPERM; later calls return the given result immediately.
type recheckHarness struct {
	e        *WorktreeEnricher
	calls    atomic.Int32
	started  chan struct{}
	release  chan struct{}
	observed chan error
	deadline chan time.Time
	timers   atomic.Int32
}

func newRecheckHarness(t *testing.T, laterErr error) *recheckHarness {
	t.Helper()
	h := &recheckHarness{started: make(chan struct{}, 4), release: make(chan struct{}), observed: make(chan error, 8), deadline: make(chan time.Time, 1)}
	h.e = NewWorktreeEnricher(WorktreeEnricherConfig{
		OnResult: func(_ string, err error) { h.observed <- err },
		// Only the first attempt's deadline can be fired (by the test); later ones never fire.
		Timer: func(time.Duration) (<-chan time.Time, func()) {
			if h.timers.Add(1) == 1 {
				return h.deadline, func() {}
			}
			return nil, func() {}
		},
		AfterFunc: func(time.Duration, func()) {},
		Probe: func(string) (*WorktreeInfo, error) {
			if h.calls.Add(1) == 1 {
				h.started <- struct{}{}
				<-h.release
				return nil, syscall.EPERM
			}
			return &WorktreeInfo{}, laterErr
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	h.e.Start(ctx)
	t.Cleanup(func() { cancel(); h.e.Stop() })
	return h
}

func TestWorktreeEnricher_RecheckDiscardsStaleInFlightProbe(t *testing.T) {
	t.Parallel()
	h := newRecheckHarness(t, nil)
	h.e.Request("/p")
	<-h.started
	h.e.Recheck("/p")         // old probe still blocked
	h.deadline <- time.Time{} // the superseded handler gives up, freeing the queued re-probe
	select {
	case err := <-h.observed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("re-probe did not finish")
	}
	close(h.release) // old probe now returns EPERM
	// Join the generation-0 flight: it completes only after the old probe's finish ran.
	<-h.e.group.DoChan("/p\x000", func() (any, error) { return struct{}{}, nil })
	select {
	case err := <-h.observed:
		t.Fatalf("stale probe reported a result: %v", err)
	default:
	}
	got, ok := h.e.Result("/p")
	require.True(t, ok)
	require.NoError(t, got.Err)
}

func TestWorktreeEnricher_RecheckDropsCachedResult(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	started, release := make(chan struct{}, 2), make(chan struct{})
	e := NewWorktreeEnricher(WorktreeEnricherConfig{Probe: func(string) (*WorktreeInfo, error) {
		started <- struct{}{}
		if calls.Add(1) == 1 {
			return nil, syscall.EPERM
		}
		<-release // hold the re-probe so only the cache drop can hide the old result
		return &WorktreeInfo{}, nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	e.Start(ctx)
	t.Cleanup(func() { cancel(); e.Stop() })
	e.Request("/p")
	<-started
	require.Eventually(t, func() bool { r, ok := e.Result("/p"); return ok && errors.Is(r.Err, syscall.EPERM) }, time.Second, time.Millisecond)

	e.Recheck("/p")
	<-started
	_, ok := e.Result("/p")
	require.False(t, ok, "cached EPERM must not survive Recheck")
	close(release)
	require.Eventually(t, func() bool { r, ok := e.Result("/p"); return ok && r.Err == nil }, time.Second, time.Millisecond)
}
