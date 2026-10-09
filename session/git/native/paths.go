package native

import "path/filepath"

// CanonicalizeWorktreePath resolves path to its symlink-free (realpath'd) form,
// matching what `git worktree list --porcelain` reports and what
// getWorktreeDirectory already produces for freshly-created worktree parents.
// On macOS /var (and /tmp) is itself a symlink to /private/var, so two code
// paths that construct the "same" worktree path differently — one via
// filepath.Join on an unresolved parent, the other by reading git's
// already-resolved output — end up as different strings for the identical
// directory (see TestBacklogFullLifecycle_SDDTriageWorktreeIsReusedBySpawnedWorkSession).
// EvalSymlinks requires the path to exist, which doesn't hold for the
// pre-creation/rehydration cases this is also used in; falling back to
// filepath.Clean on ANY error (not just ENOENT) keeps this a pure, non-failing
// normalizer, matching the established pattern in session/history_detector.go,
// session/import_correlate.go, and session/unfinished/gogitstore/open.go.
func CanonicalizeWorktreePath(path string) string {
	if path == "" {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}
