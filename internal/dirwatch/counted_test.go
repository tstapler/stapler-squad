package dirwatch

import (
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

type fakeWatcher struct{ adds, removes []string }

func (f *fakeWatcher) Add(n string) error            { f.adds = append(f.adds, n); return nil }
func (f *fakeWatcher) Remove(n string) error         { f.removes = append(f.removes, n); return nil }
func (f *fakeWatcher) Events() <-chan fsnotify.Event { return nil }
func (f *fakeWatcher) Errors() <-chan error          { return nil }
func (f *fakeWatcher) Close() error                  { return nil }

func TestCountedDedupesAddsAndCountsRemovals(t *testing.T) {
	t.Parallel()
	f := &fakeWatcher{}
	c := instrument(f, newConfig([]Option{WithName("t")}))
	require.NoError(t, c.Add("/a/b"))
	require.NoError(t, c.Add("/a/b/"))
	require.NoError(t, c.Add("/a/c"))
	require.Equal(t, 2, c.count())
	require.Len(t, f.adds, 2, "duplicate Add must not reach the underlying watcher")
	require.NoError(t, c.Remove("/a/b"))
	require.Equal(t, 1, c.count())
	require.NoError(t, c.Close())
}

func TestCountedWarnsOncePerCrossingAndRearms(t *testing.T) {
	t.Parallel()
	c := instrument(&fakeWatcher{}, newConfig([]Option{WithName("t"), WithWarnAt(2)}))
	var warns []int
	c.warn = func(_ string, n, _ int) { warns = append(warns, n) }
	for _, p := range []string{"/1", "/2", "/3", "/4"} {
		require.NoError(t, c.Add(p))
	}
	require.Equal(t, []int{3}, warns, "warn once when the count first exceeds the threshold")
	require.NoError(t, c.Remove("/4"))
	require.NoError(t, c.Remove("/3"))
	require.NoError(t, c.Add("/5"))
	require.NoError(t, c.Add("/6"))
	require.Equal(t, []int{3, 3}, warns, "re-armed after dropping back under the threshold")
	require.NoError(t, c.Close())
}
