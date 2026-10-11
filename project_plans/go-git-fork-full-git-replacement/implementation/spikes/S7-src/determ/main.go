// Deterministic single-threaded probe: linked worktree HEAD right after CLI `git worktree add`.
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
	root, _ := os.MkdirTemp("", "determ")
	main := filepath.Join(root, "main")
	os.MkdirAll(main, 0o755)
	sh(main, "init", "-q", "-b", "main")
	sh(main, "commit", "-q", "--allow-empty", "-m", "c1")
	c1 := sh(main, "rev-parse", "HEAD")
	sh(main, "commit", "-q", "--allow-empty", "-m", "c2")
	c2 := sh(main, "rev-parse", "HEAD")
	wt := filepath.Join(root, "wt")
	sh(main, "worktree", "add", "-q", "-b", "wt", wt, c1)
	sh(wt, "commit", "-q", "--allow-empty", "-m", "w1")
	w1 := sh(wt, "rev-parse", "HEAD")
	fmt.Println("c1", c1, "c2(main tip)", c2, "wt tip (cli)", w1)
	for _, common := range []bool{false, true} {
		repo, err := git.PlainOpenWithOptions(wt, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: common})
		if err != nil {
			fmt.Println("common", common, "open err", err)
			continue
		}
		h, err := repo.Head()
		if err != nil {
			fmt.Println("common", common, "Head err:", err)
			continue
		}
		_, cerr := repo.CommitObject(h.Hash())
		fmt.Printf("common=%v Head=%s name=%s commitObjErr=%v\n", common, h.Hash(), h.Name(), cerr)
	}
}
