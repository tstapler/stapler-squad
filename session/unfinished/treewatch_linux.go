//go:build linux

package unfinished

import (
	"errors"
	"path/filepath"
	"sync"

	"github.com/tstapler/stapler-squad/internal/fsevents"
	"github.com/tstapler/stapler-squad/internal/treewatch"
)

// errTreeWatcherClosed is returned by Add once the watcher has been closed, so
// a late AddRepo can't start a watch nothing will stop.
var errTreeWatcherClosed = errors.New("working-tree watcher closed")

// inotifyTreeWatcher runs one recursive inotify watch per tracked repo root.
// A repo whose tree exceeds the inotify limit is rejected by Add (the caller
// logs it) and stays on the backstop tick.
type inotifyTreeWatcher struct {
	onChange treeChangeFunc
	mu       sync.Mutex
	watches  map[string]*treewatch.Watcher
	closed   bool
}

func newTreeWatcher(onChange treeChangeFunc) (treeWatcher, error) {
	return &inotifyTreeWatcher{onChange: onChange, watches: make(map[string]*treewatch.Watcher)}, nil
}

func (w *inotifyTreeWatcher) Add(repoRoot string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errTreeWatcherClosed
	}
	if _, ok := w.watches[repoRoot]; ok {
		return nil
	}
	gitPrefix := filepath.Join(repoRoot, ".git") + string(filepath.Separator)
	tw, err := treewatch.Start(treewatch.Config{
		Root: repoRoot,
		OnEvents: func(evs []fsevents.Event) {
			if scope, relevant := batchScope(gitPrefix, evs); relevant {
				w.onChange(repoRoot, scope)
			}
		},
	})
	if err != nil {
		return err
	}
	w.watches[repoRoot] = tw
	return nil
}

func (w *inotifyTreeWatcher) Remove(repoRoot string) {
	w.mu.Lock()
	tw := w.watches[repoRoot]
	delete(w.watches, repoRoot)
	w.mu.Unlock()
	if tw != nil {
		tw.Stop()
	}
}

func (w *inotifyTreeWatcher) Close() {
	w.mu.Lock()
	w.closed = true
	watches := w.watches
	w.watches = make(map[string]*treewatch.Watcher)
	w.mu.Unlock()
	for _, tw := range watches {
		tw.Stop()
	}
}
