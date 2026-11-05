package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

// stubLiveFinder is a test double for LiveInstanceFinder that returns a fixed instance.
type stubLiveFinder struct {
	inst *session.Instance
}

func (s *stubLiveFinder) FindLiveInstance(_ string) *session.Instance {
	return s.inst
}

// newVCSStatusTestRepo creates a temp git repo with two commits on "main":
// baseSHA (one file) then headSHA (a second file added) — so a caller with
// baseSHA recorded sees exactly one unshipped commit and one changed file.
func newVCSStatusTestRepo(t *testing.T) (dir, baseSHA, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "base commit")
	baseSHA = strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0o644))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "second commit")
	headSHA = strings.TrimSpace(runGitOutput(t, dir, "rev-parse", "HEAD"))
	return dir, baseSHA, headSHA
}
