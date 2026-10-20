package unfinished

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// fsnotify event loop, git event classification, and the per-repo debounce.

// eventWanted filters the fsnotify op. Rename counts only under Tier A: macOS
// git lockfile renames (packed-refs.lock -> packed-refs) arrive as Rename.
func (s *Scanner) eventWanted(ev fsnotify.Event) bool {
	if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) {
		return true
	}
	return s.gitwatch.watchWorktreeGitdirs && ev.Has(fsnotify.Rename)
}

// rescanAllOnOverflow handles a lost-events error: every tracked repo and all
// of its worktrees are invalidated and force-enqueued.
func (s *Scanner) rescanAllOnOverflow(ctx context.Context) {
	s.gitwatch.overflowRescans.Add(1)
	s.repoSet.Range(func(k, _ any) bool {
		if ctx.Err() != nil {
			return false
		}
		if repo, _ := k.(string); repo != "" {
			s.noteDirtyRepo(repo, EventOverflow)
			s.dropRefSnapshot(repo)
			s.invalidateRepo(repo)
			s.enqueueRepo(repo, true)
		}
		return true
	})
}

// fsnotifyLoop handles fsnotify events on every watched repo's .git dir and
// enqueues a targeted rescan of just that repo — this is the "only run when
// things change" trigger; the coordinator's tick is now a pure backstop.
// Exits cleanly when ctx is cancelled, per this package's goroutine convention.
func (s *Scanner) fsnotifyLoop(ctx context.Context) {
	w := s.activeWatcher()
	if w == nil {
		return
	}
	defer w.Close()
	events, errs := w.Events(), w.Errors()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			s.handleEvent(ctx, event)
		case err, ok := <-errs:
			if !ok {
				return
			}
			log.Warn("scanner fsnotify error", "err", err)
			if s.gitwatch.watchWorktreeGitdirs && errors.Is(err, fsnotify.ErrEventOverflow) {
				s.rescanAllOnOverflow(ctx)
			} else {
				s.noteErrorAll()
			}
		}
	}
}

// noteErrorAll records a non-overflow watcher error against every tracked
// repo and worktree: events may have been lost, so no clean verdict is trusted
// until the next full walk.
func (s *Scanner) noteErrorAll() {
	s.repoSet.Range(func(k, _ any) bool {
		if repo, _ := k.(string); repo != "" {
			s.noteDirtyRepo(repo, EventError)
		}
		return true
	})
}

// dropRemovedAdmin forgets a pruned linked worktree's admin dir and schedules a
// rescan of its repo, reporting whether event.Name was a watched admin dir.
func (s *Scanner) dropRemovedAdmin(ctx context.Context, event fsnotify.Event) bool {
	if !event.Has(fsnotify.Remove) || !s.gitwatch.watchWorktreeGitdirs {
		return false
	}
	s.fsMu.Lock()
	t, ok := s.gitwatch.targets[event.Name]
	delete(s.gitwatch.targets, event.Name)
	s.fsMu.Unlock()
	if !ok {
		return false
	}
	if w := s.activeWatcher(); w != nil {
		_ = w.Remove(event.Name)
	}
	s.disarmDirty(t.worktree)
	s.noteDirty(t.worktree, EventDirChange)
	s.dropRefSnapshot(t.repoRoot)
	s.scheduleWorktreeEvent(ctx, t)
	return true
}

func (s *Scanner) handleEvent(ctx context.Context, event fsnotify.Event) {
	if s.dropRemovedAdmin(ctx, event) {
		return
	}
	if !s.eventWanted(event) {
		return
	}
	scope, relevant := classifyGitEvent(filepath.Base(event.Name))
	if !relevant {
		return
	}
	if t, ok := s.worktreeTargetFor(filepath.Dir(event.Name)); ok {
		s.noteDirty(t.worktree, EventTrackedPath)
		s.dropRefSnapshot(t.repoRoot)
		s.scheduleWorktreeEvent(ctx, t)
		return
	}
	// Derive the repo root by walking up from the event path to the
	// nearest ".git" component (mirrors WatchDirWatcher.fsnotifyLoop's
	// event-handling shape in watcher.go).
	dir := event.Name
	for {
		if filepath.Base(dir) == ".git" {
			repoRoot := filepath.Dir(dir)
			s.noteGitEvent(repoRoot, event, scope)
			s.dropRefSnapshot(repoRoot)
			s.scheduleGitEvent(ctx, repoRoot, scope)
			if s.gitwatch.watchWorktreeGitdirs && scope == gitEventAllWorktrees {
				s.watchRepo(repoRoot) // picks up linked worktrees added since the last pass
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// noteGitEvent feeds a <repo>/.git event into DirtyGen: a new or removed linked
// worktree admin dir is a directory change; anything else is a hint on the
// worktrees whose caches the event invalidates.
func (s *Scanner) noteGitEvent(repoRoot string, event fsnotify.Event, scope gitEventScope) {
	if filepath.Base(event.Name) == "worktrees" {
		s.noteDirtyRepo(repoRoot, EventDirChange)
		return
	}
	if scope == gitEventAllWorktrees {
		s.noteDirtyRepo(repoRoot, EventTrackedPath)
		return
	}
	s.noteDirty(repoRoot, EventTrackedPath)
}

// fsnotifyDebounce is the window after a repo's first .git event in which
// further events fold into one rescan; one git command emits several.
const fsnotifyDebounce = 2 * time.Second

// gitEventScope is what a .git write invalidates: just the repo's own
// worktree, or every linked worktree too.
type gitEventScope int

const (
	gitEventRepoRoot gitEventScope = iota
	gitEventAllWorktrees
)

// classifyGitEvent decides whether a write to the named file directly under
// <repo>/.git warrants a rescan. Lock files, git's index temp files and
// watchman cookies are transient; the real file's own Create/Write follows
// the lock's rename, so reacting to them only doubles the work. index, config
// and MERGE_RR churn constantly but only describe the main worktree; every
// other file (HEAD, FETCH_HEAD, COMMIT_EDITMSG, packed-refs, ...) is a
// commit/fetch/ref signal that can move linked worktrees' ahead/behind too,
// and loose refs under .git/refs are not watched, so these are the proxy.
func classifyGitEvent(name string) (scope gitEventScope, relevant bool) {
	switch {
	case strings.HasSuffix(name, ".lock"),
		strings.HasPrefix(name, "next-index-"),
		strings.HasPrefix(name, ".watchman-cookie"):
		return gitEventRepoRoot, false
	case name == "index", name == "config", name == "MERGE_RR":
		return gitEventRepoRoot, true
	default:
		return gitEventAllWorktrees, true
	}
}

// scheduleGitEvent invalidates and enqueues repoRoot once, fsDebounce after
// the first event of a burst (fixed window, so continuous events cannot starve
// a repo), widening the scope if any event in the
// burst needed it. Skipped after ctx is cancelled.
func (s *Scanner) scheduleGitEvent(ctx context.Context, repoRoot string, scope gitEventScope) {
	s.debounce(ctx, repoRoot, scope, func(scope gitEventScope) {
		if scope == gitEventAllWorktrees {
			s.invalidateRepo(repoRoot)
		} else {
			s.InvalidateCache(repoRoot)
		}
		s.EnqueueRepo(repoRoot)
	})
}

// scheduleWorktreeEvent is the Tier A burst scoped to one linked worktree:
// only its cache is invalidated, then its repo is enqueued.
func (s *Scanner) scheduleWorktreeEvent(ctx context.Context, t worktreeTarget) {
	s.debounce(ctx, "wt:"+t.worktree, gitEventRepoRoot, func(gitEventScope) {
		s.InvalidateCache(t.worktree)
		s.EnqueueRepo(t.repoRoot)
	})
}

// debounce runs flush once, fsDebounce after the first call for key.
func (s *Scanner) debounce(ctx context.Context, key string, scope gitEventScope, flushScope func(gitEventScope)) {
	flush := func(scope gitEventScope) {
		if ctx.Err() != nil {
			return
		}
		flushScope(scope)
	}
	if s.fsDebounce <= 0 {
		flush(scope)
		return
	}
	s.fsMu.Lock()
	if cur, armed := s.fsPending[key]; armed {
		s.fsPending[key] = max(cur, scope)
		s.fsMu.Unlock()
		return
	}
	if s.fsPending == nil {
		s.fsPending = make(map[string]gitEventScope)
	}
	s.fsPending[key] = scope
	s.fsMu.Unlock()
	time.AfterFunc(s.fsDebounce, func() {
		s.fsMu.Lock()
		widest := s.fsPending[key]
		delete(s.fsPending, key)
		s.fsMu.Unlock()
		flush(widest)
	})
}
