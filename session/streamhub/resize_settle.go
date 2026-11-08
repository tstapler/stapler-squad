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

// runSettle repaints every subscriber with a fresh full-state capture. It yields to a resize in
// flight (that resize arms its own settle when it ends) and gives up after maxSettleAttempts
// consecutive failures.
func (h *StreamHub) runSettle() {
	h.clearSettleTimer()
	if h.State() == HubTornDown || h.controller == nil {
		return
	}
	if !h.resizeApplyMu.TryLock() {
		recordResyncCapture(ResyncReasonResizeSettle, "skipped_busy")
		return
	}
	defer h.resizeApplyMu.Unlock()
	if h.SubscriberCount() == 0 || h.allAwaitingFirstPaint() {
		recordResyncCapture(ResyncReasonResizeSettle, "skipped_idle")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.resizeBudget.current())
	defer cancel()
	h.settle.droppedSinceCapture.Store(false)
	err := h.captureAndPublish(ctx)
	if err == nil {
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

// captureAndPublish takes a full-state capture and publishes it ahead of queued output: the
// daemon's redraw when the controller has one, else the prepared text capture.
func (h *StreamHub) captureAndPublish(ctx context.Context) error {
	if redraw, ok := h.captureRedrawContext(ctx); ok {
		h.publishSnapshot(redraw)
		return nil
	}
	content, err := h.controller.CapturePaneContentRawContext(ctx)
	if err != nil {
		return err
	}
	h.publishCaptured(content)
	return nil
}

// publishCaptured runs a raw capture through the snapshot pipeline (erase+home, CRLF, cursor
// sync), caches it as the latest text snapshot and publishes it.
func (h *StreamHub) publishCaptured(content RawPaneContent) {
	prepared := []byte(withCursorSync(h.clock, ansiSnapshotPrefix+prepareSnapshotContent(content), h.controller))
	h.snapshotMu.Lock()
	h.lastSnapshot = prepared
	h.snapshotMu.Unlock()
	h.publishSnapshot(prepared)
}
