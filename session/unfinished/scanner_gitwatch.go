package unfinished

import (
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// Git watcher plumbing: watcher interface, dirty-tracker hooks, and watchRepo/unwatchRepo lifecycle.

// gitWatcher is the seam over *fsnotify.Watcher so tests can inject a fake
// with no real filesystem events. Remove is an addition to the plan's
// four-method sketch: unwatchRepo needs it.
type gitWatcher interface {
	Add(name string) error
	Remove(name string) error
	Events() <-chan fsnotify.Event
	Errors() <-chan error
	Close() error
}

type fsnotifyWatcher struct{ w *fsnotify.Watcher }

func (f fsnotifyWatcher) Add(name string) error         { return f.w.Add(name) }
func (f fsnotifyWatcher) Remove(name string) error      { return f.w.Remove(name) }
func (f fsnotifyWatcher) Events() <-chan fsnotify.Event { return f.w.Events }
func (f fsnotifyWatcher) Errors() <-chan error          { return f.w.Errors }
func (f fsnotifyWatcher) Close() error                  { return f.w.Close() }

// worktreeTarget says what a change in one linked worktree's admin gitdir
// (<common>/.git/worktrees/<name>) invalidates: that worktree's cache, then a
// rescan of repoRoot, whose ListWorktrees scan covers it.
type worktreeTarget struct{ repoRoot, worktree string }

// gitwatchState is the Tier A (linked-worktree gitdir) state. The zero value
// is the default behaviour: only <repo>/.git is watched, exactly as before.
type gitwatchState struct {
	// override replaces the watcher derived from Scanner.fsWatcher (tests).
	override gitWatcher
	// watchWorktreeGitdirs gates every Tier A behaviour: linked gitdir
	// watches, Rename events, and overflow rescans. The config flag wires in
	// here later; it must be set before Start.
	watchWorktreeGitdirs bool
	// targets maps an admin gitdir to what its events invalidate (guarded by
	// Scanner.fsMu).
	targets map[string]worktreeTarget
	// overflowRescans counts fsnotify.ErrEventOverflow full-rescan sweeps.
	overflowRescans atomic.Int64
}

// activeWatcher returns the injected watcher, else one over fsWatcher, else
// nil (fsnotify unavailable: tick-only polling).
func (s *Scanner) activeWatcher() gitWatcher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.gitwatch.override != nil {
		return s.gitwatch.override
	}
	if s.fsWatcher == nil {
		return nil
	}
	return fsnotifyWatcher{s.fsWatcher}
}

// dirtyTracker returns the reader's DirtyGen sink, or nil for readers (fakes,
// tests) that keep none.
func (s *Scanner) dirtyTracker() dirtyTracker {
	d, _ := s.reader.(dirtyTracker)
	return d
}

// noteDirty feeds one watcher observation for worktree into DirtyGen.
func (s *Scanner) noteDirty(worktree string, kind EventKind) {
	if d := s.dirtyTracker(); d != nil && s.gitwatch.watchWorktreeGitdirs {
		d.NoteDirty(worktree, kind)
	}
}

// noteDirtyRepo notes kind for repoPath and every linked worktree known for it.
func (s *Scanner) noteDirtyRepo(repoPath string, kind EventKind) {
	s.noteDirty(repoPath, kind)
	if v, ok := s.repoWorktrees.Load(repoPath); ok {
		if set, ok := v.(*sync.Map); ok {
			set.Range(func(k, _ any) bool {
				if wt, _ := k.(string); wt != "" && wt != repoPath {
					s.noteDirty(wt, kind)
				}
				return true
			})
		}
	}
}

// armDirty marks worktree's watcher live once its watch registered (Tier A only).
func (s *Scanner) armDirty(worktree string) {
	if _, unwatched := s.treeUnwatched.Load(worktree); unwatched {
		return
	}
	if d := s.dirtyTracker(); d != nil && s.gitwatch.watchWorktreeGitdirs {
		d.ArmDirty(worktree)
	}
}

func (s *Scanner) disarmDirty(worktree string) {
	if d := s.dirtyTracker(); d != nil && s.gitwatch.watchWorktreeGitdirs {
		d.DisarmDirty(worktree)
	}
}

// watchRepo best-effort registers repoPath's .git dir with the fsnotify
// watcher so a real change triggers an immediate targeted rescan. No-op if
// fsnotify is unavailable on this platform (no active watcher) — that's the
// graceful-degradation path back to relying solely on the ticker. Errors are
// logged at Debug, not Warn: many repo paths won't have a .git dir yet at
// the moment they're first tracked (e.g. auto-spidered from a session whose
// worktree is still being created), which is a normal transient condition,
// not a fault.
func (s *Scanner) watchRepo(repoPath string) {
	s.mu.RLock()
	tw := s.treeWatch
	s.mu.RUnlock()
	if tw != nil {
		s.addTreeWatch(tw, repoPath)
	}
	w := s.activeWatcher()
	if w == nil {
		return
	}
	gitDir := filepath.Join(repoPath, ".git")
	if s.gitwatch.watchWorktreeGitdirs {
		if s.watchTierA(w, repoPath, gitDir) {
			return
		}
	}
	if err := w.Add(gitDir); err != nil {
		log.Debug("could not watch git dir", "dir", gitDir, "err", err)
	}
}

// unwatchRepo best-effort removes repoPath's .git dir (and, under Tier A, its
// linked gitdirs) from the fsnotify watcher, avoiding a slow leak of watch
// descriptors as repos come and go. No-op if fsnotify is unavailable.
func (s *Scanner) unwatchRepo(repoPath string) {
	s.mu.RLock()
	tw := s.treeWatch
	s.mu.RUnlock()
	if tw != nil {
		tw.Remove(repoPath)
	}
	s.treeUnwatched.Delete(repoPath)
	w := s.activeWatcher()
	if w == nil {
		return
	}
	_ = w.Remove(filepath.Join(repoPath, ".git"))
	if s.gitwatch.watchWorktreeGitdirs {
		_ = w.Remove(filepath.Join(repoPath, ".git", "worktrees"))
	}
	s.disarmDirty(repoPath)
	type removal struct{ admin, worktree string }
	var removals []removal
	s.fsMu.Lock()
	for admin, t := range s.gitwatch.targets {
		if t.repoRoot == repoPath {
			removals = append(removals, removal{admin, t.worktree})
			delete(s.gitwatch.targets, admin)
		}
	}
	s.fsMu.Unlock()
	for _, r := range removals {
		_ = w.Remove(r.admin)
		s.disarmDirty(r.worktree)
	}
}
