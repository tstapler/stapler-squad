package gitoracle

import (
	"os"
	"path/filepath"
)

// StandardFixtures is the shared repository corpus every cohort's parity test runs over: the
// shapes that have historically made in-process git implementations diverge from the CLI.
func StandardFixtures() []Fixture {
	return []Fixture{
		fixtureUnborn, fixtureLinear, fixtureDirty, fixtureOddFilenames,
		fixtureLinkedWorktrees, fixtureDetached, fixtureDiverged,
	}
}

func initRepo(g Git, root string) string {
	g.T.Helper()
	g.Run(root, "init", "-q", "-b", "main", "--template=")
	return root
}

// An unborn branch: no commits, one untracked file.
var fixtureUnborn = Fixture{Name: "unborn", Build: func(g Git, root string) Places {
	initRepo(g, root)
	g.Write(root, "untracked.txt", "u\n")
	return Places{"main": root}
}}

var fixtureLinear = Fixture{Name: "linear", Build: func(g Git, root string) Places {
	initRepo(g, root)
	g.Write(root, "a.txt", "one\n")
	g.Commit(root, "first")
	g.Write(root, "dir/b.txt", "two\n")
	g.Commit(root, "second")
	g.Run(root, "branch", "feature", "HEAD~1")
	g.Run(root, "tag", "v1")
	return Places{"main": root}
}}

// Every status class at once: staged add, unstaged edit, deleted tracked file, untracked file.
var fixtureDirty = Fixture{Name: "dirty", Build: func(g Git, root string) Places {
	initRepo(g, root)
	g.Write(root, "keep.txt", "k\n")
	g.Write(root, "gone.txt", "g\n")
	g.Write(root, "edit.txt", "e\n")
	g.Commit(root, "base")
	g.Write(root, "edit.txt", "e2\n")
	g.Write(root, "staged.txt", "s\n")
	g.Run(root, "add", "staged.txt")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		g.T.Fatal(err)
	}
	g.Write(root, "untracked.txt", "u\n")
	return Places{"main": root}
}}

// Names git quotes or that look like options: spaces, a leading dash, unicode, a tab and a
// newline, tracked, modified, renamed and untracked.
var fixtureOddFilenames = Fixture{Name: "odd-filenames", Build: func(g Git, root string) Places {
	initRepo(g, root)
	for _, name := range []string{"with space.txt", "-dash.txt", "unicodé 日本.txt", "tab\there.txt", "new\nline.txt", "sub dir/in side.txt"} {
		g.Write(root, name, name+"\n")
	}
	g.Commit(root, "odd names")
	g.Write(root, "with space.txt", "changed\n")
	g.Run(root, "mv", "--", "-dash.txt", "renamed -dash.txt")
	g.Write(root, "untracked über.txt", "u\n")
	g.Write(root, "untracked\nnewline.txt", "u\n")
	return Places{"main": root}
}}

// A main worktree with two linked worktrees (one on a branch, one detached). Operations run
// in all three: git dir and common dir differ in the linked ones.
var fixtureLinkedWorktrees = Fixture{Name: "linked-worktrees", Build: func(g Git, root string) Places {
	main := filepath.Join(root, "main")
	if err := os.Mkdir(main, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initRepo(g, main)
	g.Write(main, "a.txt", "one\n")
	g.Commit(main, "first")
	linked := filepath.Join(root, "linked wt")
	detached := filepath.Join(root, "detached")
	g.Run(main, "worktree", "add", "-q", "-b", "wt-branch", linked)
	g.Run(main, "worktree", "add", "-q", "--detach", detached)
	g.Write(linked, "only-in-linked.txt", "l\n")
	return Places{"main": main, "linked": linked, "detached": detached}
}}

var fixtureDetached = Fixture{Name: "detached-head", Build: func(g Git, root string) Places {
	initRepo(g, root)
	g.Write(root, "a.txt", "one\n")
	g.Commit(root, "first")
	g.Write(root, "a.txt", "two\n")
	g.Commit(root, "second")
	g.Run(root, "checkout", "-q", "--detach", "HEAD~1")
	return Places{"main": root}
}}

// Two branches that diverged from a common base, for merge-base and range queries.
var fixtureDiverged = Fixture{Name: "diverged", Build: func(g Git, root string) Places {
	initRepo(g, root)
	g.Write(root, "base.txt", "b\n")
	g.Commit(root, "base")
	g.Run(root, "checkout", "-q", "-b", "left")
	g.Write(root, "left.txt", "l\n")
	g.Commit(root, "left work")
	g.Run(root, "checkout", "-q", "main")
	g.Write(root, "main.txt", "m\n")
	g.Commit(root, "main work")
	return Places{"main": root}
}}
