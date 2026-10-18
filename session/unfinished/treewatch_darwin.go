//go:build darwin

package unfinished

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

// errTreeWatcherClosed is returned by Add once the watcher has been closed, so
// a late AddRepo can't start a stream nothing will stop.
var errTreeWatcherClosed = errors.New("working-tree watcher closed")

// fseventsTreeWatcher runs one FSEvents stream per tracked repo root.
// Streams start at SinceNow: persisting event IDs across restarts is not done
// yet, so a restart relies on the initial scan AddRepo already performs.
type fseventsTreeWatcher struct {
	onChange treeChangeFunc
	mu       sync.Mutex
	streams  map[string]*fsevents.Stream
	closed   bool
}

func newTreeWatcher(onChange treeChangeFunc) (treeWatcher, error) {
	return &fseventsTreeWatcher{onChange: onChange, streams: make(map[string]*fsevents.Stream)}, nil
}

func (w *fseventsTreeWatcher) Add(repoRoot string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errTreeWatcherClosed
	}
	if _, ok := w.streams[repoRoot]; ok {
		return nil
	}
	// FSEvents reports resolved paths, so compare .git against the resolved root.
	real, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return err
	}
	gitPrefix := filepath.Join(real, ".git") + string(filepath.Separator)
	s, err := fsevents.NewStream(fsevents.Config{
		Paths:     []string{real},
		SinceWhen: fsevents.SinceNow,
		OnEvents: func(evs []fsevents.Event) {
			// One onChange per batch (a build or checkout delivers thousands).
			scope, relevant := gitEventRepoRoot, false
			for _, e := range evs {
				if e.Flags.NeedsRescan() {
					scope, relevant = gitEventAllWorktrees, true
					break
				}
				// .git is already covered (with finer classification) by fsnotifyLoop.
				if e.Flags&fsevents.HistoryDone != 0 || strings.HasPrefix(e.Path+string(filepath.Separator), gitPrefix) {
					continue
				}
				relevant = true
			}
			if relevant {
				w.onChange(repoRoot, scope)
			}
		},
	})
	if err != nil {
		return err
	}
	if err := s.Start(); err != nil {
		return err
	}
	w.streams[repoRoot] = s
	return nil
}

func (w *fseventsTreeWatcher) Remove(repoRoot string) {
	w.mu.Lock()
	s := w.streams[repoRoot]
	delete(w.streams, repoRoot)
	w.mu.Unlock()
	if s != nil {
		s.Stop()
	}
}

func (w *fseventsTreeWatcher) Close() {
	w.mu.Lock()
	w.closed = true
	streams := w.streams
	w.streams = make(map[string]*fsevents.Stream)
	w.mu.Unlock()
	for _, s := range streams {
		s.Stop()
	}
}
