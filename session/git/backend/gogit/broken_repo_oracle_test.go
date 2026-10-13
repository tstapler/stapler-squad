//go:build gitoracle

package gogit_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// brokenRepoOps is the whole served surface (see servedOperations) on a repository git may
// reject: the answers are the layout and config reads that run before any ref is touched.
func brokenRepoOps() []gitoracle.Op {
	ops := append(simpleOps(), resolveOps([]string{"HEAD", "main", "nonexistent"})...)
	ops = append(ops, listRefOps(false)...)
	ops = append(ops, mergeBaseOps("broken-repos")[:3]...)
	for _, key := range []string{"user.name", "core.bare", "missing.key"} {
		k := backend.ConfigKey(key)
		ops = append(ops, op("GetConfig("+key+")", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.GetConfig(ctx, l, k)
		}))
	}
	return ops
}

// fixtureBrokenRepos: every way a work tree's repository link can dangle or be damaged. The
// shadow soak recorded the first one (a linked worktree whose admin directory under
// <main>/.git/worktrees was removed): git says "not a git repository" and stops, without
// searching parent directories, so the candidate must answer ErrNotARepo too, not fall back.
var fixtureBrokenRepos = gitoracle.Fixture{Name: "broken-repos", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	must := func(err error) {
		g.T.Helper()
		if err != nil {
			g.T.Fatal(err)
		}
	}
	write := func(path, content string) {
		g.T.Helper()
		must(os.MkdirAll(filepath.Dir(path), 0o700))
		must(os.WriteFile(path, []byte(content), 0o600))
	}
	mkdir := func(path string) string {
		g.T.Helper()
		must(os.MkdirAll(path, 0o700))
		return path
	}
	main := filepath.Join(root, "main")
	mkdir(main)
	initMain(g, main)
	commitFile(g, main, "a.txt", "1\n")
	commitFile(g, main, "b.txt", "2\n")
	places := gitoracle.Places{"main": main}

	// A damaged admin directory makes `git worktree add` itself fail, so every linked worktree
	// is created first and damaged afterwards.
	damages := []struct {
		name   string
		damage func(admin string)
	}{
		{"wt-admin-deleted", func(admin string) { must(os.RemoveAll(admin)) }},
		{"wt-admin-emptied", func(admin string) { must(os.RemoveAll(admin)); mkdir(admin) }},
		{"wt-admin-no-head", func(admin string) { must(os.Remove(filepath.Join(admin, "HEAD"))) }},
		{"wt-admin-no-commondir", func(admin string) { must(os.Remove(filepath.Join(admin, "commondir"))) }},
		{"wt-admin-commondir-dangling", func(admin string) { write(filepath.Join(admin, "commondir"), "../../nowhere\n") }},
		{"wt-admin-commondir-empty", func(admin string) { write(filepath.Join(admin, "commondir"), "") }},
		{"wt-admin-head-garbage", func(admin string) { write(filepath.Join(admin, "HEAD"), "garbage\n") }},
		{"wt-admin-head-empty", func(admin string) { write(filepath.Join(admin, "HEAD"), "") }},
		{"wt-admin-no-backlink", func(admin string) { must(os.Remove(filepath.Join(admin, "gitdir"))) }},
		{"wt-admin-is-file", func(admin string) { must(os.RemoveAll(admin)); write(admin, "not a directory\n") }},
		{"wt-admin-is-dangling-symlink", func(admin string) {
			must(os.RemoveAll(admin))
			must(os.Symlink(filepath.Join(root, "nowhere"), admin))
		}},
	}
	for _, d := range damages {
		dir := filepath.Join(root, d.name)
		g.Run(main, "worktree", "add", "-q", "-b", d.name, dir)
		places[d.name] = dir
	}
	for _, d := range damages {
		d.damage(filepath.Join(main, ".git", "worktrees", d.name))
	}
	places["wt-admin-deleted-subdir"] = mkdir(filepath.Join(places["wt-admin-deleted"], "sub"))

	gitfile := func(name, content string) string {
		dir := mkdir(filepath.Join(root, name))
		write(filepath.Join(dir, ".git"), content)
		places[name] = dir
		return dir
	}

	// .git files
	gitfile("gitfile-missing-absolute", "gitdir: "+filepath.Join(root, "nowhere")+"\n")
	gitfile("gitfile-missing-relative", "gitdir: ../nowhere\n")
	gitfile("gitfile-missing-no-newline", "gitdir: "+filepath.Join(root, "nowhere"))
	gitfile("gitfile-to-main-gitdir", "gitdir: "+filepath.Join(main, ".git")+"\n")
	gitfile("gitfile-to-empty-dir", "gitdir: "+mkdir(filepath.Join(root, "empty-target"))+"\n")
	gitfile("gitfile-to-file", "gitdir: "+filepath.Join(main, "a.txt")+"\n")
	gitfile("gitfile-to-worktree-root", "gitdir: "+main+"\n")
	gitfile("gitfile-garbage", "garbage\n")
	gitfile("gitfile-empty", "")
	gitfile("gitfile-no-path", "gitdir: \n")
	gitfile("gitfile-no-space", "gitdir:"+filepath.Join(main, ".git")+"\n")
	gitfile("gitfile-two-lines", "gitdir: "+filepath.Join(root, "nowhere")+"\nsecond\n")
	gitfile("gitfile-trailing-spaces", "gitdir: "+main+"/.git   \n")
	gitfile("gitfile-trailing-tab", "gitdir: "+main+"/.git\t\n")
	// a relative target that climbs through a symlink resolves through the OS, not lexically
	must(os.Symlink(filepath.Join(root, "wt-admin-deleted"), filepath.Join(root, "link")))
	gitfile("gitfile-relative-through-symlink", "gitdir: ../link/../main/.git\n")
	gitfile("gitfile-loop", "gitdir: "+filepath.Join(root, "gitfile-loop", ".git")+"\n")
	gitfile("gitfile-crlf", "gitdir: "+filepath.Join(root, "nowhere")+"\r\n")
	// a parent repository exists above this one
	write(filepath.Join(main, "sub-broken", ".git"), "gitdir: "+filepath.Join(root, "nowhere")+"\n")
	places["gitfile-in-repo"] = filepath.Join(main, "sub-broken")
	must(os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(mkdir(filepath.Join(root, "dotgit-dangling-symlink")), ".git")))
	places["dotgit-dangling-symlink"] = filepath.Join(root, "dotgit-dangling-symlink")

	// .git directories and bare directories
	dotgitDir := func(name string, files map[string]string, dirs ...string) {
		dir := mkdir(filepath.Join(root, name))
		mkdir(filepath.Join(dir, ".git"))
		for _, d := range dirs {
			mkdir(filepath.Join(dir, ".git", d))
		}
		for f, c := range files {
			write(filepath.Join(dir, ".git", f), c)
		}
		places[name] = dir
	}
	dotgitDir("dotgit-empty", nil)
	dotgitDir("dotgit-head-only", map[string]string{"HEAD": "ref: refs/heads/main\n"})
	dotgitDir("dotgit-no-objects", map[string]string{"HEAD": "ref: refs/heads/main\n"}, "refs")
	dotgitDir("dotgit-no-refs", map[string]string{"HEAD": "ref: refs/heads/main\n"}, "objects")
	dotgitDir("dotgit-head-garbage", map[string]string{"HEAD": "garbage\n"}, "objects", "refs")
	dotgitDir("dotgit-head-empty", map[string]string{"HEAD": ""}, "objects", "refs")
	dotgitDir("dotgit-skeleton-unborn", map[string]string{"HEAD": "ref: refs/heads/main\n"}, "objects", "refs")
	places["plain-empty"] = mkdir(filepath.Join(root, "plain-empty"))
	places["bare-empty"] = mkdir(filepath.Join(root, "bare-empty.git"))
	places["bare-no-objects"] = mkdir(filepath.Join(root, "bare-no-objects.git"))
	write(filepath.Join(places["bare-no-objects"], "HEAD"), "ref: refs/heads/main\n")
	mkdir(filepath.Join(places["bare-no-objects"], "refs"))
	return places
}}

// TestParityBrokenRepos: git's verdict on a damaged repository is part of the contract, and
// unlike the other fixtures no fallback is excused unless brokenRepoFallback names it. The
// shadow soak classifies "git says not-a-repository, the candidate falls back" as a real
// mismatch, so a silent fallback here would resurface there.
func TestParityBrokenRepos(t *testing.T) {
	diffs := gitoracle.RunOps(t, newCandidate(t), brokenRepoOps(), []gitoracle.Fixture{fixtureBrokenRepos})
	var unexpected []string
	for _, d := range diffs {
		if d.Kind == gitoracle.DiffError && strings.Contains(d.Candidate, "fallback eligible") && brokenRepoFallback(d) {
			continue
		}
		unexpected = append(unexpected, oneLine(d))
	}
	sort.Strings(unexpected)
	for _, u := range unexpected {
		t.Error(u)
	}
}

// cliOnlyBrokenPlaces are damaged shapes where git itself dies with a plain fatal error (a
// CommandError, no sentinel), so there is nothing typed to reproduce. The backend hands them to
// the CLI, and the shadow comparator sees two undifferentiated failures as the same answer.
var cliOnlyBrokenPlaces = map[string]bool{
	"gitfile-empty": true, "gitfile-garbage": true, "gitfile-no-path": true, "gitfile-no-space": true,
	"wt-admin-commondir-empty": true,
}

// brokenRepoFallback excuses a fallback only where the CLI's answer is such an untyped failure:
// a fallback where git says "not a git repository" is exactly the divergence shadow reports.
func brokenRepoFallback(d gitoracle.Diff) bool {
	return cliOnlyBrokenPlaces[d.Place] && strings.HasPrefix(d.Oracle, "CommandError")
}
