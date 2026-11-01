package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
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

// DefaultSendKeysTimeout bounds a plain (no-Enter) SendKeys write via
// SendKeysWithTimeout, matching the timeout server/mcp and server/services
// have historically used to guard against a wedged PTY write hanging a
// request handler indefinitely.
const DefaultSendKeysTimeout = 5 * time.Second

// ErrSubmitNotConfirmed is returned by SubmitDriverContent when the Enter
// keystroke was written successfully but the pane never showed any change
// afterward, even after one retry — the signature of Claude Code's TUI
// paste-detector swallowing the submit keystroke. Before this, a swallowed
// submit was indistinguishable from success at the call site (SendKeys
// returning nil), which is exactly the silent-failure shape the original bug
// report described.
var ErrSubmitNotConfirmed = errors.New("submit keystroke sent but pane showed no change (may have been swallowed by paste detection)")

// keySender is the narrow interface SendKeysWithTimeout needs — satisfied by
// *Instance's existing SendKeys method.
type keySender interface {
	leaseOwner
	SendKeys(keys string) error
}

// paneSubmitter is the narrow interface SubmitDriverContent needs — satisfied
// by *Instance's existing SendKeys and HasUpdated methods.
type paneSubmitter interface {
	paneSettleChecker
	keySender
}

// paneOwnerVerifier is optionally implemented by paneSubmitters backed by a
// real tmux pane; SubmitDriverContent calls it just before the first write.
type paneOwnerVerifier interface {
	VerifyPaneOwner(ctx context.Context) error
}

// VerifyPaneOwner re-checks, right before a write, that the tmux pane under
// this instance's name is still stamped with this instance's UUID
// (ce71ad1a). A no-op for non-tmux backends.
func (i *Instance) VerifyPaneOwner(ctx context.Context) error {
	if i.GetTmuxSession() == nil {
		return nil
	}
	return VerifyPaneOwnershipBeforeWrite(ctx, i)
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
// the retry also shows no change, ErrSubmitNotConfirmed is returned. This can
// double-submit a bare Enter if the first one actually registered but the
// pane simply hadn't rendered within maxWait — a rarer failure than the one
// this guards against.
//
// lease is the caller's held write lease for inst (Story 5.0): this function
// only receives it and releases it exactly once on every return path, in the
// goroutine that issues the write. A nil, zero or foreign lease is refused
// with ErrNoLease/ErrLeaseMismatch and 0 writes.
func SubmitDriverContent(ctx context.Context, inst paneSubmitter, lease *HeldLease, content string, pollInterval, maxWait time.Duration) error {
	defer lease.Release()
	if err := lease.check(inst); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("submit cancelled before sending content: %w", err)
	}
	if v, ok := inst.(paneOwnerVerifier); ok {
		if err := v.VerifyPaneOwner(ctx); err != nil {
			log.Error("SubmitDriverContent: refusing to write, pane owner not verified", "err", err)
			return fmt.Errorf("pane owner not verified, content not sent: %w", err)
		}
	}
	if err := sendKeysChunked(ctx, inst, content); err != nil {
		return fmt.Errorf("send content: %w", err)
	}
	waitForPaneSettle(ctx, inst, pollInterval, maxWait)
	// A ctx that expired during waitForPaneSettle must stop us here — otherwise
	// an abandoned goroutine (caller already gave up on timeoutCtx) still fires
	// a real Enter keystroke into the pane after the fact.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("submit cancelled before sending Enter: %w", err)
	}
	baseline, haveBaseline := capturePaneBaseline(inst)
	if err := inst.SendKeys(EnterKeySequence); err != nil {
		return fmt.Errorf("send submit keystroke: %w", err)
	}
	if waitForSubmitEffect(ctx, inst, baseline, haveBaseline, pollInterval, maxWait) {
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
	if waitForSubmitEffect(ctx, inst, baseline, haveBaseline, pollInterval, maxWait) {
		return nil
	}
	return ErrSubmitNotConfirmed
}

// sendKeysChunked writes content in maxInlinePromptBytes pieces (ADR-031's
// inline class) so one write never holds the pane writer for a whole large
// prompt. The pane is a byte stream, so splitting is lossless. A failure says
// how much was already written: partial delivery must be visible.
func sendKeysChunked(ctx context.Context, inst keySender, content string) error {
	if len(content) <= maxInlinePromptBytes {
		return inst.SendKeys(content)
	}
	for sent := 0; sent < len(content); {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("cancelled after %d of %d bytes: %w", sent, len(content), err)
		}
		end := min(sent+maxInlinePromptBytes, len(content))
		if err := inst.SendKeys(content[sent:end]); err != nil {
			return fmt.Errorf("failed after %d of %d bytes: %w", sent, len(content), err)
		}
		sent = end
	}
	return nil
}

// SubmitContentWithEnter routes content through SubmitDriverContent, bounding
// the whole call with a generous timeout since the settle-wait plus
// up-to-one-retry confirmation can take noticeably longer than a single
// SendKeys call. Shared by server/mcp and server/services so both MCP-tool
// and Connect-RPC call sites use identical submit mechanics instead of each
// duplicating this goroutine+timeout wrapper.
//
// SubmitContentWithEnter does not release lease itself: it hands it to
// SubmitDriverContent inside its goroutine, which owns the release, so the
// lease stays held until an abandoned write's goroutine returns (a
// DeadlineExceeded here does not mean the pane is free).
func SubmitContentWithEnter(ctx context.Context, inst paneSubmitter, lease *HeldLease, content string) error {
	if err := lease.check(inst); err != nil {
		lease.Release()
		return err
	}
	// timeoutCtx (not ctx) is handed to the goroutine so that once this
	// function gives up on it, SubmitDriverContent's internal settle/confirm
	// polls (which check ctx.Done()) stop promptly too, instead of
	// continuing unobserved and potentially firing the blind retry-Enter
	// write after the caller has already moved on.
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*DefaultPaneSettleMaxWait+2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- SubmitDriverContent(timeoutCtx, inst, lease, content, DefaultPaneSettlePollInterval, DefaultPaneSettleMaxWait)
	}()

	select {
	case err := <-errCh:
		return err
	case <-timeoutCtx.Done():
		return context.DeadlineExceeded
	}
}

// SendKeysWithTimeout writes content to inst with no trailing Enter, bounded
// by timeout so a PTY write that blocks forever (the scenario a wedged
// session can trigger) surfaces as context.DeadlineExceeded instead of
// hanging the caller indefinitely. This is the no-Enter counterpart to
// SubmitContentWithEnter — every SendKeys call from server/mcp and
// server/services must go through one of the two, never inst.SendKeys bare,
// so a blocking write can't hang a request handler.
//
// The write goroutine releases lease when SendKeys returns, so a timed-out
// caller leaves the lease held until the abandoned write actually finishes.
func SendKeysWithTimeout(ctx context.Context, inst keySender, lease *HeldLease, content string, timeout time.Duration) error {
	if err := lease.check(inst); err != nil {
		lease.Release()
		return err
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		defer lease.Release()
		errCh <- inst.SendKeys(content)
	}()

	select {
	case err := <-errCh:
		return err
	case <-timeoutCtx.Done():
		return context.DeadlineExceeded
	}
}

// paneCapturer is optionally implemented by paneSubmitters that can snapshot
// their pane; *Instance does.
type paneCapturer interface {
	CapturePaneContent() (string, error)
}

func capturePaneBaseline(inst paneSettleChecker) (string, bool) {
	c, ok := inst.(paneCapturer)
	if !ok {
		return "", false
	}
	content, err := c.CapturePaneContent()
	return content, err == nil
}

// waitForSubmitEffect is waitForPaneUpdate plus a private check: HasUpdated is
// a shared consume-on-read hash diff, so a concurrent poller can swallow the
// change this submit caused and make a delivered message look unconfirmed
// (ErrSubmitNotConfirmed, inviting duplicate sends). Comparing the pane
// against our own pre-Enter snapshot cannot be stolen.
func waitForSubmitEffect(ctx context.Context, inst paneSettleChecker, baseline string, haveBaseline bool, pollInterval, maxWait time.Duration) bool {
	if !haveBaseline {
		return waitForPaneUpdate(ctx, inst, pollInterval, maxWait)
	}
	c := inst.(paneCapturer)
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(pollInterval):
		}
		if now, err := c.CapturePaneContent(); err == nil && now != baseline {
			return true
		}
		if updated, _ := inst.HasUpdated(); updated {
			return true
		}
	}
	return false
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
