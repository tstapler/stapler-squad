package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsTerminalStatus_should_ReturnTrue_When_StatusIsBuiltInDoneOrArchived is
// the regression test that Epic 2.1's consolidation preserves the pre-existing
// `status == BacklogStatusDone || status == BacklogStatusArchived` behavior
// exactly, with no checker wired (today's only production state).
func TestIsTerminalStatus_should_ReturnTrue_When_StatusIsBuiltInDoneOrArchived(t *testing.T) {
	assert.True(t, IsTerminalStatus(BacklogStatusDone))
	assert.True(t, IsTerminalStatus(BacklogStatusArchived))
}

// TestIsTerminalStatus_should_ReturnFalse_When_StatusIsAnyOtherBuiltIn covers
// every other built-in status to confirm none of them regressed to terminal.
func TestIsTerminalStatus_should_ReturnFalse_When_StatusIsAnyOtherBuiltIn(t *testing.T) {
	nonTerminal := []BacklogStatus{
		BacklogStatusIdea, BacklogStatusRefining, BacklogStatusReady,
		BacklogStatusQueued, BacklogStatusInProgress, BacklogStatusReview,
		BacklogStatusPRPending,
	}
	for _, status := range nonTerminal {
		assert.False(t, IsTerminalStatus(status), "%s must not be terminal", status)
	}
}

// TestIsTerminalStatus_should_ReturnFalse_When_CustomStatusHasNoCheckerWired
// covers an unrecognized (custom) status with no SetTerminalStatusChecker
// wired — the built-in-only fallback per this decision's default state.
func TestIsTerminalStatus_should_ReturnFalse_When_CustomStatusHasNoCheckerWired(t *testing.T) {
	assert.False(t, IsTerminalStatus(BacklogStatus("legal-review")))
}

// TestIsTerminalStatus_should_ReturnTrue_When_ConfiguredCheckerMarksCustomStatusTerminal
// is the Story 2.1.3 (Task 2.1.3d) regression test: once
// SetTerminalStatusChecker is wired (simulating Epic 2.3's
// ConfiguredWorkflowEngine), a custom stage it marks IsTerminal must be
// recognized, and reverting to nil restores the built-in-only fallback.
func TestIsTerminalStatus_should_ReturnTrue_When_ConfiguredCheckerMarksCustomStatusTerminal(t *testing.T) {
	const customTerminal = BacklogStatus("legal-review")
	SetTerminalStatusChecker(func(s BacklogStatus) bool { return s == customTerminal })
	t.Cleanup(func() { SetTerminalStatusChecker(nil) })

	assert.True(t, IsTerminalStatus(customTerminal))
	assert.False(t, IsTerminalStatus(BacklogStatus("other-custom-stage")), "the checker only marks the one configured slug terminal")

	// Built-ins are unaffected by a wired checker — the OR short-circuits
	// before ever consulting it.
	assert.True(t, IsTerminalStatus(BacklogStatusDone))
	assert.True(t, IsTerminalStatus(BacklogStatusArchived))
}

// TestTerminalStatusStrings_should_ReturnExactlyDoneAndArchived documents
// TerminalStatusStrings' deliberate built-in-only scope (see its doc
// comment) — even with a checker wired, it must not attempt to enumerate
// custom stages, since ent query-builder call sites need a literal list, not
// a per-status predicate.
func TestTerminalStatusStrings_should_ReturnExactlyDoneAndArchived(t *testing.T) {
	SetTerminalStatusChecker(func(BacklogStatus) bool { return true })
	t.Cleanup(func() { SetTerminalStatusChecker(nil) })

	got := TerminalStatusStrings()
	require.ElementsMatch(t, []string{string(BacklogStatusDone), string(BacklogStatusArchived)}, got)
}
