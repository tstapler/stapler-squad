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

// eventually runs trigger, then waits briefly for a matching event, retrying so a
// slow or briefly unregistered FSEvents stream cannot fail the test spuriously.
func eventually(t *testing.T, w Watcher, trigger func(i int), match func(fsnotify.Event) bool) fsnotify.Event {
	t.Helper()
	deadline := time.After(60 * time.Second)
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
