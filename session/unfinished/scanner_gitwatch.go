package unfinished

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

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

// watchTierA watches the gitdirs HEAD/index changes land in for linked
// worktrees. It reports false when .git is neither a directory nor a pointer
// file, leaving the caller's plain Add to log the transient condition.
func (s *Scanner) watchTierA(w gitWatcher, repoPath, gitDir string) bool {
	fi, err := os.Lstat(gitDir)
	if err != nil {
		return false
	}
	if !fi.IsDir() {
		// Registered path is itself a linked worktree: watch its resolved
		// admin gitdir, not the pointer file.
		if admin, ok := linkedAdminDir(repoPath); ok {
			s.addWorktreeGitdir(w, admin, worktreeTarget{repoRoot: repoPath, worktree: repoPath})
		} else {
			log.Debug("not watching gitdir outside <commondir>/worktrees", "repo", repoPath)
		}
		return true
	}
	if err := w.Add(gitDir); err != nil {
		log.Debug("could not watch git dir", "dir", gitDir, "err", err)
	} else {
		s.armDirty(repoPath)
	}
	worktreesDir := filepath.Join(gitDir, "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return true
	}
	// Watches are not recursive: once worktrees/ exists, `git worktree add` and
	// `prune` change only its children, which emit nothing on <repo>/.git.
	if err := w.Add(worktreesDir); err != nil {
		log.Debug("could not watch worktrees dir", "dir", worktreesDir, "err", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		admin := filepath.Join(gitDir, "worktrees", e.Name())
		wt, ok := linkedWorktreePath(admin)
		if !ok {
			continue
		}
		s.addWorktreeGitdir(w, admin, worktreeTarget{repoRoot: repoPath, worktree: wt})
	}
	return true
}

// linkedAdminDir resolves a linked worktree's admin gitdir from its .git pointer
// file, accepting it only as a direct child of <commondir>/worktrees (after
// Clean). A pointer to anywhere else (absolute elsewhere, or ".." climbing out)
// is never handed to fsnotify or armed.
func linkedAdminDir(repoPath string) (string, bool) {
	admin := worktreeGitDir(repoPath)
	if filepath.Dir(admin) != filepath.Join(gitCommonDir(repoPath), "worktrees") {
		return "", false
	}
	return admin, true
}

func (s *Scanner) addWorktreeGitdir(w gitWatcher, admin string, t worktreeTarget) {
	if err := w.Add(admin); err != nil {
		log.Debug("could not watch linked worktree gitdir", "dir", admin, "err", err)
		return
	}
	s.fsMu.Lock()
	if s.gitwatch.targets == nil {
		s.gitwatch.targets = make(map[string]worktreeTarget)
	}
	s.gitwatch.targets[admin] = t
	s.fsMu.Unlock()
	s.armDirty(t.worktree)
}

// linkedWorktreePath reads <admin>/gitdir, which holds "<worktree>/.git"
// (relative to admin when git wrote it with worktree.useRelativePaths).
func linkedWorktreePath(admin string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(admin, "gitdir")) // #nosec G304 -- admin is a subdirectory of a tracked repo's .git/worktrees
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	if line == "" {
		return "", false
	}
	return filepath.Dir(resolveGitPath(admin, line)), true
}

func (s *Scanner) worktreeTargetFor(admin string) (worktreeTarget, bool) {
	s.fsMu.Lock()
	defer s.fsMu.Unlock()
	t, ok := s.gitwatch.targets[admin]
	return t, ok
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
