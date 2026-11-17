package svctest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/instance"
)

// stubLiveFinder is a test double for LiveInstanceFinder that returns a fixed instance.
type StubLiveFinder struct {
	Inst *instance.Instance
}

func (s *StubLiveFinder) FindLiveInstance(_ string) *instance.Instance {
	return s.Inst
}

// newVCSStatusTestRepo creates a temp git repo with two commits on "main":
// baseSHA (one file) then headSHA (a second file added) — so a caller with
// baseSHA recorded sees exactly one unshipped commit and one changed file.
func NewVCSStatusTestRepo(t *testing.T) (dir, baseSHA, headSHA string) {
	t.Helper()
	dir = t.TempDir()
	RunGit(t, dir, "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644))
	RunGit(t, dir, "add", ".")
	RunGit(t, dir, "commit", "-q", "-m", "base commit")
	baseSHA = strings.TrimSpace(RunGitOutput(t, dir, "rev-parse", "HEAD"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("two\n"), 0o644))
	RunGit(t, dir, "add", ".")
	RunGit(t, dir, "commit", "-q", "-m", "second commit")
	headSHA = strings.TrimSpace(RunGitOutput(t, dir, "rev-parse", "HEAD"))
	return dir, baseSHA, headSHA
}
