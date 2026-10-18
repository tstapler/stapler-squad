package unfinished

import (
	"context"
	"errors"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// treeWatcher reports working-tree changes under a repo root. It exists
// because the fsnotify watch covers only <repo>/.git, so an edit that never
// touches .git is otherwise noticed only by the backstop tick. Implemented on
// darwin via FSEvents (treewatch_darwin.go); nil elsewhere.
type treeWatcher interface {
	Add(repoRoot string) error
	Remove(repoRoot string)
	Close()
}

// treeChangeFunc is invoked for each working-tree change under repoRoot.
// scope is gitEventAllWorktrees when the event source lost track of changes
// (a rescan flag) and everything must be re-derived.
type treeChangeFunc func(repoRoot string, scope gitEventScope)

// errNoTreeWatcher means this platform has no working-tree watcher; not a fault.
var errNoTreeWatcher = errors.New("no working-tree watcher on this platform")

// backstopStretch is how much longer the backstop tick may be when a
// treeWatcher is active: it is then a safety net, not the change detector.
const backstopStretch = 2

// backstopInterval is the coordinator tick period, stretched when working-tree
// events are delivered by a treeWatcher.
func (s *Scanner) backstopInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.treeWatch != nil {
		return s.tickInterval * backstopStretch
	}
	return s.tickInterval
}

// startTreeWatch installs the platform treeWatcher, if any, and closes it when
// ctx is cancelled. Failure degrades to tick-only working-tree detection.
func (s *Scanner) startTreeWatch(ctx context.Context) {
	tw, err := newTreeWatcher(func(repoRoot string, scope gitEventScope) {
		s.scheduleGitEvent(ctx, repoRoot, scope)
	})
	if errors.Is(err, errNoTreeWatcher) {
		return
	}
	if err != nil {
		log.Warn("working-tree watcher unavailable, using backstop tick only", "err", err)
		return
	}
	s.mu.Lock()
	s.treeWatch = tw
	s.mu.Unlock()
	s.wg.Go(func() {
		<-ctx.Done()
		tw.Close()
	})
}
