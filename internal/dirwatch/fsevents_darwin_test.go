//go:build darwin

package dirwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/internal/fsevents"
)

func openFDs(t *testing.T) int {
	t.Helper()
	f, err := os.Open("/dev/fd")
	require.NoError(t, err)
	defer f.Close()
	names, err := f.Readdirnames(-1)
	require.NoError(t, err)
	return len(names)
}

// populate makes dirs sibling directories holding files regular files each.
func populate(t *testing.T, dirs, files int) (root string, paths []string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for i := 0; i < dirs; i++ {
		d := filepath.Join(root, fmt.Sprintf("d%03d", i))
		require.NoError(t, os.Mkdir(d, 0o755))
		for j := 0; j < files; j++ {
			require.NoError(t, os.WriteFile(filepath.Join(d, fmt.Sprintf("f%d", j)), nil, 0o644))
		}
		paths = append(paths, d)
	}
	return root, paths
}

// Not parallel: it measures the process-wide fd table.
func TestFDFreeWatcherHoldsNoFDPerWatchedFile(t *testing.T) {
	const dirs, files = 100, 8
	root, paths := populate(t, dirs, files)

	before := openFDs(t)
	kq, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	for _, p := range paths {
		require.NoError(t, kq.Add(p))
	}
	kqueueCost := openFDs(t) - before
	require.NoError(t, kq.Close())
	require.GreaterOrEqual(t, kqueueCost, dirs*files, "premise: kqueue holds an fd per file in every watched dir")

	w, ok := NewFDFree(WithRootFunc(func(string) string { return root }))
	require.True(t, ok)
	// The first Add pays one-time CoreServices/stream setup; measure the marginal cost after it.
	require.NoError(t, w.Add(paths[0]))
	before = openFDs(t)
	for _, p := range paths[1:] {
		require.NoError(t, w.Add(p))
	}
	fseventsCost := openFDs(t) - before
	require.NoError(t, w.Close())
	require.LessOrEqual(t, fseventsCost, 1, "FSEvents watcher must not hold an fd per watched dir/file (kqueue cost %d)", kqueueCost)
}

// waitEvent triggers once and waits for a match; for the kqueue fallback, which is reliable.
func waitEvent(t *testing.T, w Watcher, match func(fsnotify.Event) bool, trigger func()) {
	t.Helper()
	trigger()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case e, ok := <-w.Events():
			require.True(t, ok, "events channel closed")
			if match(e) {
				return
			}
		case <-deadline:
			t.Fatal("no matching event from the kqueue fallback")
		}
	}
}

// eventually runs trigger, then waits briefly for a matching event, retrying so a
// slow or briefly unregistered FSEvents stream cannot fail the test spuriously.
func eventually(t *testing.T, w Watcher, trigger func(i int), match func(fsnotify.Event) bool) fsnotify.Event {
	t.Helper()
	deadline := time.After(20 * time.Second)
	var all []fsnotify.Event
	for i := 0; ; i++ {
		trigger(i)
		retry := time.After(500 * time.Millisecond)
	wait:
		for {
			select {
			case e, ok := <-w.Events():
				require.True(t, ok, "events channel closed")
				all = append(all, e)
				if match(e) {
					return e
				}
			case <-retry:
				break wait
			case <-deadline:
				skipIfHostDropsFSEvents(t)
				t.Fatalf("no matching event; got %v", all)
			}
		}
	}
}

func TestFDFreeWatcherReportsDirectChildrenOnly(t *testing.T) {
	_, paths := populate(t, 2, 0)
	watched, other := paths[0], paths[1]
	w, ok := NewFDFree()
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(watched))

	nested := filepath.Join(watched, "sub")
	require.NoError(t, os.Mkdir(nested, 0o755))
	want := filepath.Join(watched, "HEAD")
	seen := map[string]bool{}
	eventually(t, w, func(i int) {
		content := []byte(fmt.Sprint(i))
		require.NoError(t, os.WriteFile(filepath.Join(nested, "deep"), content, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(other, "unwatched"), content, 0o644))
		require.NoError(t, os.WriteFile(want, content, 0o644))
	}, func(e fsnotify.Event) bool {
		seen[e.Name] = true
		return e.Name == want
	})
	require.False(t, seen[filepath.Join(nested, "deep")], "grandchild events must not leak through")
	require.False(t, seen[filepath.Join(other, "unwatched")], "unwatched sibling must not report")
}

func TestFDFreeWatcherReportsWatchedDirRemoval(t *testing.T) {
	_, paths := populate(t, 1, 1)
	w, ok := NewFDFree()
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(paths[0]))
	eventually(t, w, func(int) {
		require.NoError(t, os.RemoveAll(paths[0]))
		require.NoError(t, os.Mkdir(paths[0], 0o755))
	}, func(e fsnotify.Event) bool { return e.Name == paths[0] && e.Has(fsnotify.Remove) })
}

func TestFDFreeWatcherRemoveStopsStreamAndClosesChannels(t *testing.T) {
	_, paths := populate(t, 2, 0)
	w, ok := NewFDFree()
	require.True(t, ok)
	require.NoError(t, w.Add(paths[0]))
	require.NoError(t, w.Add(paths[1]))
	require.NoError(t, w.Remove(paths[0]))
	require.ErrorIs(t, w.Remove(paths[0]), fsnotify.ErrNonExistentWatch)
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	require.Len(t, fw.streams, 1, "one disjoint stream remains")
	fw.mu.Unlock()
	require.NoError(t, w.Remove(paths[1]))
	fw.mu.Lock()
	require.Empty(t, fw.streams)
	fw.mu.Unlock()
	require.NoError(t, w.Close())
	_, open := <-w.Events()
	require.False(t, open)
}

func TestFDFreeWatcherMergesNarrowStreamsUnderNewAncestor(t *testing.T) {
	root, paths := populate(t, 2, 0)
	w, ok := NewFDFree()
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(paths[0]))
	require.NoError(t, w.Add(root))
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	require.Len(t, fw.streams, 1)
	fw.mu.Unlock()
	want := filepath.Join(paths[0], "HEAD")
	eventually(t, w, func(i int) {
		require.NoError(t, os.WriteFile(want, []byte(fmt.Sprint(i)), 0o644))
	}, func(e fsnotify.Event) bool { return e.Name == want })
}

// Not parallel: the stream count is process-wide.
func TestFDFreeWatcherFallsBackToKqueueWhenStreamBudgetIsSpent(t *testing.T) {
	_, paths := populate(t, 6, 1)
	base := int(fsevents.LiveStreams())
	w, ok := NewFDFree(WithMaxStreams(base + 3))
	require.True(t, ok)
	for _, p := range paths {
		require.NoError(t, w.Add(p), "a spent stream budget must degrade, not fail the Add")
	}
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	require.Len(t, fw.streams, 3)
	require.Len(t, fw.fbNames, 3)
	fw.mu.Unlock()

	want := filepath.Join(paths[5], "HEAD") // beyond the budget: served by kqueue
	waitEvent(t, w, func(e fsnotify.Event) bool { return e.Name == want }, func() {
		require.NoError(t, os.WriteFile(want, []byte("x"), 0o644))
	})
	require.NoError(t, w.Remove(paths[5]))
	require.NoError(t, w.Close())
	require.Equal(t, base, int(fsevents.LiveStreams()), "Close must release every stream")
}

func TestFDFreeWatcherSharesOneStreamAcrossManySiblingDirs(t *testing.T) {
	root, paths := populate(t, 600, 0) // 600 > the ~512 per-process stream limit
	w, ok := NewFDFree(RootAt(root))
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	for _, p := range paths {
		require.NoError(t, w.Add(p))
	}
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	defer fw.mu.Unlock()
	require.Len(t, fw.streams, 1)
	require.Empty(t, fw.fbNames)
}

func TestFDFreeWatcherKeepsStreamUntilLastAliasIsRemoved(t *testing.T) {
	_, paths := populate(t, 1, 0)
	alias := paths[0] + "-alias"
	require.NoError(t, os.Symlink(paths[0], alias))
	w, ok := NewFDFree()
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(paths[0]))
	require.NoError(t, w.Add(alias))
	require.NoError(t, w.Remove(paths[0]))
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	require.Len(t, fw.streams, 1, "alias still watches the path")
	fw.mu.Unlock()
	require.NoError(t, w.Remove(alias))
	fw.mu.Lock()
	require.Empty(t, fw.streams)
	fw.mu.Unlock()
}

func TestOpsForRenameOntoExistingPathIsAlsoCreate(t *testing.T) {
	_, paths := populate(t, 1, 1)
	existing := filepath.Join(paths[0], "f0")
	require.True(t, opsFor(fsevents.ItemRenamed, existing).Has(fsnotify.Create), "lock-file rename onto HEAD must look like a Create")
	require.False(t, opsFor(fsevents.ItemRenamed, filepath.Join(paths[0], "gone")).Has(fsnotify.Create))
}

func TestDroppedEventsEscalateOnlyWhenTheyCanTouchAWatchedPath(t *testing.T) {
	root, paths := populate(t, 1, 0)
	w, ok := NewFDFree()
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(paths[0]))
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	defer fw.mu.Unlock()
	require.True(t, fw.touchesWatchedLocked(paths[0]))
	require.True(t, fw.touchesWatchedLocked(filepath.Join(paths[0], "child")))
	require.True(t, fw.touchesWatchedLocked(root), "an ancestor of a watched dir")
	require.False(t, fw.touchesWatchedLocked(filepath.Join(paths[0], "a", "b", "objects")), "deep churn below a watched dir")
	require.False(t, fw.touchesWatchedLocked(filepath.Join(root, "elsewhere")))
}

// skipIfHostDropsFSEvents skips when a bare FSEvents stream on a fresh dir also
// sees nothing: a host with fseventsd saturated (observed at load average ~120
// with many concurrent builds) cannot tell us anything about this package.
func skipIfHostDropsFSEvents(t *testing.T) {
	t.Helper()
	_, paths := populate(t, 1, 0)
	got := make(chan struct{}, 1)
	s, err := fsevents.NewStream(fsevents.Config{Paths: []string{paths[0]}, SinceWhen: fsevents.SinceNow, OnEvents: func([]fsevents.Event) {
		select {
		case got <- struct{}{}:
		default:
		}
	}})
	require.NoError(t, err)
	require.NoError(t, s.Start())
	defer s.Stop()
	deadline := time.After(10 * time.Second)
	for i := 0; ; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(paths[0], "canary"), []byte(fmt.Sprint(i)), 0o644))
		select {
		case <-got:
			return
		case <-time.After(500 * time.Millisecond):
		case <-deadline:
			t.Skip("host FSEvents delivered nothing to a bare canary stream; cannot judge dirwatch here")
		}
	}
}

// Not parallel: it measures the process-wide fd table. Each stream used to hold
// ~14 fds through kFSEventStreamCreateFlagWatchRoot (4,206 fds at 300 streams).
func TestFDFreeStreamsHoldNoFDsEach(t *testing.T) {
	_, paths := populate(t, 40, 0)
	w, ok := NewFDFree(WithMaxStreams(1000))
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	require.NoError(t, w.Add(paths[0])) // first stream pays one-time CoreServices setup
	before := openFDs(t)
	for _, p := range paths[1:] {
		require.NoError(t, w.Add(p))
	}
	fw := w.(*fseventsWatcher)
	fw.mu.Lock()
	streams := len(fw.streams)
	fw.mu.Unlock()
	require.Equal(t, 40, streams, "premise: one stream per disjoint dir")
	require.LessOrEqual(t, openFDs(t)-before, 2, "39 extra streams must not hold fds")
}

// The scanner's dropRemovedAdmin depends on a pruned admin dir reporting Remove
// when its stream is rooted at the parent (.git/worktrees) and WatchRoot is off.
func TestFDFreeWatcherReportsChildDirRemovalOnSharedParentStream(t *testing.T) {
	root, paths := populate(t, 2, 1)
	w, ok := NewFDFree(RootAt(root))
	require.True(t, ok)
	t.Cleanup(func() { _ = w.Close() })
	for _, p := range paths {
		require.NoError(t, w.Add(p))
	}
	eventually(t, w, func(int) {
		require.NoError(t, os.RemoveAll(paths[1]))
		require.NoError(t, os.Mkdir(paths[1], 0o755))
	}, func(e fsnotify.Event) bool { return e.Name == paths[1] && e.Has(fsnotify.Remove) })
}
