package prwatch

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
)

// pollOnce is the small stand-in for the poll loop: one real client call, its
// error mapped onto a state and fed to the real tracker.
func pollOnce(t *testing.T, cfg inbox.Config, tr InboxStateTracker) {
	t.Helper()
	c, err := inbox.NewClient(cfg) // reads the token fresh, like the loop does per tick
	if err != nil {
		tr.Observe(StateFromError(err))
		return
	}
	_, err = c.Events(t.Context(), 10)
	tr.Observe(StateFromError(err))
}

func TestInStateTracker_should_RaiseUnauthorizedNotificationAndClear_When_TokenRotatesAfter401OverRealClient(t *testing.T) {
	f := newFakeHTTPInbox(t)
	s := testNotifySettings()
	s.URL, s.TokenPath = f.URL, f.TokenPath
	s.StateNotifyGrace = 120 * time.Second
	n, clock := &fakeNotifier{}, newFakeClock()
	tr := NewInboxStateTracker(NewNotifications(n, clock, s, nil), clock, &fakeLog{}, s)
	cfg := f.Config("squad:devbox")

	pollOnce(t, cfg, tr)
	require.Empty(t, n.Sent(), "healthy inbox raises nothing")
	assert.Equal(t, InboxStateOK, tr.Current())

	// The inbox restarts with a new token; Squad's token file still holds the old one.
	f.Token = "rotated-token"
	for i := 0; i < 5; i++ {
		pollOnce(t, cfg, tr)
		clock.Advance(5 * time.Second)
	}
	require.Len(t, n.Sent(), 1, "one notification however many 401s")
	assert.Equal(t, InboxStateUnauthorized, n.Sent()[0].State)
	assert.Equal(t, TitleInboxUnauthorized, n.Sent()[0].Title)
	assert.Equal(t, InboxStateUnauthorized, tr.Current())
	assert.NotContains(t, n.Sent()[0].Message, f.Token)

	// The operator rewrites the token file; the next request succeeds.
	require.NoError(t, os.WriteFile(f.TokenPath, []byte(f.Token+"\n"), 0o600))
	pollOnce(t, cfg, tr)
	sent := n.Sent()
	require.Len(t, sent, 2)
	assert.Equal(t, TitleRecovered, sent[1].Title)
	assert.Equal(t, sent[0].DedupeKey, sent[1].DedupeKey)
	assert.Equal(t, InboxStateOK, tr.Current())

	pollOnce(t, cfg, tr)
	assert.Len(t, n.Sent(), 2, "no further notifications while healthy")
}

func TestInStateTracker_should_RaiseTokenMissingThenPermissions_When_TokenFileBreaksOverRealClient(t *testing.T) {
	f := newFakeHTTPInbox(t)
	s := testNotifySettings()
	s.URL, s.TokenPath = f.URL, f.TokenPath
	n, clock := &fakeNotifier{}, newFakeClock()
	tr := NewInboxStateTracker(NewNotifications(n, clock, s, nil), clock, &fakeLog{}, s)
	cfg := f.Config("squad:devbox")

	require.NoError(t, os.Chmod(f.TokenPath, 0o644))
	pollOnce(t, cfg, tr)
	require.Len(t, n.Sent(), 1)
	assert.Equal(t, TitleTokenPermissions, n.Sent()[0].Title)
	assert.Contains(t, n.Sent()[0].Message, "(mode 0644)")

	require.NoError(t, os.Remove(f.TokenPath))
	pollOnce(t, cfg, tr)
	titles := make([]string, 0, len(n.Sent()))
	for _, x := range n.Sent() {
		titles = append(titles, x.Title)
	}
	assert.Equal(t, []string{TitleTokenPermissions, TitleRecovered, TitleTokenMissing}, titles)
}
