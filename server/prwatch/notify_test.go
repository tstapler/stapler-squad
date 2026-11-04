package prwatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNotifySettings() Settings {
	return Settings{
		ConsumerID:     "squad:devbox",
		URL:            "http://127.0.0.1:7777",
		TokenPath:      "/home/u/.gh-signal/token",
		LockPath:       "/home/u/.stapler-squad/prwatch.leader.lock",
		StatusURL:      "http://localhost:8543/actuator/inbox-pr-watch?format=text",
		GuidePath:      "/opt/squad/" + GuideRelPath,
		HeldAgeCeiling: 24 * time.Hour,
		LeaderRetry:    30 * time.Second,
	}
}

func newTestNotes() (*Notifications, *fakeNotifier, *fakeClock) {
	n, c := &fakeNotifier{}, newFakeClock()
	return NewNotifications(n, c, testNotifySettings(), nil), n, c
}

func TestNotify_should_RaiseCoverageGapOncePerRepoAndRaiseAgain_When_CoverageReturnsThenIsLost(t *testing.T) {
	notes, n, _ := newTestNotes()
	for _, pr := range []int{1, 2, 3} {
		require.NoError(t, notes.CoverageGap("other/repo", pr, "no admin access to the repository"))
	}
	require.Len(t, n.Sent(), 1)
	assert.Equal(t, "prwatch-coverage:other/repo", n.Sent()[0].DedupeKey)

	notes.CoverageRestored("other/repo")
	require.NoError(t, notes.CoverageGap("other/repo", 1, "no admin access to the repository"))
	require.Len(t, n.Sent(), 2)
}

func TestNotify_should_UseExactWordingWithActionableCommand_When_CoverageGap(t *testing.T) {
	notes, n, _ := newTestNotes()
	require.NoError(t, notes.CoverageGap("other/repo", 42, "no admin access to the repository"))
	got := n.Sent()[0]
	assert.Equal(t, "PR watch: no webhook coverage for other/repo", got.Title)
	assert.Equal(t, "Squad cannot watch PR #42 in other/repo because no GitHub webhook reaches gh-signal for that repository, so sessions on its PRs will not be steered. gh-signal said: no admin access to the repository. Next step: run gh-signal inbox watch other/repo 42 --consumer squad:devbox. To stop this notice instead, add other/repo to inbox_pr_watch.ignore_repos.",
		strings.SplitN(got.Message, "\nGuide:", 2)[0])

	// Hostile reason: escapes and newlines removed, cut to 200 characters.
	notes, n, _ = newTestNotes()
	hostile := "\x1b[31mred\x1b[0m\nline2 " + strings.Repeat("x", 400)
	require.NoError(t, notes.CoverageGap("other/repo", 1, hostile))
	msg := n.Sent()[0].Message
	assert.NotContains(t, msg, "\x1b")
	assert.NotContains(t, strings.SplitN(msg, "\nGuide:", 2)[0], "\n")
	reason := strings.TrimPrefix(strings.SplitN(strings.SplitN(msg, "gh-signal said: ", 2)[1], ". Next step", 2)[0], "")
	assert.LessOrEqual(t, len([]rune(reason)), 200)
	assert.True(t, strings.HasPrefix(reason, "red line2 x"))

	// The next step varies with the reason.
	notes, n, _ = newTestNotes()
	require.NoError(t, notes.CoverageGap("other/repo", 1, "GitHub rate limit exceeded"))
	assert.NotContains(t, n.Sent()[0].Message, "gh-signal inbox watch")
	assert.Contains(t, n.Sent()[0].Message, "checks again every 10 minutes")
}

func TestNotify_should_DedupeByKeyAndClearOnDelivered_When_SteerFailedRepeats(t *testing.T) {
	notes, n, clock := newTestNotes()
	require.NoError(t, notes.SteerFailed("s1", "o/r", 7))
	require.NoError(t, notes.SteerFailed("s1", "o/r", 7))
	require.Len(t, n.Sent(), 1)
	assert.Equal(t, "s1", n.Sent()[0].SessionUUID)
	assert.Equal(t, "prwatch-steer-failed:s1:o/r#7", n.Sent()[0].DedupeKey)

	clock.Advance(11 * time.Minute)
	require.NoError(t, notes.SteerFailed("s1", "o/r", 7))
	require.Len(t, n.Sent(), 2, "allowed again after 10 minutes")

	notes.SteerDelivered("s1", "o/r", 7)
	require.NoError(t, notes.SteerFailed("s1", "o/r", 7))
	require.Len(t, n.Sent(), 3, "cleared by a delivered steer")

	require.NoError(t, notes.SteerFailed("s2", "o/r", 7))
	require.Len(t, n.Sent(), 4, "other session has its own key")
}

func TestNotify_should_StateHoldReasonInWords_When_HeldLongOrDropped(t *testing.T) {
	notes, n, _ := newTestNotes()
	require.NoError(t, notes.HeldLong("s1", "o/r", 7, 61*time.Minute, HoldBusy))
	require.NoError(t, notes.HeldLong("s1", "o/r", 7, 62*time.Minute, HoldBusy))
	require.NoError(t, notes.HeldDropped("s1", "o/r", 7, HoldLoginUnresolved))
	sent := n.Sent()
	require.Len(t, sent, 2)
	assert.Contains(t, sent[0].Message, "has waited 61 minutes because the session is busy. Squad will drop it after 24 hours.")
	assert.Contains(t, sent[1].Message, "waited more than 24 hours because Squad could not determine your GitHub login, and was dropped.")
	notes.SteerDelivered("s1", "o/r", 7)
	require.NoError(t, notes.HeldLong("s1", "o/r", 7, 61*time.Minute, HoldBusy))
	assert.Len(t, n.Sent(), 3)
}

func TestNotify_should_RaiseLoginAndCIOnlyOnce_When_Repeated(t *testing.T) {
	notes, n, _ := newTestNotes()
	require.NoError(t, notes.LoginUnresolved())
	require.NoError(t, notes.LoginUnresolved())
	notes.LoginResolved()
	require.NoError(t, notes.LoginUnresolved())
	require.NoError(t, notes.CIOnlyMode())
	require.NoError(t, notes.CIOnlyMode())
	assert.Len(t, n.Sent(), 3)
	assert.Equal(t, TitleCIOnly, n.Sent()[2].Title)
}

func TestNotify_should_RaiseInboxStateOnceAndRecoverOnce_When_StateComesAndGoes(t *testing.T) {
	notes, n, _ := newTestNotes()
	require.NoError(t, notes.InboxStateRecovered(InboxStateUnreachable))
	assert.Empty(t, n.Sent(), "nothing raised, nothing to recover")
	require.NoError(t, notes.InboxState(InboxStateUnreachable, ""))
	require.NoError(t, notes.InboxState(InboxStateUnreachable, ""))
	require.NoError(t, notes.InboxStateRecovered(InboxStateUnreachable))
	sent := n.Sent()
	require.Len(t, sent, 2)
	assert.Equal(t, "prwatch-inbox-state:unreachable", sent[0].DedupeKey)
	assert.Equal(t, sent[0].DedupeKey, sent[1].DedupeKey)
	assert.Equal(t, TitleRecovered, sent[1].Title)
	require.NoError(t, notes.InboxState(InboxStateUnreachable, ""))
	assert.Len(t, n.Sent(), 3, "recurrence raises again")
	require.NoError(t, notes.InboxState(InboxStateTokenPerms, "0644"))
	assert.Contains(t, n.Sent()[3].Message, "(mode 0644)")
}

func TestNotify_should_ContainOnlyValidatedFieldsAndPlainText_When_AnyKind(t *testing.T) {
	notes, n, _ := newTestNotes()
	st := NewStrangerTracker(notes, newFakeClock(), 0)
	require.NoError(t, notes.CoverageGap("o/r", 1, "denied"))
	require.NoError(t, notes.SteerFailed("s1", "o/r", 1))
	require.NoError(t, notes.HeldLong("s1", "o/r", 1, time.Hour, HoldBusy))
	require.NoError(t, notes.HeldDropped("s1", "o/r", 1, HoldBusy))
	require.NoError(t, notes.LoginUnresolved())
	require.NoError(t, notes.CIOnlyMode())
	for _, s := range []InboxState{InboxStateUnreachable, InboxStateUnauthorized, InboxStateTokenMissing, InboxStateTokenPerms, InboxStateNonLoopback, InboxStateLeaderNotHeld} {
		require.NoError(t, notes.InboxState(s, "0644"))
		require.NoError(t, notes.InboxStateRecovered(s))
	}
	for i := 0; i < 4; i++ {
		st.RecordSteer("o/r", 5, ActorOther)
	}
	require.NotEmpty(t, n.Sent())
	for _, s := range n.Sent() {
		text := s.Title + "\n" + s.Message
		assert.NotContains(t, text, "evil-actor")
		assert.NotContains(t, text, "Hostile session title")
		for _, r := range text {
			assert.True(t, r == '\n' || (r >= ' ' && r < 0x7f), "plain ASCII text only: %q in %q", r, s.Title)
		}
	}
}

func TestNotify_should_EndWithGuideAndStatusPointer_When_AnyKindRaised(t *testing.T) {
	root := t.TempDir()
	guide := filepath.Join(root, "docs", "how-to", "inbox-pr-watch.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(guide), 0o755))
	require.NoError(t, os.WriteFile(guide, []byte("x"), 0o644))

	got := ResolveGuidePath([]string{filepath.Join(root, "bin", "deep")}, nil)
	assert.Equal(t, guide, got)
	_, err := os.Stat(got)
	require.NoError(t, err)
	assert.Equal(t, GuideFallback, ResolveGuidePath([]string{t.TempDir(), "relative/dir", ""}, nil))

	s := testNotifySettings()
	s.GuidePath = got
	n, c := &fakeNotifier{}, newFakeClock()
	notes := NewNotifications(n, c, s, nil)
	require.NoError(t, notes.LoginUnresolved())
	require.NoError(t, notes.SteerFailed("s", "o/r", 1))
	require.NoError(t, notes.InboxState(InboxStateUnreachable, ""))
	st := NewStrangerTracker(notes, c, 3)
	for i := 0; i < 4; i++ {
		st.RecordSteer("o/r", i, ActorOther)
	}
	require.Len(t, n.Sent(), 4)
	for _, x := range n.Sent() {
		assert.True(t, strings.HasSuffix(x.Message, "\nGuide: "+guide+"\nStatus: "+s.StatusURL), x.Title)
	}

	// Relative or empty guide path falls back to the fixed text.
	s.GuidePath = GuideRelPath
	n2 := &fakeNotifier{}
	require.NoError(t, NewNotifications(n2, c, s, nil).LoginUnresolved())
	assert.True(t, strings.HasSuffix(n2.Sent()[0].Message, "\nGuide: "+GuideFallback+"\nStatus: "+s.StatusURL))
}

func TestNotify_should_KeepRawEnumWordsOutOfOperatorText_When_AnyKindAndOutcome(t *testing.T) {
	banned := []string{"guard_busy", "cooling_down", "no_status_source", "not_tracked", "unspecified", "SteerFailed",
		"window_batching", "login_unresolved", "session_suspended", "ci_duplicate_new_sha", "CoverageGap", "HeldLong", "HeldDropped", "StrangerActivity"}
	notes, n, _ := newTestNotes()
	for i, r := range AllHoldReasons() {
		sess := string(rune('a' + i))
		require.NoError(t, notes.HeldLong(sess, "o/r", 1, time.Hour, r))
		require.NoError(t, notes.HeldDropped(sess, "o/r", 1, r))
	}
	require.NoError(t, notes.SteerFailed("s", "o/r", 1))
	require.NoError(t, notes.CoverageGap("o/r", 1, "denied"))
	require.NoError(t, notes.LoginUnresolved())
	require.NoError(t, notes.CIOnlyMode())
	for _, st := range []InboxState{InboxStateUnreachable, InboxStateUnauthorized, InboxStateTokenMissing, InboxStateTokenPerms, InboxStateNonLoopback, InboxStateLeaderNotHeld} {
		require.NoError(t, notes.InboxState(st, "0644"))
	}
	for _, s := range n.Sent() {
		text := s.Title + "\n" + s.Message
		for _, w := range banned {
			assert.NotContains(t, text, w, s.Title)
		}
	}
	require.NoError(t, notes.SteerFailed("s9", "o/r", 2))
	assert.Contains(t, n.Sent()[len(n.Sent())-1].Message, "because the write failed")
}

func TestStrangerWatch_should_RaiseOneNoticePerRepoPerWeek_When_MoreThanThreeOtherClassSteersInRollingSevenDays(t *testing.T) {
	notes, n, clock := newTestNotes()
	sw := NewStrangerTracker(notes, clock, 0)

	for _, c := range []ActorClass{ActorSelf, ActorBot, ActorAllowlisted} {
		for i := 0; i < 5; i++ {
			sw.RecordSteer("o/r", 1, c)
		}
	}
	assert.Equal(t, 0, sw.OtherCount("o/r"))
	assert.Empty(t, n.Sent())

	for i, pr := range []int{12, 10, 12} {
		sw.RecordSteer("o/r", pr, ActorOther)
		assert.Empty(t, n.Sent(), "step %d", i)
	}
	sw.RecordSteer("o/r", 11, ActorOther)
	require.Len(t, n.Sent(), 1)
	got := n.Sent()[0]
	assert.Equal(t, "PR watch: comments from unfamiliar accounts on o/r", got.Title)
	assert.Equal(t, "prwatch-stranger:o/r", got.DedupeKey)
	assert.Contains(t, got.Message, "4 steers on o/r in the last 7 days (PRs #10, #11, #12) were caused by accounts that are not you")
	assert.Equal(t, 4, sw.OtherCount("o/r"))

	sw.RecordSteer("o/r", 13, ActorOther)
	assert.Len(t, n.Sent(), 1, "no second notice inside 7 days")

	for i := 0; i < 4; i++ {
		sw.RecordSteer("other/repo", 1, ActorOther)
	}
	assert.Len(t, n.Sent(), 2, "second repo counted separately")

	clock.Advance(8 * 24 * time.Hour)
	assert.Equal(t, 0, sw.OtherCount("o/r"), "old steers leave the window")
	for i := 0; i < 4; i++ {
		sw.RecordSteer("o/r", 20, ActorOther)
	}
	assert.Len(t, n.Sent(), 3, "raised again once the window emptied")

	notes2, n2, c2 := newTestNotes()
	sw2 := NewStrangerTracker(notes2, c2, 1)
	sw2.RecordSteer("o/r", 1, ActorOther)
	assert.Empty(t, n2.Sent())
	sw2.RecordSteer("o/r", 1, ActorOther)
	assert.Len(t, n2.Sent(), 1, "configured threshold honored")
}
