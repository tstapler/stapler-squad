package server

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/prwatch/inbox"
	"github.com/tstapler/stapler-squad/server/wakes"
)

// countingInbox counts every call; the zero embedded interface panics on
// anything the module should not touch.
type countingInbox struct {
	prwatch.Inbox
	calls atomic.Int64
}

func (c *countingInbox) Subscriptions(context.Context, string) ([]inbox.Subscription, error) {
	c.calls.Add(1)
	return nil, nil
}

type emptySessions struct{ prwatch.Sessions }

func (emptySessions) List(context.Context) ([]prwatch.SessionRef, error) { return nil, nil }

type nopNotifier struct{}

func (nopNotifier) Notify(prwatch.Notification) error { return nil }

type alwaysLeader struct{}

func (alwaysLeader) TryAcquire() (func(), bool, error) { return func() {}, true, nil }

func newTestPRWatchModule(in prwatch.Inbox, leader prwatch.LeaderGuard, enabled func() bool) *prwatchModule {
	return newPRWatchModule(prwatchWiring{
		Settings: prwatch.Settings{ConsumerID: "squad:test", PollInterval: time.Second, ReconcileInterval: time.Minute,
			LeaderRetry: time.Second, HumanFeedbackMode: prwatch.ModeOff},
		Inbox: in, Sessions: emptySessions{}, Notifier: nopNotifier{}, Leader: leader,
		Clock: prwatchClock{}, Log: prwatchLogger{}, Enabled: enabled,
	})
}

func TestPRWatchLifecycle_should_PollOnlyWhileFlagOn_When_FlagFlipsAtRuntime(t *testing.T) {
	in := &countingInbox{}
	var on atomic.Bool
	m := newTestPRWatchModule(in, alwaysLeader{}, on.Load)
	ctx := context.Background()

	m.poller.Tick(ctx)
	assert.Zero(t, in.calls.Load(), "flag off: zero inbox requests")
	assert.False(t, m.Status().Polls)

	on.Store(true)
	m.poller.Tick(ctx)
	assert.Positive(t, in.calls.Load(), "flag on: polls")
	assert.True(t, m.Status().Polls)

	before := in.calls.Load()
	on.Store(false)
	m.poller.Tick(ctx)
	assert.Equal(t, before, in.calls.Load(), "flag off again: no further requests")
	assert.False(t, m.Status().Polls)
}

func TestPRWatchModule_should_NotPollSubscribeOrSteer_When_LeaderLockHeldByAnotherProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prwatch.leader.lock")
	release, ok, err := wakes.FileLeader{Path: path}.TryAcquire()
	require.NoError(t, err)
	require.True(t, ok)
	defer release()

	in := &countingInbox{}
	m := newTestPRWatchModule(in, wakes.FileLeader{Path: path}, func() bool { return true })
	m.poller.Tick(context.Background())

	assert.Zero(t, in.calls.Load())
	assert.False(t, m.Status().Polls)
	assert.False(t, m.Status().LeaderHeld)
}

func TestPRWatchModule_should_BuildLeaderOnConfigDirLockNamePrwatchLeaderLock_When_Started(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	guard, path := prwatchLeaderGuard()
	dir, err := config.GetConfigDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "prwatch.leader.lock"), path)
	assert.Equal(t, wakes.FileLeader{Path: path}, guard)
}

func TestPRWatchSettings_should_MapConfigOntoSettings_When_Resolved(t *testing.T) {
	cs, _ := config.ResolveInboxPRWatch(&config.InboxPRWatchConfig{HumanFeedbackMode: "allowlist"})
	s := prwatchSettings(cs, "box", "http://localhost:8543/x", "/g", "/lock")

	assert.Equal(t, "squad:box", s.ConsumerID)
	assert.Equal(t, config.DefaultInboxURL, s.URL)
	assert.Equal(t, 5*time.Second, s.PollInterval)
	assert.Equal(t, 10*time.Minute, s.CoverageRecheck)
	assert.Equal(t, 24*time.Hour, s.HeldAgeCeiling)
	assert.Equal(t, prwatch.ModeOff, s.HumanFeedbackMode, "allowlist with an empty list maps to the effective mode off")
	assert.Equal(t, "http://localhost:8543/actuator/inbox-pr-watch?format=text", prwatchStatusURL("localhost:8543"))
}
