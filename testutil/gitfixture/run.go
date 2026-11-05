package gitfixture

import (
	"context"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// RunOutput runs `git args...` in dir and returns its combined output, failing the test on error.
func RunOutput(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, out)
	}
	return string(out)
}

// Run runs `git args...` in dir for test setup, failing the test on error.
func Run(t testing.TB, dir string, args ...string) {
	t.Helper()
	RunOutput(t, dir, args...)
}

// LsFiles returns the paths tracked in the git index at dir.
func LsFiles(t testing.TB, dir string) []string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", "ls-files")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files failed: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
