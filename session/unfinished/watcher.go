package unfinished

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// WatchDirWatcher discovers git repos under configured watch directories and
// registers each one with Scanner via AddRepo. It owns no fsnotify watcher of
// its own — Scanner.AddRepo already registers a .git-dir fsnotify watch and
// triggers an immediate scan for every repo it's given (see Scanner.watchRepo),
// so a second watcher here would only double fd usage and event handling for
// the exact same repos. WatchDirWatcher's only job is discovery: finding which
// repos exist under a watch dir in the first place, which fsnotify on an
// already-known repo's .git dir cannot do for a repo that doesn't exist yet.
type WatchDirWatcher struct {
	scanner    *Scanner
	stateStore *StateStore

	// discovered tracks which repo paths were found under each watch dir, so
	// RemoveWatchDir can unregister exactly those repos from the scanner
	// instead of leaving them tracked forever after their watch dir is removed.
	// map[watchDir]map[repoPath]struct{}, guarded by mu.
	mu         sync.Mutex
	discovered map[string]map[string]struct{}
}

// NewWatchDirWatcher creates a WatchDirWatcher.
func NewWatchDirWatcher(scanner *Scanner, stateStore *StateStore) *WatchDirWatcher {
	return &WatchDirWatcher{
		scanner:    scanner,
		stateStore: stateStore,
		discovered: make(map[string]map[string]struct{}),
	}
}

// Start performs an initial walk of every configured watch dir and pinned
// repo, then begins the periodic re-walk that picks up newly created repos.
// Changes to already-discovered repos are Scanner's responsibility from here
// on (its own fsnotify watch, registered by AddRepo below).
func (w *WatchDirWatcher) Start(ctx context.Context) {
	for _, dir := range w.stateStore.WatchDirs() {
		w.walkDir(dir)
	}
	for _, repo := range w.stateStore.PinnedRepos() {
		w.scanner.AddRepo(repo)
	}

	go w.periodicReWalk(ctx)
}

// AddWatchDir adds a new watch directory at runtime and walks it immediately.
func (w *WatchDirWatcher) AddWatchDir(dir string) {
	w.walkDir(dir)
}

// RemoveWatchDir removes a watch directory and unregisters every repo that
// was discovered under it (via Scanner.RemoveRepo), unless the caller still
// wants a given repo tracked through another source (pinned repos, active
// sessions) — RemoveRepo only affects this watcher's own claim; Scanner
// itself has no per-source refcounting, so a repo also tracked via
// auto-spider or pinning gets re-added the next time that source fires.
func (w *WatchDirWatcher) RemoveWatchDir(dir string) {
	w.mu.Lock()
	repos := w.discovered[dir]
	delete(w.discovered, dir)
	w.mu.Unlock()

	for repoPath := range repos {
		w.scanner.RemoveRepo(repoPath)
	}
}

// watchDirSkipDirs names directories walkDir never descends into, to avoid
// false-positive repo detection and fd exhaustion under large dependency trees.
var watchDirSkipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".cache":       true,
	"dist":         true,
	"build":        true,
	".git":         true,
}

// walkDir recursively walks root looking for .git directories at depth <= 5.
func (w *WatchDirWatcher) walkDir(root string) {
	w.walkDirAt(root, root, 0)
}

// walkDirAt is walkDir's recursive step: dir is the directory currently being
// scanned, root is the original watch dir (passed through unchanged, for
// addRepo's discovered-repos index), depth counts levels below root.
func (w *WatchDirWatcher) walkDirAt(root, dir string, depth int) {
	if depth > 5 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsPermission(err) {
			log.Debug("permission denied walking directory", "dir", dir, "err", err)
		}
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || watchDirSkipDirs[entry.Name()] {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())

		if entry.Name() == ".git" {
			w.addRepo(root, dir) // parent is the repo root
			return               // don't recurse into .git
		}

		// Check if this subdirectory is itself a repo root; still recurse
		// into it afterward in case of a monorepo with nested repos.
		if _, err := os.Stat(filepath.Join(fullPath, ".git")); err == nil {
			w.addRepo(root, fullPath)
		}
		w.walkDirAt(root, fullPath, depth+1)
	}
}

// addRepo registers repoPath with the scanner and records it against
// watchDir in the discovered index, so RemoveWatchDir can later unregister
// exactly the repos this watch dir contributed.
func (w *WatchDirWatcher) addRepo(watchDir, repoPath string) {
	w.scanner.AddRepo(repoPath)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.discovered[watchDir] == nil {
		w.discovered[watchDir] = make(map[string]struct{})
	}
	w.discovered[watchDir][repoPath] = struct{}{}
}

// periodicReWalk re-walks watch dirs every 60 seconds to pick up new repos.
// Scanner's own fsnotify watch on each already-discovered repo's .git dir
// (registered by AddRepo) handles change detection for known repos; this
// ticker exists only to notice a brand-new repo appearing under a watch dir,
// which nothing is watching for yet.
func (w *WatchDirWatcher) periodicReWalk(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, dir := range w.stateStore.WatchDirs() {
				w.walkDir(dir)
			}
		}
	}
}
