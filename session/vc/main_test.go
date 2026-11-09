package vc

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps t.TempDir() outside any VCS ancestor: detection walks up the
// tree, so a TMPDIR under a jj/git-managed $HOME makes every "empty" dir look
// like a repo.
func TestMain(m *testing.M) {
	if isJujutsuRepo(os.TempDir()) || isGitRepo(os.TempDir()) {
		dir, err := os.MkdirTemp("/tmp", "ssq-vc-test-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "vc tests: cannot create hermetic temp dir:", err)
			os.Exit(1)
		}
		os.Setenv("TMPDIR", dir)
		code := m.Run()
		os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}
