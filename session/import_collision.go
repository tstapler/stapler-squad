package session

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrPathAlreadyManaged is returned by CheckPathNotAlreadyManaged when
// candidatePath is already covered by an existing managed Instance -- either
// exactly (same directory) or as a subdirectory of an existing Instance's
// worktree/working directory. Importing such a candidate would create a
// second managed Instance pointed at the same on-disk tree, which is exactly
// the dual-writer scenario this whole feature exists to avoid.
var ErrPathAlreadyManaged = errors.New("import commit: path is already managed by an existing session")

// CheckPathNotAlreadyManaged reports ErrPathAlreadyManaged if candidatePath
// (after tilde/relative resolution via ResolveSessionPath) is the same as,
// or a subdirectory of, any existing Instance's Path, WorkingDir, or
// worktree path. registry is typically session.Storage, accessed through
// the InstanceStore interface (ListInstanceData) so this function stays
// testable against a fake without a real ent-backed Storage.
func CheckPathNotAlreadyManaged(candidatePath string, registry InstanceStore) error {
	if registry == nil {
		return fmt.Errorf("check path not already managed: registry is required")
	}

	resolved, err := ResolveSessionPath(candidatePath)
	if err != nil {
		return fmt.Errorf("check path not already managed: %w", err)
	}

	existing, err := registry.ListInstanceData()
	if err != nil {
		return fmt.Errorf("check path not already managed: failed to list instances: %w", err)
	}

	for _, inst := range existing {
		for _, managedPath := range []string{inst.Path, inst.WorkingDir, inst.Worktree.WorktreePath, inst.MainRepoPath} {
			if managedPath == "" {
				continue
			}
			resolvedManaged, err := ResolveSessionPath(managedPath)
			if err != nil {
				continue
			}
			if pathsCollide(resolved, resolvedManaged) {
				return fmt.Errorf("%w: %q collides with existing session %q at %q", ErrPathAlreadyManaged, candidatePath, inst.Title, managedPath)
			}
		}
	}

	return nil
}

// pathsCollide reports whether a and b are the same directory, or one is a
// subdirectory of the other.
func pathsCollide(a, b string) bool {
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}
