// Package gogit implements backend.Backend in-process with go-git, for the read operations of
// the refs cohort whose go-git form is proven at least as fast as the CLI and identical in
// edge semantics (go-git fork plan Story 2.1.1, gate G3). Every operation it does not serve,
// and every input whose CLI behaviour it cannot reproduce exactly, returns
// backend.ErrFallbackEligible{Wrote: false}, so the Router answers from the CLI instead of
// from a wrong in-process result. A divergence is therefore always visible as a fallback,
// never as a different answer.
//
// Served: CurrentBranch, HeadRef, ResolveRef, RefExists, RepoRoot, GitDir, CommonDir,
// ListRefs, MergeBase, GetConfig. Not served (G3: slower than the CLI or not semantically
// equal): CountCommits, Log, ListBranches and everything outside the refs cohort.
//
// Rules this package follows:
//   - repositories are opened only through native.OpenRepo (EnableDotGitCommonDir set), with a
//     fresh handle per call and no cached *git.Repository (go-git caches pack indexes per
//     handle and is not safe to share, plan handle-lifetime rule);
//   - it never starts a process and never writes;
//   - a SHA is returned only after its object was read back, so a dangling ref never escapes
//     (spike S7);
//   - it never imports session/git, session/tmux, config or executor/safeexec.
package gogit

import (
	"context"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// Backend is the go-git implementation of backend.Backend. It is safe for concurrent use: it
// holds no per-repository state.
type Backend struct {
	environ    func() []string
	retries    int
	retryDelay time.Duration
	sleep      func(context.Context, time.Duration)
	uid        func() int
	systemCfgs []string
}

var _ backend.Backend = (*Backend)(nil)

// Option configures New.
type Option func(*Backend)

// WithEnviron sets the environment the backend consults for the variables git itself reads
// (GIT_DIR, GIT_CONFIG_GLOBAL, HOME, ...), in "KEY=value" form. It defaults to os.Environ at
// call time. A Runner that gives git a scrubbed environment should pass the same one here, so
// both implementations see the same configuration.
func WithEnviron(environ func() []string) Option {
	return func(b *Backend) { b.environ = environ }
}

// Defaults of the cohort retry policy for a torn read (plan Story 2.1.2a): 3 re-reads, 20 ms apart.
const (
	defaultRetries    = 3
	defaultRetryDelay = 20 * time.Millisecond
)

// New returns a go-git backend.
func New(opts ...Option) *Backend {
	b := &Backend{
		environ: os.Environ, retries: defaultRetries, retryDelay: defaultRetryDelay, sleep: sleepCtx,
		uid: effectiveUID, systemCfgs: systemConfigCandidates,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
