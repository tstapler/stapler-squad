package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
)

// ---------------------------------------------------------------------------
// Epic 2.1: reasonSignature type and builder
// ---------------------------------------------------------------------------

func TestBuildReasonSignature_ExtractsHeadersOnly(t *testing.T) {
	fixContext := "## Merge conflict\nRebase onto main.\n\n## Failing CI checks\n- lint\n"

	got := buildReasonSignature(fixContext)

	require.Equal(t, reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}, got)
}

func TestReasonSignature_Equal_IgnoresBodyText(t *testing.T) {
	fixContext1 := "## Failing CI checks\n- lint\n"
	fixContext2 := "## Failing CI checks\n- unit-tests\n"

	sig1 := buildReasonSignature(fixContext1)
	sig2 := buildReasonSignature(fixContext2)

	require.True(t, sig1.equal(sig2), "same header, different body text should compare equal")
}

func TestReasonSignature_HasHeader(t *testing.T) {
	sig := reasonSignature{headers: []string{"## Merge conflict"}}

	require.True(t, sig.hasHeader("## Merge conflict"))
	require.False(t, sig.hasHeader("## Failing CI checks"))
}

func TestBuildReasonSignature_HeaderlessFixContext_DifferentMessagesProduceDifferentSignatures(t *testing.T) {
	// Modeled directly on session/backlog_lifecycle_pr.go's actual
	// header-less fixCtx shape for the "PR closed without merging" call site.
	fixContext1 := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", 42, "https://github.com/tstapler/stapler-squad/pull/42")
	fixContext2 := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", 99, "https://github.com/tstapler/stapler-squad/pull/99")

	sig1 := buildReasonSignature(fixContext1)
	sig2 := buildReasonSignature(fixContext2)

	require.False(t, sig1.equal(sig2), "two different closed PRs must not falsely dedup as the same reason")
}

func TestBuildReasonSignature_HeaderlessFixContext_IdenticalMessagesProduceEqualSignatures(t *testing.T) {
	fixContext1 := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", 42, "https://github.com/tstapler/stapler-squad/pull/42")
	fixContext1Again := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", 42, "https://github.com/tstapler/stapler-squad/pull/42")

	sig1 := buildReasonSignature(fixContext1)
	sig2 := buildReasonSignature(fixContext1Again)

	require.True(t, sig1.equal(sig2), "the same closed PR re-observed on a later tick must still dedup")
}

// TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender pins
// buildReasonSignature's extraction against PRStatus.render()'s actual
// output for all four header-emitting branches, using the exported
// git.ParsePRStatusPayload test-fixture entry point (PRStatus's other
// fields are unexported, so this is the only way to exercise render() from
// outside package git without a struct literal). A future wording change to
// render() fails this test loudly instead of silently changing every
// dedup signature's identity.
func TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender(t *testing.T) {
	raw := []byte(`{
		"statusCheckRollup": [{"__typename": "CheckRun", "name": "lint", "conclusion": "FAILURE", "status": "COMPLETED"}],
		"reviews": [
			{"state": "CHANGES_REQUESTED", "body": "Fix the null check", "author": {"login": "reviewer1"}, "submittedAt": "2026-08-02T14:00:00Z"},
			{"state": "COMMENTED", "body": "Consider extracting this into a helper function.", "author": {"login": "copilot-pull-request-reviewer[bot]"}, "submittedAt": "2026-08-02T14:32:07Z"}
		],
		"comments": [{"body": "Please rebase this branch soon.", "author": {"login": "tstapler"}, "createdAt": "2026-08-02T15:10:00Z"}],
		"mergeable": "CONFLICTING",
		"mergeStateStatus": "DIRTY"
	}`)

	status, err := git.ParsePRStatusPayload(raw)
	require.NoError(t, err)
	require.True(t, status.HasConflicts, "fixture must exercise the HasConflicts branch")
	require.NotEmpty(t, status.FeedbackText)

	sig := buildReasonSignature(status.FeedbackText)

	require.Equal(t, []string{
		"## Merge conflict",
		"## Failing CI checks",
		"## Review: changes requested by @reviewer1",
		"## Reviewer comments",
		"## PR comments",
	}, sig.headers)
}

// ---------------------------------------------------------------------------
// Epic 2.2: Cooldown-based dedup
// ---------------------------------------------------------------------------

func TestIsDuplicateSteerReason_ZeroValue_NotDuplicate(t *testing.T) {
	last := lastSteerReason{}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}

	got := isDuplicateSteerReason(candidate, last, "uuid-1", time.Now(), steerCooldown)

	require.False(t, got)
}

func TestIsDuplicateSteerReason_WithinCooldown_IsDuplicate(t *testing.T) {
	now := time.Now()
	sig := reasonSignature{headers: []string{"## Failing CI checks"}}
	last := lastSteerReason{signature: sig, at: now.Add(-1 * time.Minute), sessionUUID: "uuid-1"}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}

	got := isDuplicateSteerReason(candidate, last, "uuid-1", now, 5*time.Minute)

	require.True(t, got)
}

func TestIsDuplicateSteerReason_AfterCooldown_NotDuplicate(t *testing.T) {
	now := time.Now()
	sig := reasonSignature{headers: []string{"## Failing CI checks"}}
	last := lastSteerReason{signature: sig, at: now, sessionUUID: "uuid-1"}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}

	got := isDuplicateSteerReason(candidate, last, "uuid-1", now.Add(6*time.Minute), 5*time.Minute)

	require.False(t, got)
}

func TestIsDuplicateSteerReason_SessionUUIDChanged_NotDuplicate_EvenWithinCooldown(t *testing.T) {
	now := time.Now()
	sig := reasonSignature{headers: []string{"## Failing CI checks"}}
	last := lastSteerReason{signature: sig, at: now.Add(-1 * time.Minute), sessionUUID: "uuid-1"}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}

	got := isDuplicateSteerReason(candidate, last, "uuid-2", now, 5*time.Minute)

	require.False(t, got, "a session-UUID mismatch must be treated as never-delivered, bypassing cooldown")
}

func TestNextLastSteerReason_NotDelivered_Unchanged(t *testing.T) {
	prev := lastSteerReason{}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}

	got := nextLastSteerReason(prev, candidate, "uuid-1", false)

	require.Equal(t, prev, got)
}

func TestNextLastSteerReason_Delivered_AdvancesAndRecordsSessionUUID(t *testing.T) {
	prev := lastSteerReason{}
	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}
	before := time.Now()

	got := nextLastSteerReason(prev, candidate, "uuid-1", true)

	require.True(t, candidate.equal(got.signature))
	require.Equal(t, "uuid-1", got.sessionUUID)
	require.False(t, got.at.Before(before))
	require.WithinDuration(t, time.Now(), got.at, time.Second)
}

// ---------------------------------------------------------------------------
// Epic 2.3: Conflict two-consecutive-tick debounce
// ---------------------------------------------------------------------------

func TestConfirmConflictChange_FirstTick_NotConfirmed(t *testing.T) {
	last := reasonSignature{headers: []string{"## Failing CI checks"}}
	state := conflictDebounceState{}
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}

	confirmed, next := confirmConflictChange(candidate, last, "uuid-1", state)

	require.False(t, confirmed)
	require.NotNil(t, next.pending)
	require.True(t, next.pending.signature.equal(candidate))
	require.Equal(t, "uuid-1", next.pending.sessionUUID)
}

func TestConfirmConflictChange_SecondConsecutiveTick_Confirmed(t *testing.T) {
	last := reasonSignature{headers: []string{"## Failing CI checks"}}
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(candidate, last, "uuid-1", conflictDebounceState{})

	confirmed, next := confirmConflictChange(candidate, last, "uuid-1", pending)

	require.True(t, confirmed)
	require.Equal(t, conflictDebounceState{}, next)
}

func TestConfirmConflictChange_ConflictResolved_ClearsPending(t *testing.T) {
	last := reasonSignature{headers: []string{"## Failing CI checks"}}
	priorCandidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(priorCandidate, last, "uuid-1", conflictDebounceState{})
	require.NotNil(t, pending.pending)

	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}
	confirmed, next := confirmConflictChange(candidate, last, "uuid-1", pending)

	require.False(t, confirmed)
	require.Equal(t, conflictDebounceState{}, next)
}

func TestConfirmConflictChange_SessionUUIDChanged_RestartsConfirmation(t *testing.T) {
	last := reasonSignature{headers: []string{"## Failing CI checks"}}
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(candidate, last, "uuid-1", conflictDebounceState{})

	confirmed, next := confirmConflictChange(candidate, last, "uuid-2", pending)

	require.False(t, confirmed, "a session change must restart confirmation, not confirm")
	require.NotNil(t, next.pending)
	require.Equal(t, "uuid-2", next.pending.sessionUUID)
}

// Note (Story 2.3.1's fourth GWT — not independently testable as a
// confirmConflictChange unit test, since it's a caller-side condition):
// callers only invoke confirmConflictChange when
// candidate.hasHeader(conflictHeader) && !last.hasHeader(conflictHeader) —
// i.e. the conflict header is newly appearing relative to the last
// *delivered* signature. A tick where the conflict was already present in
// `last` (e.g. CI newly failing alongside an unchanged, already-delivered
// conflict) bypasses this debounce entirely and is gated only by Epic 2.2's
// cooldown/dedup check. See Story 4.2.1 (Phase 4, not implemented in this
// file) for that call-site condition.

// ---------------------------------------------------------------------------
// Epic 3.1: Program-gated message construction with truncation
// ---------------------------------------------------------------------------

func TestBuildSteerMessage_ClaudeProgram_AppendsSuffix(t *testing.T) {
	fixContext := "## Failing CI checks\n- lint\n"

	got := buildSteerMessage("claude", fixContext)

	require.Equal(t, fixContext+prShipSuffix, got)
}

func TestBuildSteerMessage_NonClaudeProgram_NoSuffix(t *testing.T) {
	fixContext := "## Failing CI checks\n- lint\n"

	got := buildSteerMessage("aider", fixContext)

	require.Equal(t, fixContext, got)
}

func TestBuildSteerMessage_OverLength_Truncates(t *testing.T) {
	fixContext := strings.Repeat("x", session.MaxSteerMessageLength+500)

	got := buildSteerMessage("claude", fixContext)

	require.LessOrEqual(t, len(got), session.MaxSteerMessageLength)
	require.Contains(t, got, truncationPointer)
	require.True(t, strings.HasSuffix(got, prShipSuffix))
}

func TestBuildSteerMessage_TruncatedResult_NeverExceedsMaxLength(t *testing.T) {
	for _, program := range []string{"claude", "", "aider", "proxy-claude"} {
		fixContext := strings.Repeat("y", session.MaxSteerMessageLength*2)

		got := buildSteerMessage(program, fixContext)

		require.LessOrEqualf(t, len(got), session.MaxSteerMessageLength, "program=%q", program)
	}
}

// TestBuildSteerMessage_RealisticComposedLongFixContext_SuffixSurvivesTruncation
// guards against a real bug in an earlier draft: appending the suffix
// before truncating and cutting from the tail silently dropped the one
// actionable instruction the steered agent needs. This mirrors
// PRStatus.render()'s actual combined shape (several failing checks plus
// one substantive review comment body), not a synthetic oversized string.
func TestBuildSteerMessage_RealisticComposedLongFixContext_SuffixSurvivesTruncation(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("## Failing CI checks\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, "- integration-test-shard-%d\n", i)
	}
	sb.WriteString("\n## Review: changes requested by @reviewer1\n")
	sb.WriteString(strings.Repeat("This needs a substantial rework of the retry logic. ", 300))
	fixContext := sb.String()
	require.Greater(t, len(fixContext)+len(prShipSuffix), session.MaxSteerMessageLength, "fixture must actually require truncation")

	got := buildSteerMessage("claude", fixContext)

	require.LessOrEqual(t, len(got), session.MaxSteerMessageLength)
	require.True(t, strings.HasSuffix(got, prShipSuffix), "suffix must survive truncation, got %q", got)
}
