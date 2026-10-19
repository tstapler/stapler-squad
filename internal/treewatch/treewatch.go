// Package treewatch is the Linux counterpart of internal/fsevents: a recursive
// inotify watch over a working tree that delivers event batches in the same
// fsevents.Event shape, so callers share their filtering code across platforms.
//
// inotify needs one watch per directory, so a tree is only watched when every
// directory fits under fs.inotify.max_user_watches. Start fails otherwise
// rather than watching part of the tree: a partial watch would silently miss
// edits while the caller trusts it as the primary change source.
package treewatch

import (
	"errors"
	"time"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

// ErrUnsupported means this platform has no tree watcher implementation.
var ErrUnsupported = errors.New("treewatch: unsupported platform")

// ErrWatchLimit means the tree has more directories than inotify can watch.
var ErrWatchLimit = errors.New("treewatch: inotify watch limit reached (raise fs.inotify.max_user_watches)")

// skipDir reports directories that are never watched. .git is covered
// separately by the callers' own .git watch; node_modules is large and almost
// always gitignored.
func skipDir(name string) bool { return name == ".git" || name == "node_modules" }

// batchWindow coalesces events that arrive back to back into one OnEvents call.
const batchWindow = 20 * time.Millisecond

// Config configures a Watcher.
type Config struct {
	Root string
	// OnEvents receives each batch. Events lost to a kernel queue overflow are
	// reported as one event carrying fsevents.KernelDropped. Must not block.
	OnEvents func([]fsevents.Event)
}
