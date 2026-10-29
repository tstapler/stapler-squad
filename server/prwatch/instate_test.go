package prwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newInStateTracker(t *testing.T) (InboxStateTracker, *fakeNotifier, *fakeClock, Settings) {
	t.Helper()
	s := testNotifySettings()
	s.StateNotifyGrace = 120 * time.Second
	n, c := &fakeNotifier{}, newFakeClock()
	notes := NewNotifications(n, c, s, nil)
	return NewInboxStateTracker(notes, c, &fakeLog{}, s), n, c, s
}

func TestInboxState_should_RaiseOneNotificationWithNextStepAfterGrace_When_InboxUnreachable(t *testing.T) {
	tr, n, c, s := newInStateTracker(t)
	for i := 0; i < 5; i++ { // 0..40s
		tr.Observe(InboxStateUnreachable)
		c.Advance(10 * time.Second)
	}
	require.Empty(t, n.Sent(), "nothing before the grace period")

	c.Advance(80 * time.Second) // 130s since first observation
	tr.Observe(InboxStateUnreachable)
	tr.Observe(InboxStateUnreachable)

	sent := n.Sent()
	require.Len(t, sent, 1)
	assert.Equal(t, NotifyInboxState, sent[0].Kind)
	assert.Equal(t, InboxStateUnreachable, sent[0].State)
	assert.Equal(t, TitleInboxUnreachable, sent[0].Title)
	assert.Contains(t, sent[0].Message, s.URL)
	assert.Contains(t, sent[0].Message, "start the gh-signal-inbox service")
	assert.Contains(t, sent[0].Message, "\nGuide: "+s.GuidePath)
	assert.Contains(t, sent[0].Message, "\nStatus: "+s.StatusURL)
	assert.Equal(t, InboxStateUnreachable, tr.Current())
}

func TestInboxState_should_NotNotify_When_UnreachableRecoversInsideGrace(t *testing.T) {
	tr, n, c, _ := newInStateTracker(t)
	tr.Observe(InboxStateUnreachable)
	c.Advance(60 * time.Second)
	tr.Observe(InboxStateOK)
	c.Advance(100 * time.Second)
	tr.Observe(InboxStateUnreachable) // grace restarts
	assert.Empty(t, n.Sent())
}

func TestInboxState_should_RaiseImmediatelyOncePerState_When_UnauthorizedTokenMissingTokenPermissionsOrNonLoopbackURL(t *testing.T) {
	loose := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(loose, []byte("super-secret-token\n"), 0o644))
	require.NoError(t, os.Chmod(loose, 0o644))

	cases := []struct {
		state     InboxState
		title     string
		contains  []string
		tokenPath string
	}{
		{InboxStateUnauthorized, TitleInboxUnauthorized, []string{"answered 401", "restart gh-signal-inbox"}, ""},
		{InboxStateTokenMissing, TitleTokenMissing, []string{"no readable token file at", "inbox_pr_watch.token_path"}, ""},
		{InboxStateTokenPerms, TitleTokenPermissions, []string{"(mode 0644)", "chmod 600"}, loose},
		{InboxStateNonLoopback, TitleNonLoopbackURL, []string{"not 127.0.0.1, ::1 or localhost", "http://127.0.0.1:7777"}, ""},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			tr, n, _, s := newInStateTracker(t)
			if tc.tokenPath != "" {
				s.TokenPath = tc.tokenPath
				tr = NewInboxStateTracker(NewNotifications(n, newFakeClock(), s, nil), newFakeClock(), &fakeLog{}, s)
			}
			tr.Observe(tc.state)
			tr.Observe(tc.state)
			sent := n.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, tc.title, sent[0].Title)
			assert.Equal(t, tc.state, sent[0].State)
			for _, want := range tc.contains {
				assert.Contains(t, sent[0].Message, want)
			}
			assert.NotContains(t, sent[0].Message, "super-secret-token")
		})
	}
}

func TestInboxState_should_RaiseLeaderNotHeldAfterGrace_When_LockHeldByAnotherProcess(t *testing.T) {
	tr, n, c, s := newInStateTracker(t)
	tr.Observe(InboxStateLeaderNotHeld)
	c.Advance(60 * time.Second)
	tr.Observe(InboxStateLeaderNotHeld)
	require.Empty(t, n.Sent())

	c.Advance(61 * time.Second)
	tr.Observe(InboxStateLeaderNotHeld)
	require.Len(t, n.Sent(), 1)
	assert.Equal(t, TitleLeaderNotHeld, n.Sent()[0].Title)
	assert.Contains(t, n.Sent()[0].Message, s.LockPath)

	tr.Observe(InboxStateOK) // lock acquired
	require.Len(t, n.Sent(), 2)
	assert.Equal(t, TitleRecovered, n.Sent()[1].Title)
	assert.Equal(t, n.Sent()[0].DedupeKey, n.Sent()[1].DedupeKey)
}

func TestInboxState_should_DedupeOnStateAndClearOnRecovery_When_StateRepeatsThenRecovers(t *testing.T) {
	tr, n, c, _ := newInStateTracker(t)
	for i := 0; i < 100; i++ {
		tr.Observe(InboxStateUnauthorized)
		c.Advance(5 * time.Second)
	}
	require.Len(t, n.Sent(), 1)

	tr.Observe(InboxStateOK)
	tr.Observe(InboxStateOK)
	sent := n.Sent()
	require.Len(t, sent, 2, "exactly one recovery update")
	assert.Equal(t, TitleRecovered, sent[1].Title)
	assert.Equal(t, InboxStateOK, sent[1].State)
	assert.Equal(t, sent[0].DedupeKey, sent[1].DedupeKey)
	assert.Contains(t, sent[1].Message, "is resolved.")
	assert.Equal(t, InboxStateOK, tr.Current())

	tr.Observe(InboxStateUnauthorized) // recurrence raises again
	require.Len(t, n.Sent(), 3)
	assert.Equal(t, TitleInboxUnauthorized, n.Sent()[2].Title)
}

func TestInboxState_should_KeepTwoStatesDistinct_When_LeaderStateAndInboxStateBothRaised(t *testing.T) {
	tr, n, c, _ := newInStateTracker(t)
	tr.Observe(InboxStateLeaderNotHeld)
	c.Advance(130 * time.Second)
	tr.Observe(InboxStateLeaderNotHeld)
	tr.Observe(InboxStateTokenMissing) // implies this process polls: leader state clears
	var keys []string
	for _, s := range n.Sent() {
		keys = append(keys, fmt.Sprintf("%s|%s", s.DedupeKey, s.Title))
	}
	assert.Equal(t, []string{
		"prwatch-inbox-state:leader_not_held|" + TitleLeaderNotHeld,
		"prwatch-inbox-state:leader_not_held|" + TitleRecovered,
		"prwatch-inbox-state:token_missing|" + TitleTokenMissing,
	}, keys)
}

func TestInboxState_should_RetryNextTick_When_NotifierFails(t *testing.T) {
	tr, n, _, _ := newInStateTracker(t)
	n.err = fmt.Errorf("bus down")
	tr.Observe(InboxStateTokenMissing)
	n.mu.Lock()
	n.err = nil
	n.mu.Unlock()
	tr.Observe(InboxStateTokenMissing)
	assert.Len(t, n.Sent(), 2, "failed send, then a successful retry")
}
