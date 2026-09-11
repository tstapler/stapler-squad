package session

import (
	"context"
	"fmt"
	"time"
)

// defaultPaneSettlePollInterval and defaultPaneSettleMaxWait mirror
// AutonomousDriver's own defaults (autonomous_driver.go) so every
// driver-generated submission through SubmitDriverContent waits on the same
// tuning unless a caller has its own configured values (as AutonomousDriver does).
const (
	defaultPaneSettlePollInterval = 150 * time.Millisecond
	defaultPaneSettleMaxWait      = 2 * time.Second
)

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
func SubmitDriverContent(ctx context.Context, inst paneSubmitter, content string, pollInterval, maxWait time.Duration) error {
	if err := inst.SendKeys(content); err != nil {
		return fmt.Errorf("send content: %w", err)
	}
	waitForPaneSettle(ctx, inst, pollInterval, maxWait)
	if err := inst.SendKeys(EnterKeySequence); err != nil {
		return fmt.Errorf("send submit keystroke: %w", err)
	}
	return nil
}
