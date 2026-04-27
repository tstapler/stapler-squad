package unfinished

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// WatchDirWatcher discovers git repos under configured watch directories
// and triggers scans via fsnotify on .git/ changes.
type WatchDirWatcher struct {
	scanner    *Scanner
	stateStore *StateStore
	watcher    *fsnotify.Watcher // nil when fallback to polling
}

// NewWatchDirWatcher creates a WatchDirWatcher. It attempts to create an fsnotify watcher
// and falls back to polling if the system doesn't support it.
func NewWatchDirWatcher(scanner *Scanner, stateStore *StateStore) *WatchDirWatcher {
	w := &WatchDirWatcher{
		scanner:    scanner,
		stateStore: stateStore,
	}
	var err error
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.WarningLog.Printf("[unfinished] fsnotify unavailable, falling back to polling: %v", err)
		w.watcher = nil
	}
	return w
}

// Start begins watching all configured watch dirs and pinned repos.
// It performs an initial walk then starts the event loop.
func (w *WatchDirWatcher) Start(ctx context.Context) {
	// Initial walk.
	for _, dir := range w.stateStore.WatchDirs() {
		w.walkDir(dir)
	}
	for _, repo := range w.stateStore.PinnedRepos() {
		w.addRepo(repo)
	}

	if w.watcher != nil {
		go w.fsnotifyLoop(ctx)
	}
	go w.periodicReWalk(ctx)
}

// AddWatchDir adds a new watch directory at runtime and walks it immediately.
func (w *WatchDirWatcher) AddWatchDir(dir string) {
	w.walkDir(dir)
}

// RemoveWatchDir removes a watch directory (repos only removed if not covered by other sources).
func (w *WatchDirWatcher) RemoveWatchDir(dir string) {
	if w.watcher == nil {
		return
	}
	// Remove the fsnotify watch on dir's git subdirs (best-effort).
	_ = w.watcher.Remove(dir)
}

// AddPinnedRepo adds a pinned repo and triggers an immediate scan.
func (w *WatchDirWatcher) AddPinnedRepo(repo string) {
	w.addRepo(repo)
}

// walkDir recursively walks root looking for .git directories at depth <= 5.
// It skips common build/cache directories to avoid false positives and fd exhaustion.
func (w *WatchDirWatcher) walkDir(root string) {
	skipDirs := map[string]bool{
		"node_modules": true,
		"vendor":       true,
		".cache":       true,
		"dist":         true,
		"build":        true,
		".git":         true,
	}

	var walkFn func(dir string, depth int)
	walkFn = func(dir string, depth int) {
		if depth > 5 {
			return
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsPermission(err) {
				log.DebugLog.Printf("[unfinished] permission denied walking %s: %v", dir, err)
			}
			return
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if skipDirs[name] {
				continue
			}
			fullPath := filepath.Join(dir, name)

			if name == ".git" {
				// parent is the repo root
				repoRoot := dir
				w.addRepo(repoRoot)
				return // don't recurse into .git
			}

			// Check if this subdirectory is itself a repo root.
			gitDir := filepath.Join(fullPath, ".git")
			if _, err := os.Stat(gitDir); err == nil {
				w.addRepo(fullPath)
				// Still recurse in case of monorepo with nested repos.
			}

			walkFn(fullPath, depth+1)
		}
	}

	walkFn(root, 0)
}

// addRepo registers a repo root with the scanner and fsnotify watcher.
func (w *WatchDirWatcher) addRepo(repoPath string) {
	w.scanner.AddRepo(repoPath)

	if w.watcher == nil {
		return
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := w.watcher.Add(gitDir); err != nil {
		log.DebugLog.Printf("[unfinished] could not watch %s: %v", gitDir, err)
	}
}

// fsnotifyLoop handles fsnotify events and enqueues repo scans.
func (w *WatchDirWatcher) fsnotifyLoop(ctx context.Context) {
	defer w.watcher.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Derive the repo root from the watched .git directory.
				gitDir := event.Name
				if strings.HasSuffix(gitDir, "/.git") || filepath.Base(gitDir) == ".git" {
					repoRoot := filepath.Dir(gitDir)
					w.scanner.InvalidateCache(repoRoot)
					w.scanner.EnqueueRepo(repoRoot)
				} else {
					// Event for a file inside .git/ — walk up to find .git.
					dir := gitDir
					for {
						if filepath.Base(dir) == ".git" {
							repoRoot := filepath.Dir(dir)
							w.scanner.InvalidateCache(repoRoot)
							w.scanner.EnqueueRepo(repoRoot)
							break
						}
						parent := filepath.Dir(dir)
						if parent == dir {
							break
						}
						dir = parent
					}
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.DebugLog.Printf("[unfinished] fsnotify error: %v", err)
		}
	}
}

// periodicReWalk re-walks watch dirs every 60 seconds to pick up new repos.
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
