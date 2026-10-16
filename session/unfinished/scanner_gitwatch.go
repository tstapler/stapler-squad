package unfinished

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// watchRepo best-effort registers repoPath's .git dir with the fsnotify
// watcher so a real change triggers an immediate targeted rescan. No-op if
// fsnotify is unavailable on this platform (s.fsWatcher == nil) — that's the
// graceful-degradation path back to relying solely on the ticker. Errors are
// logged at Debug, not Warn: many repo paths won't have a .git dir yet at
// the moment they're first tracked (e.g. auto-spidered from a session whose
// worktree is still being created), which is a normal transient condition,
// not a fault.
func (s *Scanner) watchRepo(repoPath string) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	gitDir := filepath.Join(repoPath, ".git")
	if err := w.Add(gitDir); err != nil {
		log.Debug("could not watch git dir", "dir", gitDir, "err", err)
	}
}

// unwatchRepo best-effort removes repoPath's .git dir from the fsnotify
// watcher, avoiding a slow leak of watch descriptors as repos come and go.
// No-op if fsnotify is unavailable.
func (s *Scanner) unwatchRepo(repoPath string) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	_ = w.Remove(filepath.Join(repoPath, ".git"))
}

// fsnotifyLoop handles fsnotify events on every watched repo's .git dir and
// enqueues a targeted rescan of just that repo — this is the "only run when
// things change" trigger; the coordinator's tick is now a pure backstop.
// Exits cleanly when ctx is cancelled, per this package's goroutine convention.
func (s *Scanner) fsnotifyLoop(ctx context.Context) {
	s.mu.RLock()
	w := s.fsWatcher
	s.mu.RUnlock()
	if w == nil {
		return
	}
	defer w.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}
			scope, relevant := classifyGitEvent(filepath.Base(event.Name))
			if !relevant {
				continue
			}
			// Derive the repo root by walking up from the event path to the
			// nearest ".git" component (mirrors WatchDirWatcher.fsnotifyLoop's
			// event-handling shape in watcher.go).
			dir := event.Name
			for {
				if filepath.Base(dir) == ".git" {
					s.scheduleGitEvent(ctx, filepath.Dir(dir), scope)
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Warn("scanner fsnotify error", "err", err)
		}
	}
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
	flush := func(scope gitEventScope) {
		if ctx.Err() != nil {
			return
		}
		if scope == gitEventAllWorktrees {
			s.invalidateRepo(repoRoot)
		} else {
			s.InvalidateCache(repoRoot)
		}
		s.EnqueueRepo(repoRoot)
	}
	if s.fsDebounce <= 0 {
		flush(scope)
		return
	}
	s.fsMu.Lock()
	if cur, armed := s.fsPending[repoRoot]; armed {
		s.fsPending[repoRoot] = max(cur, scope)
		s.fsMu.Unlock()
		return
	}
	if s.fsPending == nil {
		s.fsPending = make(map[string]gitEventScope)
	}
	s.fsPending[repoRoot] = scope
	s.fsMu.Unlock()
	time.AfterFunc(s.fsDebounce, func() {
		s.fsMu.Lock()
		widest := s.fsPending[repoRoot]
		delete(s.fsPending, repoRoot)
		s.fsMu.Unlock()
		flush(widest)
	})
}
