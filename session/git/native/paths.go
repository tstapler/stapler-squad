package native

import (
	"errors"
	"io/fs"
	"path/filepath"
)

// CanonicalizeWorktreePath resolves path to its symlink-free (realpath'd) form,
// matching what `git worktree list --porcelain` reports and what
// getWorktreeDirectory already produces for freshly-created worktree parents.
// On macOS /var (and /tmp) is itself a symlink to /private/var, so two code
// paths that construct the "same" worktree path differently — one via
// filepath.Join on an unresolved parent, the other by reading git's
// already-resolved output — end up as different strings for the identical
// directory (see TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession).
// EvalSymlinks requires the path to exist, which doesn't hold for the
// pre-creation/rehydration cases this is also used in; for those the nearest existing
// ancestor is resolved and the missing tail re-appended, so the result is the same
// string the path will resolve to once created (a bare filepath.Clean left /var
// unresolved on macOS and diverged from git's /private/var output). Any other error
// falls back to filepath.Clean, keeping this a pure, non-failing normalizer, matching
// session/history_detector.go, session/import_correlate.go, and
// session/unfinished/gogitstore/open.go.
func CanonicalizeWorktreePath(path string) string {
	if path == "" {
		return path
	}
	// EvalSymlinks on the raw input: Clean would collapse "lnk/../y" lexically,
	// while git records the physical resolution.
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	cleaned := filepath.Clean(path)
	if !errors.Is(err, fs.ErrNotExist) {
		return cleaned
	}
	parent := filepath.Dir(cleaned)
	if parent == cleaned {
		return cleaned
	}
	return filepath.Join(CanonicalizeWorktreePath(parent), filepath.Base(cleaned))
}
