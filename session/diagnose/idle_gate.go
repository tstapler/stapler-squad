package diagnose

import (
	"log/slog"
	"time"

	"github.com/tstapler/stapler-squad/session/detection"
)

// SafeIdleStatusContexts allowlists the exact detection.StatusIdle pattern
// descriptions (session/detection/binaries/claude.go's Idle group) that are
// unambiguously Claude Code's own idle prompt, as opposed to a raw shell/vim/
// editor prompt that also reports StatusIdle. insert_mode is deliberately
// excluded: its regex/description aren't distinguishable from a real vim
// INSERT-mode status line.
//
// Canonical copy: server/services/session_service.go's safeIdleStatusContexts
// (the original home, predating Story 3.2.1) now aliases this var rather than
// duplicating it -- session/diagnose can't import server/services back
// without a cycle (session/tmux already imports session/diagnose, and
// server/services -> session -> session/tmux). Pinned against drift by
// TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions
// (server/services/session_service_test.go).
var SafeIdleStatusContexts = map[string]bool{
	"Claude Code readline input prompt":                                                              true, // claude_readline_prompt
	"Claude Code idle prompt showing ? for shortcuts":                                                true, // claude_shortcuts_prompt
	"Claude Code 'accept edits' review mode — session completed turn, user reviews proposed changes": true, // claude_accept_edits
}

// IsSafeSteerStatus reports whether a detection result is safe for an
// unattended PTY write: StatusIdle with a description on the Claude-specific
// SafeIdleStatusContexts allowlist. StatusIdle alone is NOT sufficient — a
// raw shell/editor prompt shares the same DetectedStatus value.
func IsSafeSteerStatus(status detection.DetectedStatus, statusContext string) bool {
	return status == detection.StatusIdle && SafeIdleStatusContexts[statusContext]
}

// isClaudeCodeProgram mirrors server/services/backlog_service_pr_fix_steer.go's
// function of the same name (ADR-001's exact-match idiom). Duplicated, not
// imported -- session/diagnose importing server/services cycles even from a
// test file (`go test`: "import cycle not allowed in test"), and the
// original's file is outside this story's scope to move. Kept in sync with
// the original by code review, not an automated check.
func isClaudeCodeProgram(program string) bool {
	return program == "" || program == "claude"
}

// IdleGate implements the idle-settle-window nudge-safety gate (Story 3.2.1,
// resolving MDD #5b): a target session is safe to nudge only when
// IsSafeSteerStatus -- bare detection.StatusIdle narrowed by the
// Claude-Code-specific SafeIdleStatusContexts allowlist -- holds
// CONTINUOUSLY for windowSeconds. detection.StatusReady/StatusSuccess are
// deliberately never treated as idle here (see
// .claude/rules/instance-lock-free-reads.md's sibling precedent on
// detection.StatusReady being dead code); only bare StatusIdle, further
// narrowed by the allowlist, ever passes.
//
// Any single poll observation off the allowlist -- even one poll before the
// window would otherwise have completed -- resets the settle timer to zero.
// There is no partial credit for a near-miss: sustained through the whole
// window, reset on any departure.
//
// One IdleGate instance tracks the settle window for exactly one target
// session; it is not safe to share across sessions or to reuse after the
// session it tracks ends.
type IdleGate struct {
	windowSeconds int
	logger        *slog.Logger

	settleStart time.Time // zero value: not currently within a settle window
}

// NewIdleGate constructs an IdleGate requiring windowSeconds of continuous
// safe-idle status before Evaluate reports safe-to-nudge (windowSeconds is
// expected to come from DiagnoseNudgeConfig.IdleSettleWindowSecondsOrDefault()).
// A nil logger falls back to slog.Default().
func NewIdleGate(windowSeconds int, logger *slog.Logger) *IdleGate {
	if logger == nil {
		logger = slog.Default()
	}
	return &IdleGate{windowSeconds: windowSeconds, logger: logger}
}

// Evaluate advances the settle-window state machine by one poll observation
// and reports whether the window has now been sustained in full. now is
// passed in explicitly (rather than read via time.Now()) so tests can drive
// the state machine deterministically.
//
// A non-Claude-Code program in the pane (Task 3.2.1c) never passes,
// regardless of status -- isClaudeCodeProgram is checked alongside, and with
// equal weight to, the status/context allowlist, not as an afterthought.
func (g *IdleGate) Evaluate(now time.Time, program string, status detection.DetectedStatus, statusContext string) (bool, *SafetyGateReason) {
	safe := isClaudeCodeProgram(program) && IsSafeSteerStatus(status, statusContext)
	if !safe {
		g.reset(now, statusContext)
		reason := SafetyGateReasonNotIdle
		return false, &reason
	}

	if g.settleStart.IsZero() {
		g.settleStart = now
	}

	if now.Sub(g.settleStart) < time.Duration(g.windowSeconds)*time.Second {
		reason := SafetyGateReasonNotIdle
		return false, &reason
	}

	return true, nil
}

// reset clears the settle timer. When the gate had already accrued
// sustained-idle progress (settleStart was running), it logs
// diagnose.idle.settle_reset naming the disqualifying status context and how
// many seconds of progress were lost (pre-mortem.md P1 #1) -- the data
// source for measuring, before DiagnoseNudgeExecutionFeatureFlag is ever
// flipped on, whether the settle window realistically completes against real
// sessions. A reset from an already-idle (zero) settleStart logs nothing --
// there was no progress to lose.
func (g *IdleGate) reset(now time.Time, statusContext string) {
	if !g.settleStart.IsZero() {
		secondsLost := int(now.Sub(g.settleStart).Seconds())
		g.logger.Info("diagnose.idle.settle_reset",
			"status_context", statusContext,
			"seconds_lost", secondsLost,
		)
	}
	g.settleStart = time.Time{}
}
