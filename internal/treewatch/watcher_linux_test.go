//go:build linux

package treewatch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// recorder collects every delivered event.
type recorder struct {
	mu  sync.Mutex
	evs []fsevents.Event
}

func (r *recorder) on(b []fsevents.Event) {
	r.mu.Lock()
	r.evs = append(r.evs, b...)
	r.mu.Unlock()
}

func (r *recorder) saw(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.evs {
		if e.Path == path {
			return true
		}
	}
	return false
}

func start(t *testing.T, root string) *recorder {
	t.Helper()
	r := &recorder{}
	w, err := Start(Config{Root: root, OnEvents: r.on})
	require.NoError(t, err)
	t.Cleanup(w.Stop)
	return r
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 5*time.Second, 5*time.Millisecond)
}

func TestWatchesFilesInRootAndSubdirs(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0o755))
	r := start(t, root)

	deep := filepath.Join(root, "a", "b", "f.go")
	require.NoError(t, os.WriteFile(deep, []byte("x"), 0o644))
	eventually(t, func() bool { return r.saw(deep) })
}

func TestWatchesDirectoriesCreatedLater(t *testing.T) {
	root := t.TempDir()
	r := start(t, root)

	dir := filepath.Join(root, "new", "sub")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	// Adding the watch for "new" races MkdirAll's creation of "sub", so wait
	// until "sub" is known to be watched by writing until an event arrives.
	f := filepath.Join(dir, "f")
	eventually(t, func() bool {
		_ = os.WriteFile(f, []byte("x"), 0o644)
		return r.saw(f)
	})
}

func TestSkipsGitAndNodeModules(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".git", "node_modules"} {
		require.NoError(t, os.Mkdir(filepath.Join(root, d), 0o755))
	}
	r := start(t, root)

	ignored := []string{filepath.Join(root, ".git", "index"), filepath.Join(root, "node_modules", "x")}
	for _, p := range ignored {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	// A watched file written afterwards arrives after any ignored one would have.
	sentinel := filepath.Join(root, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("x"), 0o644))
	eventually(t, func() bool { return r.saw(sentinel) })
	for _, p := range ignored {
		require.False(t, r.saw(p), p)
	}
}

func TestDroppedChmodOnly(t *testing.T) {
	w := &Watcher{}
	_, ok := w.translate(chmodEvent("/x"))
	require.False(t, ok)
}

func TestStopIsIdempotent(t *testing.T) {
	w, err := Start(Config{Root: t.TempDir()})
	require.NoError(t, err)
	w.Stop()
	w.Stop()
}

func TestStartMissingRoot(t *testing.T) {
	_, err := Start(Config{Root: filepath.Join(t.TempDir(), "nope")})
	require.Error(t, err)
}
