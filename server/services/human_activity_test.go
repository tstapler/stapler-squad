package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// manualClock is a settable clock for the activity tests.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func newManualClock() *manualClock { return &manualClock{t: time.Unix(1_800_000_000, 0)} }

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// namedPM gives the test instance a tmux session name.
type namedPM struct {
	*replyPM
	name string
}

func (n namedPM) GetSessionIdentifier() string { return n.name }

func namedInstance(t *testing.T, tmuxName string) *session.Instance {
	t.Helper()
	return session.NewStartedInstanceForTest(t, "human-active", namedPM{replyPM: &replyPM{}, name: tmuxName})
}

func TestPaneWriter_ShouldRecordHumanActivity_WhenBrowserInputIsSent(t *testing.T) {
	clock := newManualClock()
	w := &paneWriter{sender: &fakeTmuxSender{}, activity: NewHumanActivity(clock.Now)}

	require.NoError(t, w.SendInput(context.Background(), PaneInput{ControlMode: &recordingControlMode{}, TmuxSession: "ssq_a", Data: []byte("x")}))

	assert.True(t, w.activity.Within("ssq_a", humanActiveWindow))
	assert.False(t, w.activity.Within("ssq_b", humanActiveWindow))
}

func TestHumanActiveGuard_ShouldFollowTheFiveSecondWindow_WhenWebTerminalSentInput(t *testing.T) {
	clock := newManualClock()
	activity := NewHumanActivity(clock.Now)
	guard := NewHumanActiveGuard(activity, nil)
	inst := namedInstance(t, "ssq_a")

	activity.Record("ssq_a")
	clock.Advance(2 * time.Second)
	assert.True(t, guard.Active(context.Background(), inst), "input 2 s ago is active")

	clock.Advance(4 * time.Second)
	assert.False(t, guard.Active(context.Background(), inst), "input 6 s ago is quiet")
}

func TestHumanActiveGuard_ShouldIgnoreControlModeClientsAndCountAHumansAttach(t *testing.T) {
	clock := newManualClock()
	var listed []tmux.ClientActivity
	lister := func(context.Context, string) ([]tmux.ClientActivity, error) { return listed, nil }
	guard := NewHumanActiveGuard(NewHumanActivity(clock.Now), lister)
	inst := namedInstance(t, "ssq_a")

	control := tmux.ClientActivity{Session: "ssq_a", ControlMode: true, Activity: clock.Now().Add(-time.Second)}
	listed = []tmux.ClientActivity{control}
	assert.False(t, guard.Active(context.Background(), inst), "ssq's own control-mode client is not a human")

	listed = []tmux.ClientActivity{control, {Session: "ssq_a", Activity: clock.Now().Add(-2 * time.Second)}}
	assert.True(t, guard.Active(context.Background(), inst), "a plain attached client active 2 s ago is a human")

	listed = []tmux.ClientActivity{{Session: "ssq_other", Activity: clock.Now().Add(-time.Second)}}
	assert.False(t, guard.Active(context.Background(), inst), "a client on another session is not typing here")

	listed = []tmux.ClientActivity{{Session: "ssq_a", Activity: clock.Now().Add(-6 * time.Second)}}
	assert.False(t, guard.Active(context.Background(), inst), "an idle attached client is quiet")
}

func TestHumanActiveGuard_ShouldFallBackToWebTerminalSignal_WhenListClientsFails(t *testing.T) {
	clock := newManualClock()
	activity := NewHumanActivity(clock.Now)
	guard := NewHumanActiveGuard(activity, func(context.Context, string) ([]tmux.ClientActivity, error) {
		return nil, errors.New("no server")
	})
	inst := namedInstance(t, "ssq_a")

	assert.False(t, guard.Active(context.Background(), inst), "quiet web terminal and a failed list: proceed")

	activity.Record("ssq_a")
	assert.True(t, guard.Active(context.Background(), inst), "signal (a) always applies")
}
