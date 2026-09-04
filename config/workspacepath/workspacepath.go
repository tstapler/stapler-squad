// Package workspacepath implements the cwd/preference-file-driven directory
// resolution shared by config.GetConfigDirForDir (Priorities 3-5) and
// log.GetConfigDirForDir. It exists to prevent the two callers from drifting,
// as they did before this package existed — see log/log.go's GetConfigDir doc
// comment. Stdlib-only so both config (which imports log) and log can import
// it without a cycle.
package workspacepath

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsTestMode detects if the current process is a go test/benchmark binary, by
// inspecting os.Args the same way config.IsTestMode() does (Priority 3).
func IsTestMode() bool {
	for _, arg := range os.Args {
		if strings.Contains(arg, ".test") ||
			strings.Contains(arg, "-test.") ||
			strings.HasSuffix(arg, ".test.exe") ||
			strings.Contains(arg, "-bench") {
			return true
		}
	}
	return false
}

// PreferredWorkspaceFile returns the path to the preferred-workspace
// preference file (written by the SwitchDatabase RPC) under baseDir.
func PreferredWorkspaceFile(baseDir string) string {
	return filepath.Join(baseDir, "preferred_workspace")
}

// PreferredWorkspaceDir reads and validates Priority 4's preference file
// under baseDir. Returns ("", false) when no file is present, its contents
// don't point inside baseDir, or the target directory no longer exists on
// disk — any of which means the caller should fall through to Priority 5/6.
func PreferredWorkspaceDir(baseDir string) (string, bool) {
	data, err := os.ReadFile(PreferredWorkspaceFile(baseDir))
	if err != nil {
		return "", false
	}
	prefDir := strings.TrimSpace(string(data))
	if !filepath.IsAbs(prefDir) || !IsWithinStateDir(prefDir, baseDir) {
		return "", false
	}
	if _, statErr := os.Stat(prefDir); statErr != nil {
		return "", false
	}
	return prefDir, true
}

// IsWithinStateDir reports whether workDir is baseDir itself or a descendant
// of it. A cwd inside stapler-squad's own state directory (e.g. a session
// worktree under ~/.stapler-squad/workspaces/.../worktrees/...) hashes to a
// workspace distinct from the one normally used from the project/home
// directory.
func IsWithinStateDir(workDir, baseDir string) bool {
	if workDir == "" || baseDir == "" {
		return false
	}
	workDir = filepath.Clean(workDir)
	baseDir = filepath.Clean(baseDir)
	return workDir == baseDir || strings.HasPrefix(workDir, baseDir+string(filepath.Separator))
}

// WorkspaceModeEnabled reports whether Priority 5's opt-in per-directory
// isolation is active. Exact string equality to "true" — not case-insensitive
// or truthy-string parsing — matching the scheme every existing
// STAPLER_SQUAD_WORKSPACE_MODE deployment already relies on.
func WorkspaceModeEnabled() bool {
	return os.Getenv("STAPLER_SQUAD_WORKSPACE_MODE") == "true"
}

// HashWorkspaceID hashes workDir into a stable, filesystem-safe identifier.
// The truncation scheme (first 8 bytes of SHA-256, hex-encoded) must stay
// byte-identical to avoid orphaning existing
// ~/.stapler-squad/workspaces/<hash>/ directories on disk.
func HashWorkspaceID(workDir string) string {
	hash := sha256.Sum256([]byte(workDir))
	return fmt.Sprintf("%x", hash[:8])
}

// WorkspaceModeDir returns Priority 5's per-cwd hashed workspace directory
// under baseDir for the given (already-resolved) workDir.
func WorkspaceModeDir(baseDir, workDir string) string {
	return filepath.Join(baseDir, "workspaces", HashWorkspaceID(workDir))
}

// ResolveDefaultDirResult carries Priority 4-6's resolved directory plus
// diagnostics the caller may want to log — kept out of this leaf package so
// it stays free of a log dependency (config already imports log; log is
// where the log package's own equivalent lives).
type ResolveDefaultDirResult struct {
	// Dir is the resolved config directory.
	Dir string
	// WorkDir is the cwd resolved for workspace-mode hashing (Priority 5),
	// set only when that path ran.
	WorkDir string
	// WithinStateDir is true when WorkDir is inside baseDir — the caller
	// should warn, since this usually means the process was started from a
	// worktree instead of the user's normal project/home directory.
	WithinStateDir bool
	// GetwdErr holds an os.Getwd failure hit while resolving workspace mode,
	// if any — the caller should warn, then this result still falls through
	// to Priority 6 (Dir == baseDir).
	GetwdErr error
}

// ResolveDefaultDir implements Priority 4-6 of GetConfigDirForDir — preferred
// workspace file, opt-in per-directory workspace isolation, then shared
// state — as the single implementation shared by config.GetConfigDirForDir
// and log.GetConfigDirForDir, so the two can't drift the way they did before
// this package existed (see log/log.go's GetConfigDir doc comment). dir is
// the caller-supplied cwd override; empty uses os.Getwd() internally.
func ResolveDefaultDir(dir, baseDir string) ResolveDefaultDirResult {
	// Priority 4: Preferred workspace from preference file (SwitchDatabase RPC).
	if prefDir, ok := PreferredWorkspaceDir(baseDir); ok {
		return ResolveDefaultDirResult{Dir: prefDir}
	}

	// Priority 5: Per-directory workspace isolation — opt-in only. See
	// STAPLER_SQUAD_WORKSPACE_MODE's env-var check for why this isn't the
	// default: a single shared workspace, with per-cwd auto-isolation as an
	// explicit user action, avoids a process started from an unusual cwd
	// silently landing on an empty, unrelated workspace.
	if WorkspaceModeEnabled() {
		workDir := dir
		var err error
		if workDir == "" {
			workDir, err = os.Getwd()
		}
		if err == nil && workDir != "" {
			return ResolveDefaultDirResult{
				Dir:            WorkspaceModeDir(baseDir, workDir),
				WorkDir:        workDir,
				WithinStateDir: IsWithinStateDir(workDir, baseDir),
			}
		}
		if err != nil {
			return ResolveDefaultDirResult{Dir: baseDir, GetwdErr: err}
		}
	}

	// Priority 6: Global shared state (default)
	return ResolveDefaultDirResult{Dir: baseDir}
}
