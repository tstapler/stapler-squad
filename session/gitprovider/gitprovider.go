// Package gitprovider hands packages that cannot import session/gitwiring (session/git,
// session/vcs, session/vc, pkg/classifier; see depguard rule git_no_gitwiring_import) the
// process-wide git backend. The composition root registers it once from its own init; callers
// resolve it per call so cohort routing and test doubles take effect without constructor churn.
//
// It deliberately starts no process and constructs no real backend: an unregistered process
// fails git calls with backend.ErrNoLocalRunner rather than silently running unrouted git.
package gitprovider

import (
	"context"
	"sync/atomic"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

type provider func() backend.Backend

var (
	current  atomic.Pointer[provider]
	unwired  = cli.New(nil)
	warnOnce atomic.Bool
)

// Register installs p as the backend source and returns a func restoring the previous one
// (for tests). p may be called from many goroutines and must be cheap after the first call.
// A nil p unregisters.
func Register(p func() backend.Backend) (restore func()) {
	var next *provider
	if p != nil {
		pv := provider(p)
		next = &pv
	}
	prev := current.Swap(next)
	return func() { current.Store(prev) }
}

// Backend returns the registered backend, or one that rejects every Local location when
// nothing is registered (logged once).
func Backend() backend.Backend {
	if p := current.Load(); p != nil {
		return (*p)()
	}
	if warnOnce.CompareAndSwap(false, true) {
		log.Error("git backend provider not registered: import session/gitwiring from the main binary; local git calls will fail")
	}
	return unwired
}

// RefReader satisfies pkg/classifier.RefReader over the registered backend. Paths are plain
// strings, so pkg/ needs no backend types; dir may be any directory inside the repository.
type RefReader struct{}

// ShowToplevel is `git rev-parse --show-toplevel`; an error means dir is not in a repository.
func (RefReader) ShowToplevel(ctx context.Context, dir string) (string, error) {
	root, err := Backend().RepoRoot(ctx, backend.Local{Root: backend.RepoRoot(dir)})
	return string(root), err
}

// GitDir is `git rev-parse --git-dir`, absolute (the per-worktree directory for a linked worktree).
func (RefReader) GitDir(ctx context.Context, dir string) (string, error) {
	gd, err := Backend().GitDir(ctx, backend.Local{Root: backend.RepoRoot(dir)})
	return string(gd), err
}
