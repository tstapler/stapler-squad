// Class 3 probe: does a long-lived *git.Repository return a stale-but-existing HEAD after CLI
// advances the branch (loose update, then pack-refs --all --prune, then another advance)?
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
)

func sh(dir string, args ...string) string {
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@a", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@a")
	out, err := c.CombinedOutput()
	if err != nil {
		panic(fmt.Sprint(args, err, string(out)))
	}
	return strings.TrimSpace(string(out))
}

func main() {
	root, _ := os.MkdirTemp("", "ll")
	m := filepath.Join(root, "m")
	os.MkdirAll(m, 0o755)
	sh(m, "init", "-q", "-b", "main")
	sh(m, "commit", "-q", "--allow-empty", "-m", "c1")
	wt := filepath.Join(root, "wt")
	sh(m, "worktree", "add", "-q", "-b", "wt", wt)
	repo, _ := git.PlainOpenWithOptions(wt, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	stale := 0
	for i := 0; i < 30; i++ {
		sh(wt, "commit", "-q", "--allow-empty", "-m", fmt.Sprint("w", i))
		switch i % 3 {
		case 0:
			sh(m, "pack-refs", "--all", "--prune")
		case 1:
			sh(m, "pack-refs", "--all")
		}
		want := sh(wt, "rev-parse", "HEAD")
		h, err := repo.Head()
		if err != nil || h.Hash().String() != want {
			stale++
			fmt.Println("iter", i, "go-git (long-lived repo) =", h, err, "cli =", want)
		}
	}
	fmt.Println("long-lived repo stale/mismatch reads:", stale, "of 30")
}
