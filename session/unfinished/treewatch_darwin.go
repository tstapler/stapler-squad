//go:build darwin

package unfinished

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/internal/fsevents"
)

// fseventsTreeWatcher runs one FSEvents stream per tracked repo root.
// Streams start at SinceNow: persisting event IDs across restarts is not done
// yet, so a restart relies on the initial scan AddRepo already performs.
type fseventsTreeWatcher struct {
	onChange treeChangeFunc
	mu       sync.Mutex
	streams  map[string]*fsevents.Stream
}

func newTreeWatcher(onChange treeChangeFunc) (treeWatcher, error) {
	return &fseventsTreeWatcher{onChange: onChange, streams: make(map[string]*fsevents.Stream)}, nil
}

func (w *fseventsTreeWatcher) Add(repoRoot string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
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
			for _, e := range evs {
				// .git is already covered (with finer classification) by fsnotifyLoop.
				if strings.HasPrefix(e.Path+string(filepath.Separator), gitPrefix) && !e.Flags.NeedsRescan() {
					continue
				}
				if e.Flags&fsevents.HistoryDone != 0 {
					continue
				}
				scope := gitEventRepoRoot
				if e.Flags.NeedsRescan() {
					scope = gitEventAllWorktrees
				}
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
	streams := w.streams
	w.streams = make(map[string]*fsevents.Stream)
	w.mu.Unlock()
	for _, s := range streams {
		s.Stop()
	}
}
