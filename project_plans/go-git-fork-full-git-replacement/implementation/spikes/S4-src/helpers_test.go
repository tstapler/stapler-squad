package s4lock

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
)

var gitEnv = append(os.Environ(),
	"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0")

func sig() *object.Signature {
	return &object.Signature{Name: "t", Email: "t@t", When: time.Unix(1700000000, 0)}
}

func gitRun(dir string, args ...string) (string, string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = gitEnv
	var o, e bytes.Buffer
	c.Stdout, c.Stderr = &o, &e
	err := c.Run()
	return o.String(), e.String(), err
}

func cg(t testing.TB, dir string, args ...string) string {
	t.Helper()
	o, e, err := gitRun(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, e)
	}
	return strings.TrimSpace(o)
}

func write(t testing.TB, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repo with n tracked files (one commit, "base") on branch master.
func newRepo(t testing.TB, n int) string {
	t.Helper()
	d := t.TempDir()
	cg(t, d, "init", "-q", "-b", "master")
	for i := 0; i < n; i++ {
		write(t, d, fmt.Sprintf("d%02d/f%05d.txt", i%50, i), fmt.Sprintf("content %d\n", i))
	}
	cg(t, d, "add", "-A")
	cg(t, d, "commit", "-qm", "base")
	return d
}

func lsFiles(t testing.TB, d string) []string {
	t.Helper()
	o := cg(t, d, "ls-files")
	if o == "" {
		return nil
	}
	return strings.Split(o, "\n")
}

func noLocks(t testing.TB, d string) {
	t.Helper()
	var found []string
	filepath.Walk(filepath.Join(d, ".git"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".lock") {
			found = append(found, p)
		}
		return nil
	})
	if len(found) > 0 {
		t.Fatalf("leaked locks: %v", found)
	}
}
