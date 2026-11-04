package config

import (
	"os"
	"sync"
	"sync/atomic"
)

// syncHook is the fsync of each config save. On macOS os.File.Sync is
// F_FULLFSYNC, which waits on every other writer's flush too, so tests count
// saves through this seam. Atomic: a background goroutine may be saving while
// a test swaps it.
var syncHook atomic.Pointer[func(*os.File) error] //nolint:gochecknoglobals // test seam

func syncFile(f *os.File) error {
	if h := syncHook.Load(); h != nil {
		return (*h)(f)
	}
	return f.Sync()
}

// SetSyncConfigFileForTest installs fn as the per-save fsync (it must call
// f.Sync itself if durability matters) and returns a restore func. The hook
// sees every save in the process; filter on filepath.Dir(f.Name()).
func SetSyncConfigFileForTest(fn func(*os.File) error) (restore func()) {
	prev := syncHook.Swap(&fn)
	return func() { syncHook.Store(prev) }
}

// configReads counts config.json reads per path, so a test asserts on its own
// state directory and ignores background goroutines elsewhere.
var configReads sync.Map //nolint:gochecknoglobals // path -> *atomic.Int64

func countConfigRead(path string) {
	c, _ := configReads.LoadOrStore(path, new(atomic.Int64))
	c.(*atomic.Int64).Add(1)
}

// ConfigReads returns how many times the config file at path has been read.
func ConfigReads(path string) int64 {
	if c, ok := configReads.Load(path); ok {
		return c.(*atomic.Int64).Load()
	}
	return 0
}
