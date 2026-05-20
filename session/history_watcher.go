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
	}
}

// Start begins watching the directory. It returns without error even if the
// directory does not exist (degraded mode — polling fallback still works).
func (w *HistoryFileWatcher) Start(ctx context.Context) error {
	if _, err := os.Stat(w.watchDir); os.IsNotExist(err) {
		log.Warn("history file watcher: watch directory does not exist", "path", w.watchDir)
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	if err := watcher.Add(w.watchDir); err != nil {
		watcher.Close()
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

func (w *HistoryFileWatcher) run(ctx context.Context) {
	defer func() {
		if w.watcher != nil {
			w.watcher.Close()
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

// Stop closes the watcher.
func (w *HistoryFileWatcher) Stop() {
	if w.watcher != nil {
		w.watcher.Close()
	}
}
