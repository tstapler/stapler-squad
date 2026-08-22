package streamhub_test

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/session/streamhub"
)

// reentrancyTrackingController is a SessionController double built to catch
// the specific race this test targets: two RequestResize callers each
// independently deciding "the negotiated size changed" and both driving
// applyNegotiatedSize's SetWindowSize -> quiescence-wait -> CapturePaneContent
// pipeline at the same time. inPipeline is set for the pipeline's full
// duration (SetWindowSize entry through CapturePaneContent return) so a
// second, overlapping call is caught even though each individual method call
// is short.
type reentrancyTrackingController struct {
	mu         sync.Mutex
	inPipeline bool

	reentered    atomic.Bool
	pipelineRuns atomic.Int64
}

func (c *reentrancyTrackingController) enterPipeline() {
	c.mu.Lock()
	if c.inPipeline {
		c.reentered.Store(true)
	}
	c.inPipeline = true
	c.mu.Unlock()
}

func (c *reentrancyTrackingController) exitPipeline() {
	c.mu.Lock()
	c.inPipeline = false
	c.mu.Unlock()
}

func (c *reentrancyTrackingController) SetWindowSize(_, _ int) error {
	c.enterPipeline()
	// Widen the race window: without resizeApplyMu serializing
	// RequestResize end to end, this sleep gives a second concurrent
	// caller ample time to also observe changed == true and enter its own
	// SetWindowSize call before this one exits the pipeline.
	time.Sleep(2 * time.Millisecond)
	return nil
}

func (c *reentrancyTrackingController) ResizePTY(_, _ int) error { return nil }

func (c *reentrancyTrackingController) CapturePaneContent() (string, error) {
	time.Sleep(2 * time.Millisecond)
	c.pipelineRuns.Add(1)
	c.exitPipeline()
	return "snapshot", nil
}

func (c *reentrancyTrackingController) StopControlMode() error { return nil }

// SubscribeControlModeUpdates returns an already-closed channel so
// waitForQuiescence returns immediately (its receive-from-closed-channel
// branch), independent of any other concurrent call's own subscription —
// unlike fakeSessionController (lifecycle_test.go), this fake hands out a
// fresh channel per call instead of one shared channel, since Story 1.3.2's
// single-in-flight-resize assumption is exactly what this test violates on
// purpose to prove RequestResize now prevents it from ever happening for
// real.
func (c *reentrancyTrackingController) SubscribeControlModeUpdates() (string, <-chan []byte) {
	ch := make(chan []byte)
	close(ch)
	return "sub", ch
}

func (c *reentrancyTrackingController) UnsubscribeControlModeUpdates(_ string) {}

// TestRequestResize_should_NeverRunApplyNegotiatedSizePipelineConcurrentlyWithItself_When_ManySubscribersVoteSimultaneously
// is the regression test for the race a Layer 1 Go-idiom review found: two
// subscribers voting for a resize near-simultaneously could each
// independently observe changed == true (under the narrowly-scoped
// resizeMu) and both call applyNegotiatedSize at once, running two
// overlapping SetWindowSize -> quiescence-wait -> CapturePaneContent
// pipelines against the same SessionController — contradicting
// applyNegotiatedSize's own "exactly once per call" doc comment and this
// package's single-owner concurrency guarantee. The fix serializes
// RequestResize's full negotiate-then-apply sequence behind a dedicated
// resizeApplyMu (hub.go).
func TestRequestResize_should_NeverRunApplyNegotiatedSizePipelineConcurrentlyWithItself_When_ManySubscribersVoteSimultaneously(t *testing.T) {
	defer goleak.VerifyNone(t)

	controller := &reentrancyTrackingController{}
	hub := streamhub.NewStreamHub("concurrent-resize-session", controller,
		streamhub.WithTeardownGrace(time.Hour),
		streamhub.WithQuiescenceTimeout(50*time.Millisecond),
		streamhub.WithQuiescenceQuietPeriod(5*time.Millisecond),
	)
	defer func() {
		if err := hub.ForceTeardown(); err != nil {
			t.Fatalf("ForceTeardown() returned unexpected error: %v", err)
		}
	}()

	const numSubscribers = 40
	ids := make([]streamhub.SubscriberID, numSubscribers)
	for i := range ids {
		ids[i] = hub.AttachSubscriber(noopConcurrencyTransport{}, streamhub.SubscriberCapability{CanResize: true})
	}

	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(numSubscribers)

	for i, id := range ids {
		id := id
		// Every subscriber votes a distinct size so negotiateSize's
		// component-wise minimum keeps changing as votes land, which
		// keeps forcing `changed == true` down each goroutine's own
		// RequestResize path instead of only the first one.
		cols := 80 + i
		rows := 24 + i
		go func() {
			defer done.Done()
			start.Wait()
			size, err := streamhub.NewTerminalSize(cols, rows)
			if err != nil {
				t.Errorf("NewTerminalSize(%d, %d) returned unexpected error: %v", cols, rows, err)
				return
			}
			hub.RequestResize(id, size)
		}()
	}

	start.Done()
	done.Wait()

	if controller.reentered.Load() {
		t.Fatal("applyNegotiatedSize's pipeline ran concurrently with itself: RequestResize failed to serialize negotiate+apply across concurrent callers")
	}
	if runs := controller.pipelineRuns.Load(); runs == 0 {
		t.Fatal("expected at least one applyNegotiatedSize pipeline run across " + strconv.Itoa(numSubscribers) + " concurrent RequestResize calls, got 0")
	}
}

// noopConcurrencyTransport is a Transport that swallows everything — this
// test only cares about the resize pipeline's own serialization, not
// delivered frames.
type noopConcurrencyTransport struct{}

func (noopConcurrencyTransport) Send(_ []byte) error { return nil }
func (noopConcurrencyTransport) Close() error        { return nil }
