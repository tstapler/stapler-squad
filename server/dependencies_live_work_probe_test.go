package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type noLiveSessions struct{}

func (noLiveSessions) LiveSnapshot(string) (*session.InstanceSnapshot, bool) { return nil, false }

func newWiringFixtures(t *testing.T) (*session.BacklogLifecycleListener, *services.BacklogService, *session.Storage) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return session.NewBacklogLifecycleListener(storage), services.NewBacklogService(storage, nil, nil, nil, nil, nil), storage
}

// The real dependency wiring seam shares ONE non-nil probe between the lifecycle
// listener, BacklogService and its cleanup gate, and exports the gauge.
func TestWireLiveWorkProbe_ShouldShareOneProbeAcrossListenerServiceAndGate(t *testing.T) {
	listener, backlog, storage := newWiringFixtures(t)

	require.NoError(t, wireLiveWorkProbe(listener, backlog, noLiveSessions{}, storage))

	require.NotNil(t, listener.LiveWorkProbe())
	assert.Same(t, listener.LiveWorkProbe(), backlog.LiveWorkProbe())
	assert.Same(t, backlog.LiveWorkProbe(), backlog.CleanupGate().Probe())
	assert.True(t, session.LiveWorkProbeWired(), "ssq_live_work_probe_wired is 1")
}

func TestAssertLiveWorkProbeShared_ShouldFail_WhenAnyConsumerIsMissingOrDiffers(t *testing.T) {
	listener, backlog, storage := newWiringFixtures(t)
	require.Error(t, assertLiveWorkProbeShared(listener, backlog), "nothing wired: a missed site would silently revert to kill-at-done")

	probe := session.NewSnapshotLiveWorkProbe(noLiveSessions{}, storage)
	listener.SetLiveWorkProbe(probe)
	err := assertLiveWorkProbeShared(listener, backlog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backlog service=false")

	backlog.SetLiveWorkProbe(session.NewSnapshotLiveWorkProbe(noLiveSessions{}, storage))
	err = assertLiveWorkProbeShared(listener, backlog)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "differ")

	backlog.SetLiveWorkProbe(probe)
	require.NoError(t, assertLiveWorkProbeShared(listener, backlog))
}
