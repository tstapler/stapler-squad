package qos

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeNow struct{ t time.Time }

func (f *fakeNow) now() time.Time          { return f.t }
func (f *fakeNow) advance(d time.Duration) { f.t = f.t.Add(d) }

func newFake() (*Tracker, *fakeNow) {
	f := &fakeNow{t: time.Unix(1000, 0)}
	return NewTracker(f.now, 5*time.Second), f
}

func TestTracker_should_ExpireInteractive_When_WindowElapses(t *testing.T) {
	t.Parallel()
	tr, f := newFake()
	tr.Note("a")
	assert.True(t, tr.Interactive("a"))
	assert.True(t, tr.AnyInteractive())
	assert.False(t, tr.OtherInteractive("a"))
	assert.True(t, tr.OtherInteractive("b"))
	f.advance(6 * time.Second)
	assert.False(t, tr.Interactive("a"))
	assert.False(t, tr.AnyInteractive())
}

func TestTracker_should_BoundDeferral_When_UserTypesContinuously(t *testing.T) {
	t.Parallel()
	tr, f := newFake()
	tr.Note("a")
	assert.True(t, tr.Defer("poller", 30*time.Second), "first tick yields to typing")
	for i := 0; i < 5; i++ {
		f.advance(5 * time.Second)
		tr.Note("a")
		assert.True(t, tr.Defer("poller", 30*time.Second), "still inside the bound")
	}
	f.advance(5 * time.Second)
	tr.Note("a")
	assert.False(t, tr.Defer("poller", 30*time.Second), "bound reached: runs once")
	assert.True(t, tr.Defer("poller", 30*time.Second), "streak restarts")
}

func TestTracker_should_NeverDefer_When_NobodyIsTyping(t *testing.T) {
	t.Parallel()
	tr, _ := newFake()
	assert.False(t, tr.Defer("poller", time.Minute))
}

func TestTracker_should_NotifyWatchers_OnlyOnTransitions(t *testing.T) {
	t.Parallel()
	tr, f := newFake()
	var calls int
	cancel := tr.Watch(func() { calls++ })
	tr.Note("a")
	tr.Note("a")
	tr.Note("a")
	assert.Equal(t, 1, calls, "repeat keystrokes inside the window are not transitions")
	f.advance(2 * time.Second)
	assert.Equal(t, 3*time.Second, tr.Expire("a"), "window not over yet: no notification")
	assert.Equal(t, 1, calls)
	f.advance(4 * time.Second)
	assert.Equal(t, time.Duration(0), tr.Expire("a"))
	assert.Equal(t, 2, calls, "expiry notifies")
	cancel()
	tr.Note("a")
	assert.Equal(t, 2, calls, "cancelled watcher stays quiet")
}

func TestTracker_should_ClassifyActivity(t *testing.T) {
	t.Parallel()
	tr, _ := newFake()
	assert.Equal(t, ActivityNormal, tr.ActivityOf("a"))
	tr.Note("a")
	assert.Equal(t, ActivityInteractive, tr.ActivityOf("a"))
	assert.Equal(t, ActivityBackground, tr.ActivityOf("b"))
}

func TestTracker_should_DropSession_When_Forgotten(t *testing.T) {
	t.Parallel()
	tr, _ := newFake()
	tr.Note("a")
	tr.Forget("a")
	assert.False(t, tr.AnyInteractive())
}
