// Package testenv holds env-isolation helpers for tests. Stdlib only (no import
// cycle from any _test.go file). Tests opt in per helper: a global test env var
// would break tests that assert their own socket or home handling.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gitConfig = "[user]\n\tname = Test User\n\temail = test@example.invalid\n[init]\n\tdefaultBranch = main\n"

// IsolatedHome points HOME at a fresh temp dir holding a .gitconfig with an
// identity, so git-committing tests work without touching the real home.
func IsolatedHome(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	return home
}

// ShortTempDir returns a temp dir under /tmp so unix socket paths stay below the
// 104-byte sun_path limit even when TMPDIR or TEST_TMPDIR is long.
func ShortTempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ssq-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// UniqueTmuxSocket sets STAPLER_SQUAD_TMUX_SOCKET to a name unique to this test
// process and returns it, keeping the test off the default tmux server.
func UniqueTmuxSocket(t testing.TB) string {
	t.Helper()
	name := fmt.Sprintf("ssq-test-%d-%d", os.Getpid(), os.Getuid())
	t.Setenv("STAPLER_SQUAD_TMUX_SOCKET", name)
	return name
}

// LiveStateError returns an error when, under Bazel (TEST_TMPDIR set), the
// stapler-squad state dir would resolve to the real ~/.stapler-squad. It mirrors
// config.GetConfigDirForDir's priorities without importing it. Nil outside Bazel.
func LiveStateError() error {
	tmp := os.Getenv("TEST_TMPDIR")
	if tmp == "" {
		return nil
	}
	if os.Getenv("STAPLER_SQUAD_INSTANCE") == "shared" {
		return fmt.Errorf("STAPLER_SQUAD_INSTANCE=shared resolves to the live ~/.stapler-squad")
	}
	if dir := os.Getenv("STAPLER_SQUAD_TEST_DIR"); dir != "" {
		return requireUnder("STAPLER_SQUAD_TEST_DIR", dir, tmp)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return requireUnder("HOME", home, tmp)
}

// RequireIsolatedState fails the test when LiveStateError reports a problem.
func RequireIsolatedState(t testing.TB) {
	t.Helper()
	if err := LiveStateError(); err != nil {
		t.Fatalf("refusing to run against live state: %v", err)
	}
}

func requireUnder(what, dir, root string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s=%s is outside TEST_TMPDIR=%s", what, dir, root)
	}
	return nil
}

// SkipUnderBazel skips a test that reads parts of the checkout a Bazel sandbox cannot
// contain (.git, or trees under .bazelignore such as website/); it still runs under go test.
func SkipUnderBazel(t testing.TB, why string) {
	t.Helper()
	if os.Getenv("TEST_TMPDIR") != "" && os.Getenv("TEST_SRCDIR") != "" {
		t.Skip(why + " (absent from the Bazel sandbox; runs under go test)")
	}
}

// SkipGoListUnderBazel skips a test that shells out to `go list` over the module.
// A Bazel test sandbox has neither the synced gen/ and session/ent trees nor the Go
// module cache, so `go list` cannot resolve the graph there; the same test still runs
// under `go test`, and the Bazel-native twin is the package's forbidden_deps_test target
// (tools/bzl/forbidden_deps.bzl). Tracked in https://github.com/tstapler/stapler-squad/issues/1423.
func SkipGoListUnderBazel(t testing.TB) {
	t.Helper()
	if os.Getenv("TEST_TMPDIR") != "" && os.Getenv("TEST_SRCDIR") != "" {
		t.Skip("go list needs the module cache and generated trees, absent in a Bazel sandbox (#1423); enforced there by the forbidden_deps_test target (tools/bzl/forbidden_deps.bzl), here under go test")
	}
}
