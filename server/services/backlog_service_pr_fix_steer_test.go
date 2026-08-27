package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
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
	state := conflictDebounceState{}
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}

	confirmed, next := confirmConflictChange(candidate, "uuid-1", state)

	require.False(t, confirmed)
	require.NotNil(t, next.pending)
	require.True(t, next.pending.signature.equal(candidate))
	require.Equal(t, "uuid-1", next.pending.sessionUUID)
}

func TestConfirmConflictChange_SecondConsecutiveTick_Confirmed(t *testing.T) {
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(candidate, "uuid-1", conflictDebounceState{})

	confirmed, next := confirmConflictChange(candidate, "uuid-1", pending)

	require.True(t, confirmed)
	require.Equal(t, conflictDebounceState{}, next)
}

func TestConfirmConflictChange_ConflictResolved_ClearsPending(t *testing.T) {
	priorCandidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(priorCandidate, "uuid-1", conflictDebounceState{})
	require.NotNil(t, pending.pending)

	candidate := reasonSignature{headers: []string{"## Failing CI checks"}}
	confirmed, next := confirmConflictChange(candidate, "uuid-1", pending)

	require.False(t, confirmed)
	require.Equal(t, conflictDebounceState{}, next)
}

func TestConfirmConflictChange_SessionUUIDChanged_RestartsConfirmation(t *testing.T) {
	candidate := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	_, pending := confirmConflictChange(candidate, "uuid-1", conflictDebounceState{})

	confirmed, next := confirmConflictChange(candidate, "uuid-2", pending)

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

// TestBuildSteerMessage_MultiByteRuneAtTruncationBoundary_NeverSplitsRune guards
// against the naive body[:budget] byte slice buildSteerMessage used before —
// fixContext embeds GitHub reviewer/PR comment bodies verbatim
// (PRStatus.render, session/git/worktree_git.go), which routinely contain
// non-ASCII content (em dashes, curly quotes, emoji, non-English text), so a
// truncation boundary landing mid-rune is a routine failure mode, not a
// synthetic edge case. "€" (U+20AC) is placed so its first byte lands
// exactly at the naive byte budget: body[:budget] would previously cut
// after only that first byte, producing invalid UTF-8 in the PTY-bound
// steer message.
func TestBuildSteerMessage_MultiByteRuneAtTruncationBoundary_NeverSplitsRune(t *testing.T) {
	budget := session.MaxSteerMessageLength - len(prShipSuffix) - len(truncationPointer)
	require.Positive(t, budget)

	fixContext := strings.Repeat("a", budget-1) + "€" + strings.Repeat("b", 200)
	require.Greater(t, len(fixContext), budget, "fixture must actually require truncation")

	got := buildSteerMessage("claude", fixContext)

	require.Truef(t, utf8.ValidString(got), "truncated steer message must be valid UTF-8, got %q", got)
	require.LessOrEqual(t, len(got), session.MaxSteerMessageLength)
	require.True(t, strings.HasSuffix(got, prShipSuffix))
}

// ---------------------------------------------------------------------------
// Epic 4.3: StuckReasonSteerFailed and notifyActiveSessionSteered
// ---------------------------------------------------------------------------

func TestHumanReadableReasonSet_NoHeaders_FallsBackToGenericPhrase(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{})

	require.Equal(t, "a PR problem", got)
}

func TestHumanReadableReasonSet_SingleHeader(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{"## Merge conflict"}})

	require.Equal(t, "a merge conflict", got)
}

func TestHumanReadableReasonSet_TwoHeaders_JoinedWithAnd(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}})

	require.Equal(t, "a merge conflict and failing CI", got)
}

func TestHumanReadableReasonSet_ThreeOrMoreHeaders_OxfordCommaJoined(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{
		"## Merge conflict", "## Failing CI checks", "## Reviewer comments",
	}})

	require.Equal(t, "a merge conflict, failing CI, and reviewer comments", got)
}

// TestHumanReadableReasonSet_FiveHeaders_OxfordCommaJoined exercises the
// full realistic 5-header case PRStatus.render() can emit at once (per
// TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender's fixture)
// directly through humanReadableReasonSet, not just the 3-header case the
// "3+" branch's other test covers.
func TestHumanReadableReasonSet_FiveHeaders_OxfordCommaJoined(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{
		"## Merge conflict",
		"## Failing CI checks",
		"## Review: changes requested by @reviewer1",
		"## Reviewer comments",
		"## PR comments",
	}})

	require.Equal(t, "a merge conflict, failing CI, a blocking review, reviewer comments, and PR comments", got)
}

func TestHumanReadableReasonSet_ReviewHeader_StripsAuthorToGenericPhrase(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{"## Review: changes requested by @reviewer1"}})

	require.Equal(t, "a blocking review", got)
}

// TestHumanReadableReasonSet_MultipleReasons_NamesTheSet and
// TestHumanReadableReasonSet_HeaderlessSignature_FallsBackToGenericPhrase are
// plan.md Task 5.3.1g's literal test names for coverage
// TestHumanReadableReasonSet_TwoHeaders_JoinedWithAnd and
// TestHumanReadableReasonSet_NoHeaders_FallsBackToGenericPhrase above already
// provide — kept as their own named tests so a `go test -run` against either
// exact name (per this repo's naming-as-documentation convention) finds
// something.
func TestHumanReadableReasonSet_MultipleReasons_NamesTheSet(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}})

	require.Equal(t, "a merge conflict and failing CI", got, "both reasons must be named, not just the first")
}

func TestHumanReadableReasonSet_HeaderlessSignature_FallsBackToGenericPhrase(t *testing.T) {
	got := humanReadableReasonSet(reasonSignature{})

	require.Equal(t, "a PR problem", got)
}

// TestHumanReadableReasonSet_HeaderStrings_MatchPRStatusRender pins
// humanReadableReasonSet's switch cases against PRStatus.render()'s actual
// output, mirroring TestBuildReasonSignature_HeaderStrings_MatchPRStatusRender
// (Task 2.1.1d) — a future wording change to render()'s headers should fail
// this test loudly instead of silently making humanReadableReasonSet fall
// through to the empty-phrases branch for a header it no longer recognizes.
func TestHumanReadableReasonSet_HeaderStrings_MatchPRStatusRender(t *testing.T) {
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

	sig := buildReasonSignature(status.FeedbackText)
	got := humanReadableReasonSet(sig)

	require.Equal(t, "a merge conflict, failing CI, a blocking review, reviewer comments, and PR comments", got)
}

func newTestBacklogServiceForSteerNotify(t *testing.T) (*BacklogService, *events.EventBus) {
	t.Helper()
	storage := createTestStorage(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	bus := events.NewEventBus(4)
	svc.SetEventBus(bus)
	return svc, bus
}

func createTestBacklogItemForSteerNotify(t *testing.T, svc *BacklogService) string {
	t.Helper()
	item, err := svc.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{
		Title:  "Steer notify test item",
		Status: string(session.BacklogStatusPRPending),
	})
	require.NoError(t, err)
	return item.ID
}

func openStuckReasons(t *testing.T, svc *BacklogService) map[domain.StuckReason]bool {
	t.Helper()
	open, err := svc.storage.FindOpenStuckStates(context.Background())
	require.NoError(t, err)
	reasons := make(map[domain.StuckReason]bool, len(open))
	for _, row := range open {
		reasons[row.Reason] = true
	}
	return reasons
}

func TestNotifyActiveSessionSteered_Success_PublishesInfoAndDoesNotMarkStuck(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	reason := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reason, nil)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO), ev.NotificationType, "unexpected notification type")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event on success")
	}

	reasons := openStuckReasons(t, svc)
	assert.False(t, reasons[domain.StuckReasonSteerFailed], "success must not mark steer_failed")
	assert.False(t, reasons[domain.StuckReasonRespawnBlockedActive], "success must not leave respawn_blocked_active open")
}

func TestNotifyActiveSessionSteered_Success_ResolvesOpenRespawnBlockedActiveRow(t *testing.T) {
	svc, _ := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()

	_, err := svc.storage.MarkStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive, session.BacklogStatusPRPending, "previously skipped")
	require.NoError(t, err)

	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reasonSignature{headers: []string{"## Failing CI checks"}}, nil)

	reasons := openStuckReasons(t, svc)
	assert.False(t, reasons[domain.StuckReasonRespawnBlockedActive], "a successful steer must resolve a stale respawn_blocked_active row")
}

// TestNotifyActiveSessionSteered_Success_ResolvesOpenSteerFailedRow is the
// mutual-exclusion regression test (adversarial review): a success on a
// later tick must clear a StuckReasonSteerFailed row left open by an
// earlier failed tick, not leave it lingering forever.
func TestNotifyActiveSessionSteered_Success_ResolvesOpenSteerFailedRow(t *testing.T) {
	svc, _ := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()

	_, err := svc.storage.MarkStuck(ctx, itemID, domain.StuckReasonSteerFailed, session.BacklogStatusPRPending, "previous tick failed")
	require.NoError(t, err)

	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reasonSignature{headers: []string{"## Failing CI checks"}}, nil)

	reasons := openStuckReasons(t, svc)
	assert.False(t, reasons[domain.StuckReasonSteerFailed], "success on a later tick must resolve a prior steer_failed row")
}

func TestNotifyActiveSessionSteered_Success_TitleAndBodyNameTheReason(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	reason := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reason, nil)

	ev := requireNotificationEvent(t, ch)
	assert.Equal(t, "Steered active session — My Item has a merge conflict and failing CI", ev.NotificationTitle)
	assert.Contains(t, ev.NotificationMessage, "session-uuid-1")
	assert.Contains(t, ev.NotificationMessage, "a merge conflict and failing CI")
}

func TestNotifyActiveSessionSteered_Failure_MarksStuckAndPublishesWarning(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	deliverErr := fmt.Errorf("SendKeys failed: pty closed")
	reason := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reason, deliverErr)

	select {
	case ev := <-ch:
		assert.Equal(t, events.EventNotification, ev.Type)
		assert.Equal(t, int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING), ev.NotificationType, "unexpected notification type")
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event on failure")
	}

	reasons := openStuckReasons(t, svc)
	assert.True(t, reasons[domain.StuckReasonSteerFailed], "failure must mark steer_failed")
}

func TestNotifyActiveSessionSteered_Failure_ResolvesOpenRespawnBlockedActiveRow(t *testing.T) {
	svc, _ := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()

	_, err := svc.storage.MarkStuck(ctx, itemID, domain.StuckReasonRespawnBlockedActive, session.BacklogStatusPRPending, "previously skipped")
	require.NoError(t, err)

	deliverErr := fmt.Errorf("SendKeys failed: pty closed")
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reasonSignature{headers: []string{"## Failing CI checks"}}, deliverErr)

	reasons := openStuckReasons(t, svc)
	assert.False(t, reasons[domain.StuckReasonRespawnBlockedActive], "a failed steer must resolve a stale respawn_blocked_active row — the two reasons are mutually exclusive")
	assert.True(t, reasons[domain.StuckReasonSteerFailed])
}

func TestNotifyActiveSessionSteered_Failure_TitleAndBodyNameTheReasonAndError(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	deliverErr := fmt.Errorf("SendKeys failed: pty closed")
	reason := reasonSignature{headers: []string{"## Merge conflict", "## Failing CI checks"}}
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "aider", reason, deliverErr)

	ev := requireNotificationEvent(t, ch)
	assert.Equal(t, "Failed to steer active session — My Item needs attention for a merge conflict and failing CI", ev.NotificationTitle)
	assert.Contains(t, ev.NotificationMessage, "session-uuid-1")
	assert.Contains(t, ev.NotificationMessage, "a merge conflict and failing CI")
	assert.Contains(t, ev.NotificationMessage, deliverErr.Error())
}

func TestNotifyActiveSessionSteered_Failure_ClaudeCodeProgram_AppendsRemediationPath(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	deliverErr := fmt.Errorf("SendKeys failed: pty closed")
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", reasonSignature{headers: []string{"## Failing CI checks"}}, deliverErr)

	ev := requireNotificationEvent(t, ch)
	assert.Contains(t, ev.NotificationMessage, " — try /github:pr-ship manually")
}

func TestNotifyActiveSessionSteered_Failure_NonClaudeCodeProgram_NoRemediationPath(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	deliverErr := fmt.Errorf("SendKeys failed: pty closed")
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "aider", reasonSignature{headers: []string{"## Failing CI checks"}}, deliverErr)

	ev := requireNotificationEvent(t, ch)
	assert.NotContains(t, ev.NotificationMessage, "/github:pr-ship")
}

func TestNotifyActiveSessionSteered_HeaderlessReason_FallsBackToGenericPhrase(t *testing.T) {
	svc, bus := newTestBacklogServiceForSteerNotify(t)
	itemID := createTestBacklogItemForSteerNotify(t, svc)
	ctx := context.Background()
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch, _ := bus.Subscribe(subCtx)

	headerless := buildReasonSignature("PR #42 (https://github.com/tstapler/stapler-squad/pull/42) was closed without merging.")
	svc.notifyActiveSessionSteered(ctx, itemID, "My Item", session.BacklogStatusPRPending, "session-uuid-1", "steer message", "claude", headerless, nil)

	ev := requireNotificationEvent(t, ch)
	assert.Contains(t, ev.NotificationTitle, "a PR problem")
}

// requireNotificationEvent drains ch for a single notification event within
// a bounded timeout, failing the test loudly if none arrives — same pattern
// as TestNotifyReworkCapHit_should_stillPublishNotification_When_MarkStuckReturnsError.
func requireNotificationEvent(t *testing.T, ch <-chan *events.Event) *events.Event {
	t.Helper()
	select {
	case ev := <-ch:
		require.Equal(t, events.EventNotification, ev.Type)
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("expected a notification event")
		return &events.Event{}
	}
}
