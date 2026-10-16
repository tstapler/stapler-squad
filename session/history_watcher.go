package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// HistoryFileWatcher watches ~/.claude/projects/ for new JSONL files.
type HistoryFileWatcher struct {
	watchDir string
	callback func(filePath string)
	watcher  *fsnotify.Watcher
	stopped  chan struct{}
}

// NewHistoryFileWatcher creates a watcher for the given directory.
// If watchDir is empty, defaults to ~/.claude/projects/.
func NewHistoryFileWatcher(watchDir string, callback func(filePath string)) *HistoryFileWatcher {
	if watchDir == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			watchDir = filepath.Join(homeDir, ".claude", "projects")
		}
	}
	return &HistoryFileWatcher{
		watchDir: watchDir,
		callback: callback,
		stopped:  make(chan struct{}),
	}
}

// Start begins watching the directory. It returns without error even if the
// directory does not exist (degraded mode — polling fallback still works).
func (w *HistoryFileWatcher) Start(ctx context.Context) error {
	if _, err := os.Stat(w.watchDir); os.IsNotExist(err) {
		log.Warn("history file watcher: watch directory does not exist", "path", w.watchDir)
		close(w.stopped)
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		close(w.stopped)
		return err
	}
	w.watcher = watcher

	if err := watcher.Add(w.watchDir); err != nil {
		_ = watcher.Close()
		close(w.stopped)
		return err
	}

	// Also watch any existing subdirectories so we catch files created
	// inside project-specific subdirectories.
	_ = filepath.WalkDir(w.watchDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() || path == w.watchDir {
			return nil
		}
		_ = watcher.Add(path)
		return nil
	})

	go w.run(ctx)
	return nil
}

// Stopped returns a channel that is closed when the watcher goroutine has exited.
func (w *HistoryFileWatcher) Stopped() <-chan struct{} {
	return w.stopped
}

func (w *HistoryFileWatcher) run(ctx context.Context) {
	defer close(w.stopped)
	defer func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Warn("history file watcher error", "err", err)
		}
	}
}

func (w *HistoryFileWatcher) handleEvent(event fsnotify.Event) {
	// A newly created subdirectory (e.g. a fresh git worktree's own
	// ~/.claude/projects/<encoded-path>/ directory) is not recursed into by
	// fsnotify automatically — watch it explicitly or every .jsonl written
	// inside it goes unobserved for the life of the process.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			w.watchNewDir(event.Name)
			return
		}
	}

	// Care about CREATE, RENAME, and WRITE events (WRITE fires as JSONL is appended).
	if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) == 0 {
		return
	}

	path := event.Name

	// Must end in .jsonl
	if !strings.HasSuffix(path, ".jsonl") {
		return
	}

	// Skip agent files
	base := filepath.Base(path)
	if strings.HasPrefix(base, "agent-") {
		return
	}

	if w.callback != nil {
		w.callback(path)
	}
}

// watchNewDir adds a newly created subdirectory to the fsnotify watch list
// and enqueues any .jsonl files already present in it. The directory can
// arrive non-empty if Claude Code writes its transcript before this handler
// gets to process the CREATE event for the directory itself.
func (w *HistoryFileWatcher) watchNewDir(dir string) {
	if err := w.watcher.Add(dir); err != nil {
		log.Warn("history file watcher: failed to watch new directory", "path", dir, "err", err)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, "agent-") {
			continue
		}
		if w.callback != nil {
			w.callback(filepath.Join(dir, name))
		}
	}
}

// Stop closes the watcher.
func (w *HistoryFileWatcher) Stop() {
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}
