package session

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultPaneSettlePollInterval and DefaultPaneSettleMaxWait mirror
// AutonomousDriver's own defaults (autonomous_driver.go) so every
// driver-generated submission through SubmitDriverContent waits on the same
// tuning unless a caller has its own configured values (as AutonomousDriver does).
// Exported so callers outside the session package (server/mcp, server/services)
// can drive SubmitDriverContent without duplicating these constants.
const (
	DefaultPaneSettlePollInterval = 150 * time.Millisecond
	DefaultPaneSettleMaxWait      = 2 * time.Second
)

// ErrSubmitNotConfirmed is returned by SubmitDriverContent when the Enter
// keystroke was written successfully but the pane never showed any change
// afterward, even after one retry — the signature of Claude Code's TUI
// paste-detector swallowing the submit keystroke. Before this, a swallowed
// submit was indistinguishable from success at the call site (SendKeys
// returning nil), which is exactly the silent-failure shape the original bug
// report described.
var ErrSubmitNotConfirmed = errors.New("submit keystroke sent but pane showed no change (may have been swallowed by paste detection)")

// paneSubmitter is the narrow interface SubmitDriverContent needs — satisfied
// by *Instance's existing SendKeys and HasUpdated methods.
type paneSubmitter interface {
	paneSettleChecker
	SendKeys(keys string) error
}

// SubmitDriverContent sends driver-generated content to inst, then submits it
// with a separate Enter keystroke. content and Enter MUST travel as two
// distinct SendKeys writes (BUG-031): concatenating them into a single write
// lets the trailing Enter land inside Claude Code TUI's paste-detection window
// for sufficiently long content, folding it into the pasted block instead of
// submitting it. waitForPaneSettle gives the paste detector a chance to close
// before the submit keystroke goes out.
//
// This is the ONLY sanctioned way for session-driver code to submit
// LLM/driver-generated content terminated by Enter — never call
// inst.SendKeys(content + EnterKeySequence) directly. BUG-031's single-write
// pattern was fixed once (AutonomousDriver.run), then independently
// reintroduced at two more call sites (session_driver.go's initial-prompt and
// backlog-nudge sends) before this consolidation; routing every call site
// through one function is what stops a fourth one from repeating it.
// TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation enforces this
// structurally: any new inst.SendKeys(x + EnterKeySequence) call site outside
// this file fails that test.
//
// After the Enter write, it confirms the submit actually registered by
// polling for a pane change (waitForPaneUpdate) — a swallowed Enter is
// otherwise indistinguishable from success (SendKeys returning nil either
// way). One retry of the Enter write is attempted before giving up, since a
// slow-to-render pane can otherwise be mistaken for a swallowed submit; if
// the retry also shows no change, ErrSubmitNotConfirmed is returned.
func SubmitDriverContent(ctx context.Context, inst paneSubmitter, content string, pollInterval, maxWait time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("submit cancelled before sending content: %w", err)
	}
	if err := inst.SendKeys(content); err != nil {
		return fmt.Errorf("send content: %w", err)
	}
	waitForPaneSettle(ctx, inst, pollInterval, maxWait)
	// A ctx that expired during waitForPaneSettle must stop us here — otherwise
	// an abandoned goroutine (caller already gave up on timeoutCtx) still fires
	// a real Enter keystroke into the pane after the fact.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("submit cancelled before sending Enter: %w", err)
	}
	if err := inst.SendKeys(EnterKeySequence); err != nil {
		return fmt.Errorf("send submit keystroke: %w", err)
	}
	if waitForPaneUpdate(ctx, inst, pollInterval, maxWait) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("submit cancelled before retrying Enter: %w", err)
	}
	// Retry once: resend just the Enter keystroke in case the first one was
	// swallowed by the paste detector.
	if err := inst.SendKeys(EnterKeySequence); err != nil {
		return fmt.Errorf("send submit keystroke (retry): %w", err)
	}
	if waitForPaneUpdate(ctx, inst, pollInterval, maxWait) {
		return nil
	}
	return ErrSubmitNotConfirmed
}

// waitForPaneUpdate polls inst until it reports a pane change or maxWait
// elapses, returning whether a change was observed. The counterpart to
// waitForPaneSettle (which waits for changes to STOP): this waits for a
// change to START, so a caller verifying "did my send take effect" isn't
// forced to guess a single fixed sleep duration — a slow-to-render PTY
// capture that resolves at, say, 700ms no longer reads as "swallowed" just
// because it missed a 500ms deadline.
func waitForPaneUpdate(ctx context.Context, inst paneSettleChecker, pollInterval, maxWait time.Duration) (observed bool) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
		if updated, _ := inst.HasUpdated(); updated {
			return true
		}
	}
	return false
}
