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

// fixtureSymlinkedLayouts: every way a repository's storage can be reached through a symlink,
// plus the on-disk layouts the live service creates (workspaces/*/worktrees, repos/) and awkward
// path characters. go-git's billy chroot refuses to follow a symlink out of its root, so each of
// these once failed in-process ("chroot boundary crossed") while git answered. The shadow
// comparator counts that as a real mismatch, so each shape must match git exactly.
var fixtureSymlinkedLayouts = gitoracle.Fixture{Name: "symlinked-layouts", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	must := func(err error) {
		g.T.Helper()
		if err != nil {
			g.T.Fatal(err)
		}
	}
	mkdir := func(parts ...string) string {
		g.T.Helper()
		p := filepath.Join(parts...)
		must(os.MkdirAll(p, 0o700))
		return p
	}
	link := func(target, name string) { g.T.Helper(); must(os.Symlink(target, name)) }
	write := func(path, content string) {
		g.T.Helper()
		must(os.MkdirAll(filepath.Dir(path), 0o700))
		must(os.WriteFile(path, []byte(content), 0o600))
	}
	places := gitoracle.Places{}
	// seed makes a repository with two commits, a tag, a second branch and a linked worktree.
	seed := func(dir string) {
		initMain(g, dir)
		commitFile(g, dir, "a.txt", "1\n")
		commitFile(g, dir, "b.txt", "2\n")
		g.Run(dir, "tag", "v1", "HEAD~1")
		g.Run(dir, "branch", "feature", "HEAD~1")
	}
	// moveGitDir relocates dir/.git to store and returns the new git directory.
	moveGitDir := func(dir, store string) string {
		g.T.Helper()
		must(os.MkdirAll(filepath.Dir(store), 0o700))
		must(os.Rename(filepath.Join(dir, ".git"), store))
		return store
	}

	main := mkdir(root, "main")
	seed(main)
	places["main"] = main

	// .git is a symlink to a git directory outside the work tree (absolute, relative, chained).
	abs := mkdir(root, "dotgit-symlink-abs")
	seed(abs)
	link(moveGitDir(abs, filepath.Join(root, "store", "abs.git")), filepath.Join(abs, ".git"))
	places["dotgit-symlink-abs"] = abs
	places["dotgit-symlink-abs-sub"] = mkdir(abs, "sub", "dir")

	rel := mkdir(root, "dotgit-symlink-rel")
	seed(rel)
	moveGitDir(rel, filepath.Join(root, "store", "rel.git"))
	link("../store/rel.git", filepath.Join(rel, ".git"))
	places["dotgit-symlink-rel"] = rel

	chain := mkdir(root, "dotgit-symlink-chain")
	seed(chain)
	link(moveGitDir(chain, filepath.Join(root, "store", "chain.git")), filepath.Join(root, "store", "chain-hop"))
	link(filepath.Join(root, "store", "chain-hop"), filepath.Join(chain, ".git"))
	places["dotgit-symlink-chain"] = chain

	// the work tree is reached through a symlink and its .git is a symlink too
	both := mkdir(root, "both-real")
	seed(both)
	link(moveGitDir(both, filepath.Join(root, "store", "both.git")), filepath.Join(both, ".git"))
	link(both, filepath.Join(root, "both-link"))
	places["both-via-link"] = filepath.Join(root, "both-link")

	// linked worktrees of a repository whose .git is a symlink (the worktree .git file and the
	// commondir both name the symlinked path)
	lm := mkdir(root, "linked-main")
	seed(lm)
	link(moveGitDir(lm, filepath.Join(root, "store", "lm.git")), filepath.Join(lm, ".git"))
	g.Run(lm, "worktree", "add", "-q", "-b", "wt", filepath.Join(root, "lm-wt"))
	places["linked-of-symlinked-dotgit"] = filepath.Join(root, "lm-wt")

	// a linked worktree whose admin directory is itself a symlink
	am := mkdir(root, "admin-main")
	seed(am)
	g.Run(am, "worktree", "add", "-q", "-b", "wta", filepath.Join(root, "admin-wt"))
	adminDir := filepath.Join(am, ".git", "worktrees", "admin-wt")
	moved := filepath.Join(root, "store", "admin-wt-admin")
	must(os.MkdirAll(filepath.Dir(moved), 0o700))
	must(os.Rename(adminDir, moved))
	link(moved, adminDir)
	places["admin-dir-symlink"] = filepath.Join(root, "admin-wt")
	places["admin-dir-symlink-main"] = am

	// a .git file naming the git directory through a symlinked path
	gf := mkdir(root, "gitfile-real")
	seed(gf)
	link(filepath.Join(gf, ".git"), filepath.Join(root, "gitdir-link"))
	gfw := mkdir(root, "gitfile-via-symlink")
	write(filepath.Join(gfw, ".git"), "gitdir: "+filepath.Join(root, "gitdir-link")+"\n")
	places["gitfile-via-symlink"] = gfw
	gfr := mkdir(root, "gitfile-via-symlink-rel")
	write(filepath.Join(gfr, ".git"), "gitdir: ../gitdir-link\n")
	places["gitfile-via-symlink-rel"] = gfr

	// --separate-git-dir leaves a .git file; the target also sits behind a symlink
	sep := mkdir(root, "separate")
	seed(sep)
	sepStore := moveGitDir(sep, filepath.Join(root, "store", "sep.git"))
	link(sepStore, filepath.Join(root, "sep-link.git"))
	write(filepath.Join(sep, ".git"), "gitdir: "+filepath.Join(root, "sep-link.git")+"\n")
	places["separate-git-dir-via-symlink"] = sep

	// symlinks inside the git directory
	objs := mkdir(root, "objects-symlink")
	seed(objs)
	moveTo := func(dir, rel, dst string) {
		g.T.Helper()
		src := filepath.Join(dir, ".git", rel)
		must(os.MkdirAll(filepath.Dir(dst), 0o700))
		must(os.Rename(src, dst))
		link(dst, src)
	}
	moveTo(objs, "objects", filepath.Join(root, "store", "objs-objects"))
	places["objects-symlink"] = objs
	refs := mkdir(root, "refs-symlink")
	seed(refs)
	moveTo(refs, "refs", filepath.Join(root, "store", "refs-refs"))
	places["refs-symlink"] = refs
	heads := mkdir(root, "refs-heads-symlink")
	seed(heads)
	moveTo(heads, filepath.Join("refs", "heads"), filepath.Join(root, "store", "heads-heads"))
	places["refs-heads-symlink"] = heads
	nested := mkdir(root, "refs-nested-symlink")
	seed(nested)
	g.Run(nested, "branch", "feat/x", "HEAD~1")
	g.Run(nested, "branch", "feat/y/z", "HEAD")
	moveTo(nested, filepath.Join("refs", "heads", "feat"), filepath.Join(root, "store", "feat-dir"))
	places["refs-nested-dir-symlink"] = nested
	refFile := mkdir(root, "ref-file-symlink")
	seed(refFile)
	moveTo(refFile, filepath.Join("refs", "heads", "feature"), filepath.Join(root, "store", "feature-ref"))
	places["ref-file-symlink"] = refFile
	packed := mkdir(root, "packed-refs-symlink")
	seed(packed)
	g.Run(packed, "pack-refs", "--all", "--prune")
	moveTo(packed, "packed-refs", filepath.Join(root, "store", "packed-refs-file"))
	places["packed-refs-symlink"] = packed
	cfg := mkdir(root, "config-symlink")
	seed(cfg)
	moveTo(cfg, "config", filepath.Join(root, "store", "config-file"))
	places["config-symlink"] = cfg
	head := mkdir(root, "head-symlink")
	seed(head)
	moveTo(head, "HEAD", filepath.Join(root, "store", "head-file"))
	places["head-symlink"] = head
	// a shared clone borrows objects from another repository's store: the objects live only
	// there, so a read that skips the alternates fails; one clone names the store through a
	// symlinked parent (go-git needs the alternate's basename to stay "objects")
	shared := mkdir(root, "alternates-source")
	seed(shared)
	g.Run(root, "clone", "-q", "--shared", shared, filepath.Join(root, "alternates-clone"))
	places["alternates-clone"] = filepath.Join(root, "alternates-clone")
	g.Run(root, "clone", "-q", "--shared", shared, filepath.Join(root, "alternates-clone-via-symlink"))
	link(filepath.Join(shared, ".git"), filepath.Join(root, "alternates-gitdir-link"))
	write(filepath.Join(root, "alternates-clone-via-symlink", ".git", "objects", "info", "alternates"), filepath.Join(root, "alternates-gitdir-link", "objects")+"\n")
	places["alternates-clone-via-symlink"] = filepath.Join(root, "alternates-clone-via-symlink")

	// a linked worktree whose commondir file names the common directory through a symlink
	cm := mkdir(root, "commondir-main")
	seed(cm)
	g.Run(cm, "worktree", "add", "-q", "-b", "wtc", filepath.Join(root, "commondir-wt"))
	link(filepath.Join(cm, ".git"), filepath.Join(root, "commondir-link"))
	write(filepath.Join(cm, ".git", "worktrees", "commondir-wt", "commondir"), filepath.Join(root, "commondir-link")+"\n")
	places["commondir-via-symlink"] = filepath.Join(root, "commondir-wt")

	// .git symlinked to a linked worktree's admin directory: its relative commondir applies to the
	// real directory. Nested in another repository, a wrong layout would answer for the parent.
	am2 := mkdir(root, "admin-link-main")
	seed(am2)
	g.Run(am2, "worktree", "add", "-q", "-b", "wtl", filepath.Join(root, "admin-link-wt"))
	adminReal := filepath.Join(am2, ".git", "worktrees", "admin-link-wt")
	link(adminReal, filepath.Join(mkdir(root, "dotgit-is-admin-dir"), ".git"))
	places["dotgit-is-admin-dir"] = filepath.Join(root, "dotgit-is-admin-dir")
	link(adminReal, filepath.Join(mkdir(am2, "nested"), ".git"))
	places["nested-dotgit-is-admin-dir"] = filepath.Join(am2, "nested")
	link(filepath.Join(am2, ".git", "worktrees"), filepath.Join(root, "admin-hop"))
	link(filepath.Join(root, "admin-hop", "admin-link-wt"), filepath.Join(mkdir(root, "dotgit-is-admin-dir-hop"), ".git"))
	places["dotgit-is-admin-dir-hop"] = filepath.Join(root, "dotgit-is-admin-dir-hop")

	// macOS reaches the temp directory as /var/... though it lives at /private/var/... (this
	// place only exists there; the symlinked-state-dir fixture covers a symlinked parent everywhere)
	if alias := strings.TrimPrefix(root, "/private"); alias != root {
		if real, err := filepath.EvalSymlinks(alias); err == nil && real == root {
			vm := mkdir(root, "var-alias")
			seed(vm)
			places["var-alias"] = filepath.Join(alias, "var-alias")
		}
	}

	// the service's own layouts: repos/<name>, workspaces/<hash>/worktrees/<session>
	svcRepo := mkdir(root, "svc", ".stapler-squad", "repos", "proj")
	seed(svcRepo)
	places["svc-repo"] = svcRepo
	svcWT := filepath.Join(root, "svc", ".stapler-squad", "workspaces", "abc123", "worktrees", "sess-1")
	g.Run(svcRepo, "worktree", "add", "-q", "-b", "sess-1", svcWT)
	places["svc-worktree"] = svcWT
	places["svc-worktree-sub"] = mkdir(svcWT, "pkg", "x")

	// awkward path characters
	for name, dirname := range map[string]string{"spaces": "with space", "unicode": "ünï-cödé 日本", "leading-dash": "-dash"} {
		d := mkdir(root, "chars", dirname)
		seed(d)
		places["chars-"+name] = d
		g.Run(d, "worktree", "add", "-q", "-b", "wt-"+name, filepath.Join(root, "chars", dirname+" wt"))
		places["chars-"+name+"-linked"] = filepath.Join(root, "chars", dirname+" wt")
		// the same through a symlinked .git
		s := mkdir(root, "chars-sym", dirname)
		seed(s)
		link(moveGitDir(s, filepath.Join(root, "store", "chars "+name+".git")), filepath.Join(s, ".git"))
		places["chars-"+name+"-dotgit-symlink"] = s
	}
	return places
}}

// fixtureSymlinkedStateDir: the service's state directory is reached through a symlink (a
// dotfiles-managed ~/.stapler-squad). Only the symlinked spellings are places, so the oracle's
// path normalizer cannot alias two places that share one real path.
var fixtureSymlinkedStateDir = gitoracle.Fixture{Name: "symlinked-state-dir", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	state := filepath.Join(root, "dotfiles-state")
	repo := filepath.Join(state, "repos", "proj")
	wt := filepath.Join(state, "workspaces", "abc123", "worktrees", "sess-1")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, repo)
	commitFile(g, repo, "a.txt", "1\n")
	commitFile(g, repo, "b.txt", "2\n")
	g.Run(repo, "branch", "feature", "HEAD~1")
	g.Run(repo, "worktree", "add", "-q", "-b", "sess-1", wt)
	if err := os.MkdirAll(filepath.Join(wt, "sub"), 0o700); err != nil {
		g.T.Fatal(err)
	}
	if err := os.Symlink(state, filepath.Join(root, "ssq-link")); err != nil {
		g.T.Fatal(err)
	}
	via := func(p string) string { return filepath.Join(root, "ssq-link", strings.TrimPrefix(p, state)) }
	return gitoracle.Places{"main": via(wt), "repo": via(repo), "worktree-sub": via(filepath.Join(wt, "sub"))}
}}

// Every symlinked layout must be answered exactly as git answers it. A fallback is a failure
// here: a fallback where git succeeds is what the shadow soak records as a real mismatch.
func TestParitySymlinkedLayouts(t *testing.T) {
	// Each call spawns git, and there are ~50 places, so the op list is the smallest set that
	// still touches every storage reader: refs, objects (via merge-base), layout and config.
	ops := append(simpleOps(), resolveOps([]string{"HEAD", "HEAD~1", "main", "v1", "nonexistent", "full sha of HEAD"})...)
	ops = append(ops, listRefOps(true)[:3]...)
	ops = append(ops, mergeBaseOps("symlinked-layouts")[:4:4]...)
	ops = append(ops, getConfigOp("user.name"), getConfigOp("core.bare"))
	for _, fx := range []gitoracle.Fixture{fixtureSymlinkedLayouts, fixtureSymlinkedStateDir} {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			var unexpected []string
			for _, d := range gitoracle.RunOps(t, newCandidate(t), ops, []gitoracle.Fixture{fx}) {
				if d.Kind == gitoracle.DiffError && strings.Contains(d.Candidate, "fallback eligible") && symlinkFallbackExcused(d) {
					continue
				}
				unexpected = append(unexpected, oneLine(d))
			}
			sort.Strings(unexpected)
			for _, u := range unexpected {
				t.Error(u)
			}
		})
	}
}

// symlinkFallbackExcused lists the shapes the backend deliberately leaves to git (it cannot
// model them), each on the operations that touch them.
func symlinkFallbackExcused(d gitoracle.Diff) bool {
	switch d.Place {
	case "head-symlink":
		return true // git resolves a symlinked HEAD as a legacy symref; every read starts at HEAD
	case "ref-file-symlink":
		return strings.HasPrefix(d.Op, "ListRefs") || strings.Contains(d.Op, "feature")
	case "alternates-clone", "alternates-clone-via-symlink":
		// ListRefs checks each ref's object with HasEncodedObject, which ignores alternates
		return strings.HasPrefix(d.Op, "ListRefs")
	}
	if m := revOpName.FindStringSubmatch(d.Op); m != nil {
		return unmodelledRevisions[m[2]] || absentFullSHAs[m[2]]
	}
	if m := mergeBaseName.FindStringSubmatch(d.Op); m != nil {
		return unmodelledRevisions[m[1]] || unmodelledRevisions[m[2]] || m[1] == "HEAD:a.txt" || m[2] == "HEAD:a.txt"
	}
	return false
}

func getConfigOp(key string) gitoracle.Op {
	k := backend.ConfigKey(key)
	return op("GetConfig("+key+")", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
		return b.GetConfig(ctx, l, k)
	})
}
