// Package native holds the on-disk (go-git based, no subprocess) worktree, merge and
// dirty-check code extracted from session/git. It must not import session/git,
// session/tmux, session/lifecycle, config or executor/safeexec; anything it needs from
// those is injected by the caller (see MergeDeps).
package native

import "github.com/go-git/go-git/v5"

// defaultPlainOpenOptions is the single source of truth for how this codebase opens a
// git repository. In particular, EnableDotGitCommonDir must always be set: without it,
// go-git silently resolves objects/refs for a linked worktree (`git worktree add`)
// against the wrong gitdir — not an error, a real-but-wrong result (verified
// empirically: HEAD resolved to a stale SHA from before the worktree was created).
// Read-only after init, so sharing this one instance across every OpenRepo call is safe.
var defaultPlainOpenOptions = &git.PlainOpenOptions{ //nolint:gochecknoglobals shared read-only options, not mutable state
	DetectDotGit:          true,
	EnableDotGitCommonDir: true,
}

// OpenRepo opens the git repository or worktree at path using defaultPlainOpenOptions.
// Every git repository open in this codebase must go through this function (or its
// session/git.OpenRepo alias) rather than a bare git.PlainOpen/PlainOpenWithOptions
// call — see tools/lint's norawgitopen analyzer, which enforces this.
func OpenRepo(path string) (*git.Repository, error) {
	return git.PlainOpenWithOptions(path, defaultPlainOpenOptions) //nolint:norawgitopen this is the wrapper itself
}
