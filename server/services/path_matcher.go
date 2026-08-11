package services

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// PathClassification is a bitmask that categorises a resolved filesystem path.
// Multiple bits may be set simultaneously (e.g. a path can be both PathSystemDir
// and PathGitRepo if the repository lives under /usr/local/src).
type PathClassification uint

const (
	// PathRoot matches a path that is exactly the filesystem root ("/").
	PathRoot PathClassification = 1 << iota
	// PathHome matches a path that is exactly the current user's home directory.
	PathHome
	// PathSystemDir matches a path that is, or is inside, one of the conventional
	// Unix system directories: /etc, /usr, /bin, /sbin, /lib, /lib64, /boot,
	// /dev, /sys, /proc, /run.
	PathSystemDir
	// PathTempDir matches a path that is, or is inside, a temporary directory
	// (/tmp, /var/tmp, or the value of $TMPDIR).
	PathTempDir
	// PathCwd matches a path that is, or is inside, the current working directory
	// as reported by classifier.ClassificationContext.Cwd.
	PathCwd
	// PathGitRepo matches a path that is, or is inside, the git repository root
	// as reported by classifier.ClassificationContext.RepoRoot.
	PathGitRepo
)

// systemDirs is the set of root-level system directory prefixes used by ClassifyPath.
var systemDirs = []string{
	"/etc", "/usr", "/bin", "/sbin",
	"/lib", "/lib64", "/boot",
	"/dev", "/sys", "/proc", "/run",
}

// ExpandPath expands tilde and environment variables in a single path token and
// normalises absolute paths with filepath.Clean.
//
//   - "~"       → home directory
//   - "~/foo"   → filepath.Join(home, "foo")
//   - anything else → os.ExpandEnv (handles $HOME, $TMPDIR, $VAR, …)
//
// After expansion, absolute paths are passed through filepath.Clean to remove
// redundant separators and ".." components (e.g. "/foo/../bar" → "/bar").
func ExpandPath(arg string) string {
	homeDir, _ := os.UserHomeDir()

	// Tilde expansion must happen before os.ExpandEnv so that "~" is not
	// misinterpreted as a shell variable.
	switch {
	case arg == "~":
		arg = homeDir
	case strings.HasPrefix(arg, "~/"):
		arg = filepath.Join(homeDir, arg[2:])
	default:
		arg = os.ExpandEnv(arg)
	}

	if filepath.IsAbs(arg) {
		arg = filepath.Clean(arg)
	}
	return arg
}

// ClassifyPath returns a PathClassification bitmask for path.
// path should already be expanded (via ExpandPath) before calling this function.
func ClassifyPath(path string, ctx classifier.ClassificationContext) PathClassification {
	var c PathClassification

	homeDir, _ := os.UserHomeDir()

	// Normalise for comparison, but only if the path is absolute.
	cleaned := path
	if filepath.IsAbs(path) {
		cleaned = filepath.Clean(path)
	}

	// PathRoot
	if cleaned == "/" {
		c |= PathRoot
	}

	// PathHome
	if homeDir != "" && cleaned == filepath.Clean(homeDir) {
		c |= PathHome
	}

	// PathSystemDir
	for _, sysDir := range systemDirs {
		if cleaned == sysDir || strings.HasPrefix(cleaned, sysDir+"/") {
			c |= PathSystemDir
			break
		}
	}

	// PathTempDir
	tmpDir := os.Getenv("TMPDIR")
	tempRoots := []string{"/tmp", "/var/tmp"}
	if tmpDir != "" {
		if t := filepath.Clean(tmpDir); t != "" {
			tempRoots = append(tempRoots, t)
		}
	}
	for _, t := range tempRoots {
		if cleaned == t || strings.HasPrefix(cleaned, t+"/") {
			c |= PathTempDir
			break
		}
	}

	// PathCwd
	if ctx.Cwd != "" {
		cwd := filepath.Clean(ctx.Cwd)
		if cleaned == cwd || strings.HasPrefix(cleaned, cwd+"/") {
			c |= PathCwd
		}
	}

	// PathGitRepo
	if ctx.RepoRoot != "" {
		repoRoot := filepath.Clean(ctx.RepoRoot)
		if cleaned == repoRoot || strings.HasPrefix(cleaned, repoRoot+"/") {
			c |= PathGitRepo
		}
	}

	return c
}

// PathMatcher provides structured path-based matching for Bash command arguments.
// It operates on the expanded (tilde/env-resolved) arguments of a parsed command and
// can be used alongside CommandPattern and Criteria on a Rule — all set fields must
// match (AND semantics).
//
// Example — block rm on root or home:
//
//	PathMatcher: &PathMatcher{ArgIndex: -1, MatchIf: PathRoot | PathHome}
//
// Example — reject if path is inside /tmp (allow-list inversion):
//
//	PathMatcher: &PathMatcher{ArgIndex: 0, RejectIf: PathTempDir}
type PathMatcher struct {
	// ArgIndex selects which non-flag argument to evaluate.
	// -1 (the default) means any non-flag argument may satisfy the match.
	// 0 means the first non-flag argument, 1 the second, and so on.
	ArgIndex int

	// MatchIf: at least one selected argument must match one of these
	// classifications for Matches to return true.
	// Zero means this check is skipped (always passes).
	MatchIf PathClassification

	// RejectIf: if any selected argument matches any of these classifications,
	// Matches returns false immediately regardless of MatchIf.
	// Zero means this check is skipped (never rejects).
	RejectIf PathClassification
}

// nonFlagArgs returns the subset of args that do not start with "-".
func nonFlagArgs(args []string) []string {
	out := args[:0:0] // reuse backing array only if non-empty
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

// Matches returns true when the path arguments in expandedArgs satisfy the
// PathMatcher criteria against ctx.
func (pm *PathMatcher) Matches(expandedArgs []string, ctx classifier.ClassificationContext) bool {
	pathArgs := nonFlagArgs(expandedArgs)

	// Select the candidate(s) to inspect.
	var candidates []string
	if pm.ArgIndex >= 0 {
		if pm.ArgIndex < len(pathArgs) {
			candidates = pathArgs[pm.ArgIndex : pm.ArgIndex+1]
		}
		// ArgIndex out of range → no candidates → no match.
	} else {
		candidates = pathArgs
	}

	if len(candidates) == 0 {
		return false
	}

	matchIfSatisfied := pm.MatchIf == 0 // if MatchIf is zero we skip the check

	for _, arg := range candidates {
		class := ClassifyPath(arg, ctx)

		// RejectIf vetoes immediately.
		if pm.RejectIf != 0 && (class&pm.RejectIf != 0) {
			return false
		}

		// MatchIf: record a hit.
		if pm.MatchIf != 0 && (class&pm.MatchIf != 0) {
			matchIfSatisfied = true
		}
	}

	return matchIfSatisfied
}
