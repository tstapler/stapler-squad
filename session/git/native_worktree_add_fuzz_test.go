package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// maxFuzzWorktreeNameBytes caps the raw fuzzer input consumed per iteration — large
// enough to still exercise "very long name" inputs well past any human-chosen branch
// name, without letting a single generated input dominate the fuzz budget on filesystem
// I/O (each iteration allocates a real temp repo and worktree directory).
const maxFuzzWorktreeNameBytes = 2048

// FuzzNativeWorktreeAdd fuzzes nativeSetupNewWorktree's name-handling — AllocateAdminDirName
// (session/git/native_worktree_add.go) and the admin-file/checkout pipeline it drives —
// against real git recognition (Story 5.2.2). Epic 5.1's WorktreeAdminFixture
// (native_worktree_fixture_test.go) was not present in the tree when this was written
// (checked via `ls`/`git log` per plan.md's fallback instruction), so success is verified
// directly with a real `git worktree list --porcelain` subprocess instead — the same
// pattern this package's own non-fuzz tests already use (see
// TestNativeSetupNewWorktree_CrashBetweenGitdirAndCommondir_IsPrunableToRealGit in
// native_worktree_add_test.go).
func FuzzNativeWorktreeAdd(f *testing.F) {
	f.Add([]byte("feature-x"))
	f.Add([]byte("功能/分支-名稱"))      // unicode
	f.Add([]byte("weird/../name")) // path-separator-adjacent
	f.Add([]byte("../../etc/passwd"))
	f.Add([]byte(strings.Repeat("a", 4000))) // very long

	f.Fuzz(func(t *testing.T, raw []byte) {
		name := sanitizeFuzzWorktreeName(raw)

		repoPath := setupTestRepo(t)
		tmpRoot := t.TempDir()
		worktreePath := filepath.Join(tmpRoot, "wt-"+name)

		// Test-harness safety net, not a claim about nativeSetupNewWorktree's own
		// production behavior: confirm the sanitized name didn't survive filepath.Join's
		// Clean into something that escapes tmpRoot before ever calling into the
		// function under test.
		rel, relErr := filepath.Rel(tmpRoot, worktreePath)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Skip("sanitized name escaped the temp worktree root")
		}

		wt := NewGitWorktreeFromStorageWithExecutor(repoPath, worktreePath, "fuzz-worktree-add", name, "")
		if err := wt.nativeSetupNewWorktree(); err != nil {
			// Real git itself rejects plenty of ref/path names outright (leading '-',
			// "..", a trailing ".lock", embedded spaces, a name that collides with an
			// existing file, ENAMETOOLONG past filesystem limits, ...) — an error here is
			// an expected, valid outcome for an adversarial name (per this function's own
			// doc comment and plan.md Task 5.2.2a), not a fuzz failure. Only a panic
			// (caught automatically by the fuzzing engine) or a corrupted on-disk state
			// for a *successful* add (checked below) counts as one.
			return
		}

		assertWorktreeRealGitRecognized(t, repoPath, worktreePath)
	})
}

// sanitizeFuzzWorktreeName restricts raw fuzzer bytes to a conservative charset before
// handing them to nativeSetupNewWorktree as both a git branch name and (path-joined) a
// worktree directory name: valid UTF-8 only, no control characters (including NUL), and
// no backslash (an alternate path separator on no platform this project targets, but
// excluded so an adversarial "..\\.." can't be assembled from '.', '\\', '.', '.').
//
// Real git's own considerably stricter ref-name rules (no leading '-', no "..", no
// trailing ".lock", no embedded space, no "~^:?*[" — see `git check-ref-format`) are
// deliberately NOT reimplemented here: a name that survives this sanitizer but still gets
// rejected by nativeSetupNewWorktree or the go-git checkout underneath it is an expected,
// valid fuzz outcome (an error return), not something this sanitizer needs to prevent —
// see FuzzNativeWorktreeAdd's own error-handling comment.
func sanitizeFuzzWorktreeName(raw []byte) string {
	if len(raw) > maxFuzzWorktreeNameBytes {
		raw = raw[:maxFuzzWorktreeNameBytes]
	}
	var b strings.Builder
	for _, r := range string(raw) {
		if r == utf8.RuneError || unicode.IsControl(r) || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	name := b.String()
	if name == "" || name == "." || name == ".." {
		name = "fuzz-worktree"
	}
	return name
}

// assertWorktreeRealGitRecognized asserts a successfully-added worktree is recognized by
// real git as live (present, and not "prunable") — Story 5.2.2's acceptance criterion,
// checked via a real `git worktree list --porcelain` subprocess against repoPath, matching
// this package's own established convention (runRealGit in native_worktree_add_test.go).
func assertWorktreeRealGitRecognized(t *testing.T, repoPath, worktreePath string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", "worktree", "list", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "real git worktree list failed: %s", out)

	output := string(out)
	require.Contains(t, output, filepath.Base(worktreePath),
		"a successfully-added worktree must be listed by real git: %s", output)
	require.NotContains(t, output, "prunable",
		"a successfully-added worktree must not be reported prunable: %s", output)
}
