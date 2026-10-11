package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

const rewriteCfg = `[url "git@github.com:"]
	insteadOf = https://github.com/
[url "git@ghe.example.invalid:"]
	insteadOf = https://ghe.example.invalid/
[url "ssh://git@github.com/tstapler/"]
	insteadOf = https://github.com/tstapler/
[url "https://push.example.invalid/"]
	pushInsteadOf = https://github.com/
[url "git@multi.invalid:"]
	insteadOf = https://multi-a.invalid/
	insteadOf = https://multi-b.invalid/
`

// gitOracle asks the real git (test oracle only) what a URL resolves to.
func gitOracle(t *testing.T, env []string, repoDir, rawURL string, push bool) string {
	t.Helper()
	runGit(t, repoDir, "remote", "remove", "o")
	runGit(t, repoDir, "remote", "add", "o", rawURL)
	if push {
		return runGit(t, repoDir, "remote", "get-url", "--push", "o")
	}
	return runGit(t, repoDir, "remote", "get-url", "o")
}

func TestRewriter_should_MatchGitOracle_ForInsteadOfAndPushInsteadOf(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "gc")
	writeFile(t, cfg, "[user]\n\tname=s5\n"+rewriteCfg)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-q")
	runGit(t, repoDir, "remote", "add", "o", "x")
	rw, un := LoadRewriter(cfg)
	if len(un) != 0 {
		t.Fatalf("unsupported: %v", un)
	}
	for _, u := range []string{
		"https://github.com/tstapler/stapler-squad.git", // longest prefix wins
		"https://github.com/other/repo.git",
		"https://ghe.example.invalid/org/r.git",
		"https://multi-a.invalid/x.git",
		"https://multi-b.invalid/x.git", // second insteadOf value for same base
		"https://unrelated.invalid/x.git",
	} {
		for _, push := range []bool{false, true} {
			want := gitOracle(t, nil, repoDir, u, push)
			var got string
			if push {
				got = rw.Push(u)
			} else {
				got = rw.Fetch(u)
			}
			if got != want {
				t.Errorf("push=%v %s: got %q want git %q", push, u, got, want)
			}
		}
	}
}

func TestGoGit_should_OnlyApplyInsteadOfFromRepoLocalConfig_NotGlobal(t *testing.T) {
	gc := filepath.Join(t.TempDir(), "gc")
	writeFile(t, gc, "[url \"git@github.com:\"]\n\tinsteadOf = https://github.com/\n")
	t.Setenv("GIT_CONFIG_GLOBAL", gc)
	// go-git's own global loader ignores GIT_CONFIG_GLOBAL; it reads $HOME/.gitconfig, XDG, ~/.config/git/config.
	paths, _ := config.Paths(config.GlobalScope)
	t.Logf("go-git GlobalScope paths: %v (GIT_CONFIG_GLOBAL not consulted)", paths)
	for _, p := range paths {
		if p == gc {
			t.Fatal("go-git consulted GIT_CONFIG_GLOBAL")
		}
	}

	// 1. repo-local insteadOf IS applied by go-git, via Config.Unmarshal -> applyURLRules.
	local := t.TempDir()
	runGit(t, local, "init", "-q")
	runGit(t, local, "remote", "add", "origin", "https://github.com/x/y.git")
	runGit(t, local, "config", "url.git@github.com:.insteadOf", "https://github.com/")
	repo2, _ := git.PlainOpen(local)
	r, _ := repo2.Remote("origin")
	t.Logf("repo-local insteadOf: git=%q go-git remote URL=%q", runGit(t, local, "remote", "get-url", "origin"), r.Config().URLs[0])
	if r.Config().URLs[0] != "git@github.com:x/y.git" {
		t.Fatalf("go-git did not apply repo-local insteadOf: %q", r.Config().URLs[0])
	}
	_ = config.NewConfig

	// 2. global insteadOf is NOT applied.
	plain := t.TempDir()
	runGit(t, plain, "init", "-q")
	runGit(t, plain, "remote", "add", "origin", "https://github.com/x/y.git")
	repo3, _ := git.PlainOpen(plain)
	r3, _ := repo3.Remote("origin")
	got := r3.Config().URLs[0]
	t.Logf("global-only insteadOf: go-git URL = %q, git oracle = %q", got, runGit(t, plain, "remote", "get-url", "origin"))
	if got != "https://github.com/x/y.git" {
		t.Fatalf("go-git applied global rewrite?! %q", got)
	}
	if want := runGit(t, plain, "remote", "get-url", "origin"); want != "git@github.com:x/y.git" {
		t.Fatalf("oracle: %q", want)
	}

	// 3. [include] is not followed by go-git's loader but is by our resolver; includeIf is reported.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "inc"), "[url \"git@inc.invalid:\"]\n\tinsteadOf = https://inc.invalid/\n")
	writeFile(t, filepath.Join(dir, "main"), "[include]\n\tpath = "+filepath.Join(dir, "inc")+"\n[includeIf \"gitdir:~/work/\"]\n\tpath = x\n")
	loaded, err := config.ReadConfig(strings.NewReader(mustRead(t, filepath.Join(dir, "main"))))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.URLs) != 0 {
		t.Fatal("go-git unexpectedly followed include")
	}
	rw, un := LoadRewriter(filepath.Join(dir, "main"))
	if rw.Fetch("https://inc.invalid/a") != "git@inc.invalid:a" {
		t.Fatalf("include not followed: %q", rw.Fetch("https://inc.invalid/a"))
	}
	if len(un) != 1 || !strings.HasPrefix(un[0], "includeIf.") {
		t.Fatalf("includeIf not reported: %v", un)
	}
}

func mustRead(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
