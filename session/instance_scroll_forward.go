package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/streamhub"
)

// errScrollForwardResizeRaced is returned by captureViaRedrawQuiescence when
// a pane resize was detected running during its capture window. Unlike
// AttachSubscriber, a resize doesn't acquire ScrollForwardAttachBarrier's
// scrollForwardMu and can run fully concurrently with ForwardScroll, so a
// mid-capture resize could otherwise reflow the pane in a way the
// before/after content diff misreads as a genuine DELIVERED/AT_TOP result.
// Treated as an ordinary ForwardScroll error: lease/attach-barrier still
// release via defer, and the caller falls back to tmux-native scrollback.
var errScrollForwardResizeRaced = errors.New("scroll_forward: pane resize occurred during capture window, discarding outcome")

const (
	// defaultRedrawQuiescencePollInterval/defaultRedrawQuiescenceMaxWait
	// tune waitForRedrawQuiescence's poll-and-compare loop (Story 1.3.2).
	defaultRedrawQuiescencePollInterval = 150 * time.Millisecond
	defaultRedrawQuiescenceMaxWait      = 2 * time.Second

	// nativeScrollbackDelegateHistoryLimit bounds the GetScrollbackHistory
	// call a NativeScrollbackDelegateCapture adapter's capture delegates to
	// -- mirrors handleScrollbackRequest's own maxScrollbackLimit default
	// (server/services/connectrpc_websocket.go), since ForwardScroll has no
	// caller-supplied ScrollbackRequest offset/limit to derive one from.
	nativeScrollbackDelegateHistoryLimit = 1000
)

// ForwardScroll orchestrates one app-scrollback-forward request end to end:
// gate check -> lease acquire -> attach-barrier acquire -> send -> capture ->
// release. hub is nil on PathLegacyPerConnection, but AppScrollGate already
// blocks that path unconditionally via the -1 sentinel caller convention, so
// BeginScrollForward is never reached with a nil hub in practice.
//
// On any SendInputViaControlMode or capture error, ForwardScroll returns
// immediately with a non-nil err and every other return value at its zero
// value; the lease and attach-barrier releases still fire via defer
// regardless of which point failed (see
// TestForwardScroll_should_ReleaseLeaseAndAttachBarrier_When_* below).
func (i *Instance) ForwardScroll(ctx context.Context, subscriberCount int, hub *streamhub.StreamHub) (
	outcome sessionv1.ScrollForwardOutcome,
	blockedReason sessionv1.ScrollBlockedReason,
	content []byte,
	err error,
) {
	// Story 1.5's Observability Plan: one scroll_forward_attempts_total
	// increment, plus a structured exit log recording which path served the
	// request (DELIVERED/AT_TOP -> app_forwarded, anything else -> the
	// caller falls back to tmux-native), per ForwardScroll call regardless
	// of which return path is taken below -- the named return values are
	// read by this defer at whatever they're set to when ForwardScroll
	// actually returns.
	adapterLabel := scrollForwardAdapterLabel(i.GetProgram())
	defer func() {
		recordScrollForwardAttempt(adapterLabel, outcome, blockedReason)
		log.Info("scroll_forward: request completed",
			"session", i.GetTitle(),
			"adapter", adapterLabel,
			"path", scrollForwardServedPathLabel(outcome),
			"outcome", scrollForwardOutcomeLabel(outcome),
			"blocked_reason", scrollForwardBlockedReasonLabel(outcome, blockedReason),
			"err", err)
	}()

	logScrollForwardCoverageGapOnce(i.GetProgram())

	i.kickOffClaudeVersionMismatchCheck()

	if gateOutcome, gateReason, blocked := scrollForwardGateOutcome(i, subscriberCount); blocked {
		return gateOutcome, gateReason, nil, nil
	}

	if !i.scrollLease.tryAcquire() {
		return sessionv1.ScrollForwardOutcome_BLOCKED, sessionv1.ScrollBlockedReason_LEASE_CONTENTION, nil, nil
	}
	// Registered before scrollLease.release() below so it releases *last* --
	// see ScrollForwardAttachBarrier's doc comment (session/streamhub/hub.go)
	// for why a newly-unblocked AttachSubscriber must never race the lease's
	// own cleanup.
	if hub != nil {
		releaseBarrier := hub.BeginScrollForward()
		defer releaseBarrier()
	}
	defer i.scrollLease.release()

	adapter := resolveScrollAdapter(i.GetProgram())
	if adapter == nil {
		// AppScrollGate's check (1) already verified this above, so this is
		// unreachable in practice; defense-in-depth rather than a panic.
		return 0, 0, nil, fmt.Errorf("scroll_forward: no ScrollAdapter for program %q despite AppScrollGate pass", i.GetProgram())
	}

	for _, seq := range adapter.KeySequences(ScrollUp) {
		if sendErr := i.SendInputViaControlMode(ctx, seq); sendErr != nil {
			return 0, 0, nil, fmt.Errorf("scroll_forward: send key sequence: %w", sendErr)
		}
	}

	return i.captureForwardedScroll(ctx, adapter, hub)
}

// scrollForwardGateOutcome runs AppScrollGate and maps a failure to
// ForwardScroll's typed return values (Task 1.3.1c).
func scrollForwardGateOutcome(i *Instance, subscriberCount int) (outcome sessionv1.ScrollForwardOutcome, reason sessionv1.ScrollBlockedReason, blocked bool) {
	ok, gateFailure, gateReason := AppScrollGate(i, subscriberCount)
	if ok {
		return 0, 0, false
	}
	log.Info("scroll_forward: gate rejected request", "session", i.GetTitle(), "reason", gateReason)
	return sessionv1.ScrollForwardOutcome_BLOCKED, mapGateReason(gateFailure), true
}

// mapGateReason maps AppScrollGate's typed ScrollGateFailure to a
// ScrollBlockedReason -- a total switch, not a string comparison, so a
// future edit to AppScrollGate's human-readable reason string can never
// silently collapse ScrollGateUnsupportedPath and ScrollGateTooManyViewers
// into the same client-visible Blocked reason (a solo legacy-path user must
// never see "another viewer connected" copy). Every other gate failure maps
// to the unspecified reason, since the client only requests forwarding when
// it already believes it's eligible.
func mapGateReason(gateFailure ScrollGateFailure) sessionv1.ScrollBlockedReason {
	switch gateFailure {
	case ScrollGateUnsupportedPath:
		return sessionv1.ScrollBlockedReason_UNSUPPORTED_STREAMING_PATH
	case ScrollGateTooManyViewers:
		return sessionv1.ScrollBlockedReason_MULTIPLE_VIEWERS
	case ScrollGateOK, ScrollGateNoCapability, ScrollGateNotAltScreen, ScrollGateNoActiveController, ScrollGateUnsafeStatus:
		return sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED
	default:
		return sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED
	}
}

// captureForwardedScroll branches on adapter's declared ScrollCaptureMode
// (Pattern Decisions: "Capture mechanism selection inside ForwardScroll") --
// the interface declares its own contract rather than ForwardScroll
// type-switching on the concrete adapter.
func (i *Instance) captureForwardedScroll(ctx context.Context, adapter ScrollAdapter, hub *streamhub.StreamHub) (sessionv1.ScrollForwardOutcome, sessionv1.ScrollBlockedReason, []byte, error) {
	if adapter.CaptureVia() == NativeScrollbackDelegateCapture {
		return i.captureViaNativeScrollbackDelegate()
	}
	return i.captureViaRedrawQuiescence(ctx, hub)
}

// captureViaNativeScrollbackDelegate defers entirely to the existing
// tmux-capture pagination path (NativeScrollbackDelegateCapture) -- no
// quiescence wait, no fresh pane capture. Not exercised by Claude Code today
// (its adapter uses RedrawQuiescenceCapture per the Story 1.2.1 spike), but
// implemented generically since the ScrollAdapter interface allows any
// future adapter to declare it.
func (i *Instance) captureViaNativeScrollbackDelegate() (sessionv1.ScrollForwardOutcome, sessionv1.ScrollBlockedReason, []byte, error) {
	captured, err := i.GetScrollbackHistory(fmt.Sprintf("-%d", nativeScrollbackDelegateHistoryLimit), "-1")
	if err != nil {
		return 0, 0, nil, fmt.Errorf("scroll_forward: native scrollback delegate capture: %w", err)
	}
	return sessionv1.ScrollForwardOutcome_DELIVERED, sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED, []byte(captured), nil
}

// captureViaRedrawQuiescence waits for the pane's redraw to settle
// (Story 1.3.2), then computes AtTop vs. Delivered by comparing against the
// lease's last-captured content.
//
// hub (nil on the never-actually-reached PathLegacyPerConnection case, see
// ForwardScroll's doc comment) is sampled via ResizeActivity before and
// after the capture wait so a concurrent StreamHub.RequestResize can be
// detected and the outcome discarded rather than trusted (Fix 5).
func (i *Instance) captureViaRedrawQuiescence(ctx context.Context, hub *streamhub.StreamHub) (sessionv1.ScrollForwardOutcome, sessionv1.ScrollBlockedReason, []byte, error) {
	genBefore, resizingBefore := resizeActivitySnapshot(hub)

	captureStart := time.Now()
	settled, _, err := waitForRedrawQuiescence(ctx, i.CapturePaneContentPriority, defaultRedrawQuiescencePollInterval, defaultRedrawQuiescenceMaxWait)
	recordScrollForwardCaptureDuration(scrollForwardAdapterLabel(i.GetProgram()), time.Since(captureStart))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("scroll_forward: redraw quiescence capture: %w", err)
	}

	genAfter, resizingAfter := resizeActivitySnapshot(hub)
	if resizingBefore || resizingAfter || genBefore != genAfter {
		log.Warn("scroll_forward: pane resize occurred during capture window, discarding outcome", "session", i.GetTitle())
		return 0, 0, nil, errScrollForwardResizeRaced
	}

	settledBytes := []byte(settled)
	previousCaptured := i.scrollLease.lastCaptured
	outcome := sessionv1.ScrollForwardOutcome_DELIVERED
	if bytes.Equal(settledBytes, previousCaptured) {
		outcome = sessionv1.ScrollForwardOutcome_AT_TOP
	}
	i.scrollLease.lastCaptured = settledBytes

	// Story 1.5.2: heuristic drift canary, evaluated only for DELIVERED --
	// AT_TOP's before/after are byte-identical by construction above, so
	// there's nothing ambiguous to evaluate.
	scrollForwardKeybindingCanary(i.GetTitle(), outcome, previousCaptured, settledBytes)

	return outcome, sessionv1.ScrollBlockedReason_SCROLL_BLOCKED_REASON_UNSPECIFIED, settledBytes, nil
}

// resizeActivitySnapshot wraps StreamHub.ResizeActivity, tolerating a nil
// hub (unit tests that pass nil, and the never-actually-reached
// PathLegacyPerConnection case) by reporting "no resize activity" rather
// than requiring every caller to nil-check.
func resizeActivitySnapshot(hub *streamhub.StreamHub) (generation uint64, resizing bool) {
	if hub == nil {
		return 0, false
	}
	return hub.ResizeActivity()
}

// waitForRedrawQuiescence polls paneContent until two consecutive calls
// return byte-identical content, or maxWait elapses, whichever comes first.
// A paneContent error or ctx cancellation returns immediately, not retried --
// an upstream disconnect is a real cancellation, not an honest partial
// capture. deadlineExceeded is true only when maxWait elapsed without two
// matching polls; the caller treats that as an honest partial success, not
// an error, and still computes a real outcome from the last-seen content.
func waitForRedrawQuiescence(ctx context.Context, paneContent func() (string, error), pollInterval, maxWait time.Duration) (last string, deadlineExceeded bool, err error) {
	deadline := time.Now().Add(maxWait)
	haveLast := false
	for {
		if time.Now().After(deadline) {
			return last, true, nil
		}
		select {
		case <-ctx.Done():
			return last, false, fmt.Errorf("scroll_forward: redraw quiescence wait canceled: %w", ctx.Err())
		case <-time.After(pollInterval):
		}

		cur, perr := paneContent()
		if perr != nil {
			return "", false, fmt.Errorf("scroll_forward: pane content poll: %w", perr)
		}
		if haveLast && cur == last {
			return cur, false, nil
		}
		last = cur
		haveLast = true
	}
}

// scrollForwardServedPathLabel maps a ForwardScroll outcome to which path
// actually served the request -- DELIVERED/AT_TOP mean the app-forwarded
// content is what the caller returns to the client; every other outcome
// means the caller (scrollbackResultForRequest) falls back to the unchanged
// tmux-native GetScrollbackHistory path. Satisfies Epic 1.5's Observability
// Plan requirement to log "which path served a given scroll-up request".
func scrollForwardServedPathLabel(outcome sessionv1.ScrollForwardOutcome) string {
	switch outcome {
	case sessionv1.ScrollForwardOutcome_DELIVERED, sessionv1.ScrollForwardOutcome_AT_TOP:
		return "app_forwarded"
	default:
		return "tmux_native"
	}
}

// scrollForwardCoverageGapLogged guards logScrollForwardCoverageGapOnce so
// it fires at most once per distinct program string, not once per scroll
// attempt -- a session running a program with no registered ScrollAdapter
// would otherwise log this on every single request.
var scrollForwardCoverageGapLogged sync.Map

// logScrollForwardCoverageGapOnce logs a log.Warn the first time program is
// seen with no registered ScrollAdapter (Epic 1.5's Observability Plan:
// "coverage-gap visibility, not silent degradation"), and is a no-op on
// every later call for the same program.
func logScrollForwardCoverageGapOnce(program string) {
	if resolveScrollAdapter(program) != nil {
		return
	}
	if _, already := scrollForwardCoverageGapLogged.LoadOrStore(program, struct{}{}); already {
		return
	}
	log.Warn("scroll_forward: no ScrollAdapter registered for program, scroll-forwarding unavailable", "program", program)
}
