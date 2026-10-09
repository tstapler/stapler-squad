package git

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/git/internal/gittest"
)

// runRealGit shells to real git, failing the test on error.
func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return gittest.RunGit(t, dir, args...)
}
