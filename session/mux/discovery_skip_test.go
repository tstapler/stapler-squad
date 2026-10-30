package mux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type probeRecorder struct {
	calls atomic.Int64
	fail  atomic.Bool
}

func (p *probeRecorder) probe(_ context.Context, _ string) (*SessionMetadata, error) {
	p.calls.Add(1)
	if p.fail.Load() {
		return nil, errors.New("not listening")
	}
	return &SessionMetadata{}, nil
}

func newTestDiscovery(t *testing.T, sockets ...string) (*Discovery, *probeRecorder, string, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range sockets {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	rec := &probeRecorder{}
	clock := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	d := NewDiscovery()
	d.dir, d.probe, d.now = dir, rec.probe, func() time.Time { return clock }
	return d, rec, dir, &clock
}

func TestDiscoveryScan_should_NotReprobe_When_DirectoryUnchanged(t *testing.T) {
	t.Parallel()
	d, rec, _, _ := newTestDiscovery(t, "ssq-mux-a.sock", "ssq-mux-b.sock")

	first, err := d.scanContext(context.Background())
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.EqualValues(t, 2, rec.calls.Load())

	for i := 0; i < 5; i++ {
		again, err := d.scanContext(context.Background())
		require.NoError(t, err)
		assert.Len(t, again, 2, "known sessions are still reported on a skipped scan")
	}
	assert.EqualValues(t, 2, rec.calls.Load(), "unchanged directory must not re-probe")
}

func TestDiscoveryScan_should_DiscoverNewSocket_When_DirectoryChanges(t *testing.T) {
	t.Parallel()
	d, rec, dir, _ := newTestDiscovery(t, "ssq-mux-a.sock")
	_, err := d.scanContext(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssq-mux-b.sock"), nil, 0o600))
	// Guarantee a distinct mtime even on coarse-granularity filesystems.
	require.NoError(t, os.Chtimes(dir, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))

	got, err := d.scanContext(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.EqualValues(t, 2, rec.calls.Load(), "only the new socket is probed")
}

func TestDiscoveryScan_should_DropSession_When_SocketRemoved(t *testing.T) {
	t.Parallel()
	d, _, dir, _ := newTestDiscovery(t, "ssq-mux-a.sock", "ssq-mux-b.sock")
	_, err := d.scanContext(context.Background())
	require.NoError(t, err)

	require.NoError(t, os.Remove(filepath.Join(dir, "ssq-mux-a.sock")))
	require.NoError(t, os.Chtimes(dir, time.Now().Add(time.Hour), time.Now().Add(time.Hour)))

	got, err := d.scanContext(context.Background())
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Len(t, d.GetSessions(), 1)
}

func TestDiscoveryScan_should_RetryFailedProbe_UpToBudget_ThenStop(t *testing.T) {
	t.Parallel()
	d, rec, _, _ := newTestDiscovery(t, "ssq-mux-a.sock")
	rec.fail.Store(true)

	for i := 0; i < 10; i++ {
		_, err := d.scanContext(context.Background())
		require.NoError(t, err)
	}
	assert.EqualValues(t, probeRetryBudget, rec.calls.Load(),
		"a socket that never answers is probed probeRetryBudget times, not every tick")
}

func TestDiscoveryScan_should_Rescan_When_BackstopElapses(t *testing.T) {
	t.Parallel()
	d, rec, _, clock := newTestDiscovery(t, "ssq-mux-a.sock")
	rec.fail.Store(true)
	for i := 0; i < probeRetryBudget+2; i++ {
		_, err := d.scanContext(context.Background())
		require.NoError(t, err)
	}
	before := rec.calls.Load()

	*clock = clock.Add(fullScanBackstop + time.Second)
	rec.fail.Store(false)
	got, err := d.scanContext(context.Background())
	require.NoError(t, err)

	assert.Greater(t, rec.calls.Load(), before, "backstop forces a real scan")
	assert.Len(t, got, 1, "the previously failing socket is picked up")
}
