package session

import (
	"context"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

// newTestPoller creates a ReviewQueuePoller with a minimal configuration suitable for
// unit tests: no storage, no status manager, no instances. It uses a short fastInterval
// and a longer slowInterval so tests can observe adaptive behavior within milliseconds.
func newTestPoller(t *testing.T, fastInterval, slowInterval time.Duration) (*ReviewQueuePoller, *ReviewQueue) {
	t.Helper()
	queue := NewReviewQueue()
	config := ReviewQueuePollerConfig{
		PollInterval:      fastInterval,
		SlowPollInterval:  slowInterval,
		ReconcileInterval: 0, // disable reconciliation
	}
	poller := NewReviewQueuePollerWithConfig(queue, nil, nil, config)
	return poller, queue
}

// TestAdaptivePoller_BackoffToIdleInterval verifies that when the review queue is empty
// the poll loop backs off to SlowPollInterval (R10).
//
// Strategy: use a very short fastInterval (10ms) and a longer slowInterval (200ms) so that
// we can observe the interval change without making the test slow. We start the poller with
// no sessions and no activity channel; after the first tick (which finds the queue empty),
// the loop should switch to slowInterval and NOT fire again within 2×fastInterval.
func TestAdaptivePoller_BackoffToIdleInterval(t *testing.T) {
	fastInterval := 20 * time.Millisecond
	slowInterval := 300 * time.Millisecond

	poller, queue := newTestPoller(t, fastInterval, slowInterval)
	_ = queue // queue is empty; the poller backs off after the first tick

	// Wire an activity channel so backoff logic is active.
	actCh := make(chan struct{}, 1)
	poller.SetActivityChannel(actCh)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	poller.Start(ctx)
	defer poller.Stop()

	// Wait for the first tick to actually land, with a generous timeout.
	cfg := wait.DefaultWaitConfig()
	cfg.Timeout = 500 * time.Millisecond
	cfg.PollInterval = 5 * time.Millisecond
	cfg.Description = "first poll tick"
	if err := wait.WaitForCondition(func() bool {
		return poller.tickCount.Load() > 0
	}, cfg); err != nil {
		t.Fatalf("timed out waiting for the first poll tick to fire: %v", err)
	}
	firstTickCount := poller.tickCount.Load()

	// The queue is empty and the activity channel is wired, so the loop should have
	// backed off to slowInterval. After another fastInterval + margin, the tick count
	// must NOT have increased (the next tick won't fire for ~slowInterval more).
	<-time.After(fastInterval + 20*time.Millisecond)
	tickAfterFast := poller.tickCount.Load()

	if tickAfterFast != firstTickCount {
		t.Errorf("expected tick count to stay at %d during slow interval backoff, got %d (poller fired again at fast rate)",
			firstTickCount, tickAfterFast)
	}
}

// TestAdaptivePoller_SnapOnApprovalResponse verifies that a signal on the activity
// channel causes the poll loop to snap back to PollInterval immediately (R11).
//
// Strategy: start with a very slow interval (500ms) and an activity channel. After the
// first tick the poller backs off. Then send a signal on the activity channel and verify
// that the loop fires again within fastInterval + margin (rather than waiting the full
// slow interval).
func TestAdaptivePoller_SnapOnApprovalResponse(t *testing.T) {
	fastInterval := 20 * time.Millisecond
	slowInterval := 500 * time.Millisecond

	poller, _ := newTestPoller(t, fastInterval, slowInterval)

	actCh := make(chan struct{}, 1)
	poller.SetActivityChannel(actCh)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	poller.Start(ctx)
	defer poller.Stop()

	// Wait for the first tick (fast interval) and the backoff to kick in.
	firstTickCfg := wait.DefaultWaitConfig()
	firstTickCfg.Timeout = 500 * time.Millisecond
	firstTickCfg.PollInterval = 5 * time.Millisecond
	firstTickCfg.Description = "first tick before signal"
	if err := wait.WaitForCondition(func() bool {
		return poller.tickCount.Load() > 0
	}, firstTickCfg); err != nil {
		t.Fatalf("timed out waiting for first tick: %v", err)
	}
	tickBeforeSignal := poller.tickCount.Load()

	// The loop is now on slowInterval; record the time and send the activity signal.
	signalTime := time.Now()
	actCh <- struct{}{}

	// Wait for an additional tick to fire after the activity signal snaps back to fast interval.
	// Use slowInterval as the timeout — the snap must fire before the slow interval elapses.
	snapCfg := wait.DefaultWaitConfig()
	snapCfg.Timeout = slowInterval
	snapCfg.PollInterval = 5 * time.Millisecond
	snapCfg.Description = "snap-to-fast tick after activity signal"
	if err := wait.WaitForCondition(func() bool {
		return poller.tickCount.Load() > tickBeforeSignal
	}, snapCfg); err != nil {
		t.Fatalf("timed out waiting for snap tick: %v", err)
	}
	tickAfterSignal := poller.tickCount.Load()
	elapsed := time.Since(signalTime)

	if tickAfterSignal <= tickBeforeSignal {
		t.Errorf("expected at least one additional tick after activity signal within %s, got none (elapsed: %s)",
			fastInterval*3+50*time.Millisecond, elapsed)
	}
	if elapsed >= slowInterval {
		t.Errorf("tick after signal took %s which is >= slowInterval %s — snap did not occur",
			elapsed, slowInterval)
	}
}
