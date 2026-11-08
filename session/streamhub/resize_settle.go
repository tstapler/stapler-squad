package streamhub

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ResyncReasonResizeSettle labels the capture that repaints subscribers after a resize whose
// output was suppressed or whose snapshot missed its budget.
const ResyncReasonResizeSettle = "resize_settle"

const (
	defaultResizeSettleDelay = 150 * time.Millisecond
	// maxSettleAttempts bounds repeated failures: a daemon that cannot capture three times in a
	// row is the health checker's problem, not a reason to keep capturing.
	maxSettleAttempts = 3
)

// WithResizeSettleDelay overrides how long after a resize ends the repaint capture waits
// (default 150ms), which also coalesces a burst of resizes into one capture.
func WithResizeSettleDelay(d time.Duration) HubOption {
	return func(h *StreamHub) { h.resizeSettleDelay = d }
}

// settleState tracks whether a resize left subscribers possibly stale. While a resize applies,
// OnRawOutput drops live frames, so a snapshot older than the last dropped frame (or no
// snapshot at all) means the screen stays wrong until something else repaints it.
type settleState struct {
	droppedSinceCapture atomic.Bool

	mu       sync.Mutex
	timer    Timer
	attempts int
}

// noteSuppressedOutput is called by OnRawOutput for each frame dropped during a resize.
func (h *StreamHub) noteSuppressedOutput() { h.settle.droppedSinceCapture.Store(true) }

// armSettle schedules one coalesced repaint capture; calls while one is pending do nothing.
func (h *StreamHub) armSettle() {
	h.settle.mu.Lock()
	defer h.settle.mu.Unlock()
	if h.settle.timer != nil {
		return
	}
	h.settle.timer = h.clock.AfterFunc(h.resizeSettleDelay, h.runSettle)
}

func (h *StreamHub) clearSettleTimer() {
	h.settle.mu.Lock()
	h.settle.timer = nil
	h.settle.mu.Unlock()
}

// runSettle repaints every subscriber with a fresh full-state capture. It holds no lock across
// the capture, so a resize or FirstPaint is never stalled behind it; instead it samples the
// resize generation around the capture and, if a resize ran or started meanwhile, discards the
// result and tries again later (that resize captured its own snapshot, but a retry is cheap and
// keeps the "someone always repaints" guarantee simple). It gives up after maxSettleAttempts.
func (h *StreamHub) runSettle() {
	h.clearSettleTimer()
	if h.State() == HubTornDown || h.controller == nil {
		return
	}
	if h.SubscriberCount() == 0 || h.allAwaitingFirstPaint() {
		recordResyncCapture(ResyncReasonResizeSettle, "skipped_idle")
		return
	}
	genBefore, resizing := h.ResizeActivity()
	if resizing {
		h.retrySettle("skipped_busy")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.resizeBudget.current())
	defer cancel()
	h.settle.droppedSinceCapture.Store(false)
	snapshot, err := h.captureFullState(ctx)
	if err == nil {
		if genAfter, nowResizing := h.ResizeActivity(); nowResizing || genAfter != genBefore {
			h.retrySettle("skipped_raced")
			return
		}
		h.publishFullState(snapshot)
		h.settle.mu.Lock()
		h.settle.attempts = 0
		h.settle.mu.Unlock()
		h.settleCaptures.Add(1)
		recordResyncCapture(ResyncReasonResizeSettle, "published")
		return
	}
	recordResyncCapture(ResyncReasonResizeSettle, "failed")
	log.Warn("streamhub: resize settle capture failed", "session", h.sessionName, "error", err)
	if errors.Is(err, ErrSessionNotStarted) {
		return
	}
	h.retrySettle("")
}

// retrySettle re-arms the settle (counting an outcome when given) until maxSettleAttempts.
func (h *StreamHub) retrySettle(outcome string) {
	if outcome != "" {
		recordResyncCapture(ResyncReasonResizeSettle, outcome)
	}
	h.settle.mu.Lock()
	h.settle.attempts++
	retry := h.settle.attempts < maxSettleAttempts
	h.settle.mu.Unlock()
	if retry {
		h.armSettle()
	}
}

// ResyncCaptures is how many repaint captures with the given reason this hub has published.
func (h *StreamHub) ResyncCaptures(reason string) int {
	if reason != ResyncReasonResizeSettle {
		return 0
	}
	return int(h.settleCaptures.Load())
}

// fullState is a captured paint: the daemon's redraw (cache is false) or the prepared text
// capture (cache is true, so it also becomes the latest text snapshot).
type fullState struct {
	data  []byte
	cache bool
}

// captureFullState takes a full-state capture without publishing it.
func (h *StreamHub) captureFullState(ctx context.Context) (fullState, error) {
	if redraw, ok := h.captureRedrawContext(ctx); ok {
		return fullState{data: redraw}, nil
	}
	content, err := h.controller.CapturePaneContentRawContext(ctx)
	if err != nil {
		return fullState{}, err
	}
	return fullState{data: h.prepareCaptured(content), cache: true}, nil
}

// publishFullState sends a captured paint ahead of queued output.
func (h *StreamHub) publishFullState(s fullState) {
	if s.cache {
		h.snapshotMu.Lock()
		h.lastSnapshot = s.data
		h.snapshotMu.Unlock()
	}
	h.publishSnapshot(s.data)
}

// prepareCaptured runs a raw capture through the snapshot pipeline (erase+home, CRLF, cursor sync).
func (h *StreamHub) prepareCaptured(content RawPaneContent) []byte {
	return []byte(withCursorSync(h.clock, ansiSnapshotPrefix+prepareSnapshotContent(content), h.controller))
}

// publishCaptured prepares, caches and publishes a raw capture.
func (h *StreamHub) publishCaptured(content RawPaneContent) {
	h.publishFullState(fullState{data: h.prepareCaptured(content), cache: true})
}
