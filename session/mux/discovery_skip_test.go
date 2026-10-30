package mux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discFixture is a Discovery over a private directory with counting seams, so
// tests assert how often the directory was listed and sockets probed.
type discFixture struct {
	d      *Discovery
	dir    string
	clock  atomic.Int64 // unix nanos
	probes atomic.Int64
	globs  atomic.Int64
	fail   atomic.Bool
}

func newFixture(t *testing.T, sockets ...string) *discFixture {
	t.Helper()
	f := &discFixture{dir: t.TempDir()}
	f.clock.Store(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC).UnixNano())
	for _, name := range sockets {
		f.create(t, name)
	}
	f.d = NewDiscovery()
	f.d.dir = f.dir
	f.d.now = func() time.Time { return time.Unix(0, f.clock.Load()) }
	f.d.probe = func(context.Context, string) (*SessionMetadata, error) {
		f.probes.Add(1)
		if f.fail.Load() {
			return nil, errors.New("not listening")
		}
		return &SessionMetadata{}, nil
	}
	f.d.glob = func(pattern string) ([]string, error) {
		f.globs.Add(1)
		return filepath.Glob(pattern)
	}
	return f
}

func (f *discFixture) create(t *testing.T, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(f.dir, name), nil, 0o600))
}

func (f *discFixture) scan(t *testing.T) []*DiscoveredSession {
	t.Helper()
	got, err := f.d.scanContext(context.Background())
	require.NoError(t, err)
	return got
}

func (f *discFixture) advance(d time.Duration) { f.clock.Add(int64(d)) }

func (f *discFixture) mtime(t *testing.T) time.Time {
	t.Helper()
	info, err := os.Stat(f.dir)
	require.NoError(t, err)
	return info.ModTime()
}

// realChange runs a real directory mutation, then (on coarse-timestamp
// filesystems) keeps creating and removing a scratch file until the directory
// mtime has moved past what the last scan recorded. Never touches mtimes
// directly, so it exercises the same signal production relies on.
func (f *discFixture) realChange(t *testing.T, mutate func()) {
	t.Helper()
	f.d.scan.mu.Lock()
	recorded := f.d.scan.dirMtime
	f.d.scan.mu.Unlock()
	mutate()
	scratch := filepath.Join(f.dir, "scratch.tmp")
	// Eventually runs the closure on another goroutine: no require.* inside.
	advanced := func() bool {
		info, err := os.Stat(f.dir)
		return err == nil && !info.ModTime().Equal(recorded)
	}
	require.Eventually(t, func() bool {
		if advanced() {
			return true
		}
		if os.WriteFile(scratch, nil, 0o600) != nil || os.Remove(scratch) != nil {
			return false
		}
		return advanced()
	}, 10*time.Second, 5*time.Millisecond, "directory mtime never advanced")
}

// The premise of the whole optimisation: creating or removing a socket file is
// visible as a directory mtime change. Retried because a coarse-timestamp
// filesystem can fold two operations into one tick.
func TestDirectoryMtime_should_AdvanceOnCreate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	name := filepath.Join(f.dir, "ssq-mux-x.sock")
	require.Eventually(t, func() bool {
		before := f.mtime(t)
		require.NoError(t, os.WriteFile(name, nil, 0o600))
		if f.mtime(t).After(before) {
			return true
		}
		require.NoError(t, os.Remove(name))
		return false
	}, 5*time.Second, time.Millisecond)
}

func TestDirectoryMtime_should_AdvanceOnRemove(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	name := filepath.Join(f.dir, "ssq-mux-x.sock")
	require.Eventually(t, func() bool {
		require.NoError(t, os.WriteFile(name, nil, 0o600))
		before := f.mtime(t)
		require.NoError(t, os.Remove(name))
		return f.mtime(t).After(before)
	}, 5*time.Second, time.Millisecond)
}

func TestDiscoveryScan_should_NotListOrReprobe_When_DirectoryUnchanged(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock", "ssq-mux-b.sock")

	require.Len(t, f.scan(t), 2)
	require.EqualValues(t, 1, f.globs.Load())
	require.EqualValues(t, 2, f.probes.Load())

	for i := 0; i < 5; i++ {
		assert.Len(t, f.scan(t), 2, "known sessions are still reported on a skipped scan")
	}
	assert.EqualValues(t, 1, f.globs.Load(), "unchanged directory must not be listed again")
	assert.EqualValues(t, 2, f.probes.Load())
}

func TestDiscoveryScan_should_DiscoverNewSocket_When_SocketCreated(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.scan(t)

	f.realChange(t, func() { f.create(t, "ssq-mux-b.sock") })

	assert.Len(t, f.scan(t), 2)
	assert.EqualValues(t, 2, f.globs.Load())
	assert.EqualValues(t, 2, f.probes.Load(), "only the new socket is probed")
}

func TestDiscoveryScan_should_DropSession_When_SocketRemoved(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock", "ssq-mux-b.sock")
	f.scan(t)

	f.realChange(t, func() { require.NoError(t, os.Remove(filepath.Join(f.dir, "ssq-mux-a.sock"))) })

	assert.Len(t, f.scan(t), 1)
	assert.Len(t, f.d.GetSessions(), 1)
}

func TestDiscoveryScan_should_RetryFailedProbe_UpToBudget_ThenStop(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.fail.Store(true)

	for i := 0; i < 10; i++ {
		f.scan(t)
	}
	assert.EqualValues(t, probeRetryBudget, f.probes.Load(),
		"a socket that never answers is probed probeRetryBudget times, not every tick")
	assert.EqualValues(t, probeRetryBudget, f.globs.Load())
}

func TestDiscoveryScan_should_Rescan_When_BackstopElapses(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.fail.Store(true)
	for i := 0; i < probeRetryBudget+2; i++ {
		f.scan(t)
	}
	globsBefore := f.globs.Load()

	f.advance(fullScanBackstop + time.Second)
	f.fail.Store(false)
	got := f.scan(t)

	assert.Equal(t, globsBefore+1, f.globs.Load(), "backstop forces a real scan")
	assert.Len(t, got, 1, "the previously failing socket is picked up")
}

func TestDiscoveryScan_should_FireCallbacks_When_ScansAreSkippedBetweenChanges(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	var mu sync.Mutex
	var events []string
	f.d.OnSessionChange(func(s *DiscoveredSession, isNew bool) {
		mu.Lock()
		defer mu.Unlock()
		kind := "removed"
		if isNew {
			kind = "new"
		}
		events = append(events, kind+":"+filepath.Base(s.SocketPath))
	})
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), events...)
	}

	f.scan(t)
	f.scan(t) // skipped
	f.scan(t) // skipped
	assert.Equal(t, []string{"new:ssq-mux-a.sock"}, snapshot(), "skipped scans fire nothing")

	f.realChange(t, func() { f.create(t, "ssq-mux-b.sock") })
	f.scan(t)
	f.scan(t) // skipped
	f.realChange(t, func() { require.NoError(t, os.Remove(filepath.Join(f.dir, "ssq-mux-a.sock"))) })
	f.scan(t)

	assert.Equal(t, []string{"new:ssq-mux-a.sock", "new:ssq-mux-b.sock", "removed:ssq-mux-a.sock"}, snapshot())
	assert.EqualValues(t, 3, f.globs.Load(), "exactly the three real changes caused listings")
}

func TestDiscoveryScan_should_StampLastSeenWithInjectedClock_When_ScanIsSkipped(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.scan(t)

	f.advance(5 * time.Second) // well inside the backstop
	got := f.scan(t)

	require.Len(t, got, 1)
	assert.Equal(t, time.Unix(0, f.clock.Load()), got[0].LastSeen)
	assert.EqualValues(t, 1, f.globs.Load(), "the scan was skipped")
}

// ScanFromUserOptions's doc promises a following Scan() reconciles against the
// live socket set; that must not be swallowed by the unchanged-directory skip.
func TestDiscoveryScan_should_RunFullScan_When_ScanFromUserOptionsMergedSessions(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.scan(t)
	f.scan(t)
	require.EqualValues(t, 1, f.globs.Load(), "second scan skipped")

	ghost := &DiscoveredSession{SocketPath: filepath.Join(f.dir, "ssq-mux-ghost.sock"), Metadata: &SessionMetadata{}}
	f.d.userOptions = func() ([]*DiscoveredSession, error) { return []*DiscoveredSession{ghost}, nil }
	_, err := f.d.ScanFromUserOptions()
	require.NoError(t, err)
	require.Len(t, f.d.GetSessions(), 2)

	f.scan(t)

	assert.EqualValues(t, 2, f.globs.Load(), "scan after ScanFromUserOptions must list the directory")
	assert.Len(t, f.d.GetSessions(), 1, "the session with no live socket is reconciled away")
}

// A full scan stamps LastSeen from the injected clock for sessions it already
// knew and for ones it just discovered.
func TestDiscoveryScan_should_StampLastSeenWithInjectedClock_When_ScanIsFull(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock")
	f.scan(t)

	f.advance(7 * time.Second)
	f.realChange(t, func() { f.create(t, "ssq-mux-b.sock") })
	got := f.scan(t)

	require.Len(t, got, 2, "full scan: existing and newly discovered session")
	require.EqualValues(t, 2, f.globs.Load())
	want := time.Unix(0, f.clock.Load())
	for _, s := range got {
		assert.Equal(t, want, s.LastSeen, filepath.Base(s.SocketPath))
	}
}

// Run under -race: polling scans, ad-hoc Scan() calls and a mutating
// directory at once.
func TestDiscoveryScan_should_BeRaceFree_When_ScanAndScanContextRunConcurrently(t *testing.T) {
	t.Parallel()
	f := newFixture(t, "ssq-mux-a.sock", "ssq-mux-b.sock")
	f.d.OnSessionChange(func(*DiscoveredSession, bool) {})

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(useScan bool) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if useScan {
					_, err := f.d.Scan()
					assert.NoError(t, err)
				} else {
					_, err := f.d.scanContext(context.Background())
					assert.NoError(t, err)
				}
			}
		}(i%2 == 0)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			name := filepath.Join(f.dir, "ssq-mux-churn.sock")
			assert.NoError(t, os.WriteFile(name, nil, 0o600))
			assert.NoError(t, os.Remove(name))
		}
	}()
	wg.Wait()
}
