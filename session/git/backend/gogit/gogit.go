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
//   - object storage is opened over the directories the layout scan resolved (repoview.go,
//     hostfs.go), not by go-git's own path discovery, so symlinked git directories work; the
//     handle is fresh per call and never cached (go-git caches pack indexes per handle and is
//     not safe to share, plan handle-lifetime rule);
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
	// systemCfgs and vendorCfgs are the system-scope files: the one GIT_CONFIG_SYSTEM replaces,
	// and vendor additions (Apple git's share/git-core/gitconfig) it does not replace.
	systemCfgs, vendorCfgs []string
	// systemKnown is true when those lists are authoritative (WithSystemConfig): each file is
	// read. Otherwise they are guessed candidates that are only scanned for the key, which makes
	// a read fall back to the CLI when it matters.
	systemKnown bool
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

// WithSystemConfig declares the exact system-wide config files git reads, in read order. git's
// own path is compiled into the binary and cannot be learned without running it, so this
// package never guesses it for you: the composition root learns it once by asking git
// (session/gitwiring.DiscoverSystemConfig) and passes it here. Each file is read (missing ones
// are skipped, like git does). GIT_CONFIG_SYSTEM replaces these files and GIT_CONFIG_NOSYSTEM
// drops them and the vendor files; calling it with no paths declares there is no system config.
//
// Without this option the backend falls back to guessed well-known locations that are only
// scanned: a read whose key appears in one of them goes to the CLI instead of being answered.
func WithSystemConfig(paths ...string) Option {
	return func(b *Backend) { b.systemCfgs, b.systemKnown = append([]string(nil), paths...), true }
}

// WithVendorSystemConfig declares config files a git build reads in addition to its system file
// and that GIT_CONFIG_SYSTEM does not replace (Apple git reads
// /Library/Developer/CommandLineTools/usr/share/git-core/gitconfig this way; only
// GIT_CONFIG_NOSYSTEM drops it). Declaring them also marks the system config as known.
func WithVendorSystemConfig(paths ...string) Option {
	return func(b *Backend) { b.vendorCfgs, b.systemKnown = append([]string(nil), paths...), true }
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
		uid: effectiveUID, systemCfgs: systemConfigCandidates, vendorCfgs: vendorConfigCandidates,
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
