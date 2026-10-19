package unfinished

import (
	"context"
	"errors"
	"time"

	"path/filepath"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/internal/fsevents"
	"github.com/tstapler/stapler-squad/log"
)

// treeWatcher reports working-tree changes under a repo root. It exists
// because the fsnotify watch covers only <repo>/.git, so an edit that never
// touches .git is otherwise noticed only by the backstop tick. Implemented via
// FSEvents on darwin (treewatch_darwin.go) and recursive inotify on Linux
// (treewatch_linux.go); nil elsewhere.
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
		s.onTreeChange(ctx, repoRoot, scope)
	})
	if errors.Is(err, errNoTreeWatcher) {
		return
	}
	if err != nil {
		log.Warn("working-tree watcher unavailable, using backstop tick only", "err", err)
		return
	}
	s.installTreeWatcher(ctx, tw)
}

// onTreeChange handles one working-tree event. The DirtyGen mark is immediate,
// not debounced: a scan racing the debounce window must already see the change,
// or a clean cached verdict (scan_skip) could survive it. The tree event does not
// say what changed, so it is a directory-change (forces the untracked-dir and
// index caches to refresh); a lost-events scope is an overflow on every
// worktree. Unlike noteDirty it ignores watch_worktree_gitdirs: DirtyGen is
// armed only under Tier A, and an unarmed one does full walks regardless.
func (s *Scanner) onTreeChange(ctx context.Context, repoRoot string, scope gitEventScope) {
	if d := s.dirtyTracker(); d != nil {
		kind := EventDirChange
		if scope == gitEventAllWorktrees {
			kind = EventOverflow
		}
		d.NoteDirty(repoRoot, kind)
		if scope == gitEventAllWorktrees {
			s.repoWorktreePaths(repoRoot, func(wt string) { d.NoteDirty(wt, kind) })
		}
	}
	s.scheduleGitEvent(ctx, repoRoot, scope)
}

// repoWorktreePaths calls fn for each linked worktree known for repoPath.
func (s *Scanner) repoWorktreePaths(repoPath string, fn func(worktree string)) {
	v, ok := s.repoWorktrees.Load(repoPath)
	if !ok {
		return
	}
	if set, ok := v.(*sync.Map); ok {
		set.Range(func(k, _ any) bool {
			if wt, _ := k.(string); wt != "" && wt != repoPath {
				fn(wt)
			}
			return true
		})
	}
}

// installTreeWatcher publishes tw, replays already-tracked repos into it and
// ties its lifetime to ctx.
func (s *Scanner) installTreeWatcher(ctx context.Context, tw treeWatcher) {
	s.mu.Lock()
	s.treeWatch = tw
	s.mu.Unlock()
	// Repos added before Start would otherwise sit on the stretched backstop
	// with no event source.
	s.repoSet.Range(func(k, _ any) bool {
		if repo, ok := k.(string); ok {
			if err := tw.Add(repo); err != nil {
				log.Debug("could not watch working tree", "repo", repo, "err", err)
			}
		}
		return true
	})
	s.wg.Go(func() {
		<-ctx.Done()
		tw.Close()
	})
}

// batchScope folds one event batch into a single scope for repoRoot (a build
// or checkout delivers thousands of events). .git is skipped: fsnotifyLoop
// already covers it with finer classification. relevant is false when nothing
// in the batch warrants invalidation.
func batchScope(gitPrefix string, evs []fsevents.Event) (scope gitEventScope, relevant bool) {
	scope = gitEventRepoRoot
	for _, e := range evs {
		if e.Flags.NeedsRescan() {
			return gitEventAllWorktrees, true
		}
		if e.Flags&fsevents.HistoryDone != 0 || strings.HasPrefix(e.Path+string(filepath.Separator), gitPrefix) {
			continue
		}
		relevant = true
	}
	return scope, relevant
}
