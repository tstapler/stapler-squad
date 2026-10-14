//go:build gitoracle

package gogit_test

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// refFixtures extends the shared corpus with the shapes that make in-process ref resolution
// diverge from git: ambiguous names, packed refs, odd names, other layouts, rewritten history.
func refFixtures() []gitoracle.Fixture {
	return append(gitoracle.StandardFixtures(),
		fixtureRefsRich, fixturePackedRefs, fixtureAmbiguousHead, fixtureSymlinkedRepo,
		fixtureSubdirs, fixtureShallow, fixtureSubmodule, fixtureBare, fixtureNotARepo,
		fixtureDanglingHead, fixtureUnrelatedHistories, fixtureCrissCross, fixtureDAG,
		fixtureAnnotatedTags, fixtureLocalConfigIncludes, fixtureRemoteHeadDangling, fixtureHeadNames, fixtureConfigContinuation, fixtureSymrefDepth, fixtureMalformed, fixturePackedDeltas,
	)
}

func initMain(g gitoracle.Git, root string) {
	g.T.Helper()
	g.Run(root, "init", "-q", "-b", "main", "--template=")
}

func commitFile(g gitoracle.Git, dir, name, content string) {
	g.T.Helper()
	g.Write(dir, name, content)
	g.Commit(dir, "add "+name)
}

// fixtureRefsRich: branches with odd names, lightweight and annotated tags, remote-tracking refs
// with a symbolic origin/HEAD, and a branch and a tag that share a name.
var fixtureRefsRich = gitoracle.Fixture{Name: "refs-rich", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	commitFile(g, root, "b.txt", "2\n")
	commitFile(g, root, "c.txt", "3\n")
	g.Run(root, "tag", "v1", "HEAD~2")
	g.Run(root, "tag", "-a", "-m", "annotated", "v2", "HEAD~1")
	for _, br := range []string{"feat/ünï-code", "a.b", "x@y", "feat/#1", "dash-ed", "with'quote", "UPPER", "feature/deep/er/name"} {
		g.Run(root, "branch", br, "HEAD~1")
	}
	g.Run(root, "branch", "ambig", "HEAD~1")
	g.Run(root, "tag", "ambig", "HEAD~2") // same name, different commit
	g.Run(root, "update-ref", "refs/remotes/origin/main", "HEAD")
	g.Run(root, "update-ref", "refs/remotes/origin/dev", "HEAD~1")
	g.Run(root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	g.Run(root, "update-ref", "refs/stash", "HEAD~1")
	g.Run(root, "update-ref", "refs/notes/commits", "HEAD~1")
	return gitoracle.Places{"main": root}
}}

// fixturePackedRefs: refs live in packed-refs, a loose ref overrides a packed one, and a packed
// ref's loose twin was deleted.
var fixturePackedRefs = gitoracle.Fixture{Name: "packed-refs", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	commitFile(g, root, "b.txt", "2\n")
	g.Run(root, "branch", "old", "HEAD~1")
	g.Run(root, "tag", "-a", "-m", "m", "rel", "HEAD~1")
	g.Run(root, "tag", "light", "HEAD")
	g.Run(root, "branch", "gone", "HEAD")
	g.Run(root, "pack-refs", "--all", "--prune")
	g.Run(root, "branch", "-f", "old", "HEAD") // loose override of the packed value
	g.Run(root, "branch", "fresh", "HEAD~1")
	return gitoracle.Places{"main": root}
}}

// fixtureAmbiguousHead: HEAD is on a branch whose name is also a tag, so rev-parse --abbrev-ref
// prints the longer "heads/<name>". A second place has a remote-tracking ref of the same name.
var fixtureAmbiguousHead = gitoracle.Fixture{Name: "ambiguous-head", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	g.Run(root, "checkout", "-q", "-b", "same")
	commitFile(g, root, "b.txt", "2\n")
	g.Run(root, "tag", "same")
	g.Run(root, "update-ref", "refs/remotes/same/x", "HEAD")
	g.Run(root, "update-ref", "refs/remotes/main", "HEAD")
	return gitoracle.Places{"main": root}
}}

// fixtureSymlinkedRepo: the repository is reached through a symlink, as /var -> /private/var is.
var fixtureSymlinkedRepo = gitoracle.Fixture{Name: "symlinked", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(real, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, real)
	commitFile(g, real, "a.txt", "1\n")
	g.Run(real, "worktree", "add", "-q", "-b", "wt", filepath.Join(root, "wtreal"))
	if err := os.Symlink(real, link); err != nil {
		g.T.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "wtreal"), filepath.Join(root, "wtlink")); err != nil {
		g.T.Fatal(err)
	}
	return gitoracle.Places{"main": link, "linked": filepath.Join(root, "wtlink"), "inside": filepath.Join(link, ".git")}
}}

var fixtureSubdirs = gitoracle.Fixture{Name: "subdirs", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "deep/er/f.txt", "1\n")
	g.Run(root, "worktree", "add", "-q", "-b", "wt", filepath.Join(root, "wt"))
	commitFile(g, filepath.Join(root, "wt"), "wtdir/x.txt", "x\n")
	return gitoracle.Places{
		"main": root, "sub": filepath.Join(root, "deep", "er"), "linked-sub": filepath.Join(root, "wt", "wtdir"),
		"dotgit": filepath.Join(root, ".git"), "refs-dir": filepath.Join(root, ".git", "refs", "heads"),
		"admin-dir": filepath.Join(root, ".git", "worktrees", "wt"),
	}
}}

var fixtureShallow = gitoracle.Fixture{Name: "shallow", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	origin := filepath.Join(root, "origin")
	if err := os.Mkdir(origin, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, origin)
	for i := 0; i < 4; i++ {
		commitFile(g, origin, fmt.Sprintf("f%d.txt", i), "x\n")
	}
	g.Run(origin, "branch", "side", "HEAD~2")
	clone := filepath.Join(root, "clone")
	g.Run(root, "clone", "-q", "--depth", "2", "--no-single-branch", "file://"+origin, clone)
	g.Run(clone, "branch", "shallow", "HEAD") // also the name of .git/shallow, which git reads first
	return gitoracle.Places{"main": clone}
}}

var fixtureSubmodule = gitoracle.Fixture{Name: "submodule", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	subSrc := filepath.Join(root, "subsrc")
	if err := os.Mkdir(subSrc, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, subSrc)
	commitFile(g, subSrc, "s.txt", "s\n")
	super := filepath.Join(root, "super")
	if err := os.Mkdir(super, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, super)
	commitFile(g, super, "a.txt", "a\n")
	g.Run(super, "-c", "protocol.file.allow=always", "submodule", "add", "-q", subSrc, "sm")
	g.Run(super, "commit", "-q", "-m", "add submodule")
	return gitoracle.Places{"main": super, "submodule": filepath.Join(super, "sm")}
}}

var fixtureBare = gitoracle.Fixture{Name: "bare", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	src := filepath.Join(root, "src")
	if err := os.Mkdir(src, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, src)
	commitFile(g, src, "a.txt", "1\n")
	commitFile(g, src, "b.txt", "2\n")
	bare := filepath.Join(root, "bare.git")
	g.Run(root, "clone", "-q", "--bare", src, bare)
	return gitoracle.Places{"main": bare, "src": src}
}}

var fixtureNotARepo = gitoracle.Fixture{Name: "not-a-repo", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	dir := filepath.Join(root, "plain", "inner")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		g.T.Fatal(err)
	}
	// An empty .git directory is not a repository: git keeps walking up, so does the CLI answer.
	stub := filepath.Join(root, "stub", ".git")
	if err := os.MkdirAll(stub, 0o700); err != nil {
		g.T.Fatal(err)
	}
	return gitoracle.Places{"main": dir, "stub": filepath.Join(root, "stub")}
}}

// fixtureDanglingHead: the branch HEAD names points at a SHA with no object (spike S7 class 2).
var fixtureDanglingHead = gitoracle.Fixture{Name: "dangling-head", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	g.Run(root, "branch", "healthy")
	ref := filepath.Join(root, ".git", "refs", "heads", "main")
	if err := os.WriteFile(ref, []byte(strings.Repeat("1", 40)+"\n"), 0o600); err != nil {
		g.T.Fatal(err)
	}
	return gitoracle.Places{"main": root}
}}

var fixtureUnrelatedHistories = gitoracle.Fixture{Name: "unrelated-histories", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	g.Run(root, "checkout", "-q", "--orphan", "other")
	commitFile(g, root, "o.txt", "o\n")
	g.Run(root, "checkout", "-q", "main")
	return gitoracle.Places{"main": root}
}}

// fixtureCrissCross has two equally good merge bases, where git's choice depends on its ordering.
var fixtureCrissCross = gitoracle.Fixture{Name: "criss-cross", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "base.txt", "b\n")
	g.Run(root, "checkout", "-q", "-b", "left")
	commitFile(g, root, "l.txt", "l\n")
	g.Run(root, "checkout", "-q", "-b", "right", "main")
	commitFile(g, root, "r.txt", "r\n")
	g.Run(root, "checkout", "-q", "left")
	g.Run(root, "merge", "-q", "--no-edit", "-s", "ours", "right")
	g.Run(root, "checkout", "-q", "right")
	g.Run(root, "merge", "-q", "--no-edit", "-s", "ours", "left~1")
	g.Run(root, "checkout", "-q", "-b", "m1", "left")
	commitFile(g, root, "m1.txt", "1\n")
	g.Run(root, "checkout", "-q", "-b", "m2", "right")
	commitFile(g, root, "m2.txt", "2\n")
	return gitoracle.Places{"main": root}
}}

// fixtureDAG builds a seeded random history with merges and uneven commit dates, with a branch
// at every commit (c0..cN), so MergeBase can be compared across every pair.
var fixtureDAG = gitoracle.Fixture{Name: "random-dag", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	tree := g.Run(root, "write-tree")
	rng := rand.New(rand.NewSource(7))
	const n = 22
	shas := make([]string, 0, n)
	for i := 0; i < n; i++ {
		args := []string{"commit-tree", tree, "-m", fmt.Sprintf("c%d", i)}
		if i > 0 {
			picked := map[int]bool{rng.Intn(i): true}
			if i > 2 && rng.Intn(3) == 0 {
				picked[rng.Intn(i)] = true
			}
			for p := range picked {
				args = append(args, "-p", shas[p])
			}
		}
		date := fmt.Sprintf("2020-01-%02dT%02d:00:00Z", 1+rng.Intn(27), rng.Intn(24))
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(gitoracle.Env(g.Home), "GIT_COMMITTER_DATE="+date, "GIT_AUTHOR_DATE="+date)
		out, err := cmd.Output()
		if err != nil {
			g.T.Fatalf("commit-tree: %v", err)
		}
		shas = append(shas, strings.TrimSpace(string(out)))
		g.Run(root, "branch", fmt.Sprintf("c%d", i), shas[i])
	}
	g.Run(root, "update-ref", "refs/heads/main", shas[n-1])
	return gitoracle.Places{"main": root}
}}

var fixtureAnnotatedTags = gitoracle.Fixture{Name: "annotated-tags", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	commitFile(g, root, "b.txt", "2\n")
	g.Run(root, "tag", "-a", "-m", "one", "t1", "HEAD~1")
	g.Run(root, "tag", "-a", "-m", "tag of tag", "t2", "t1")
	tree := g.Run(root, "rev-parse", "HEAD^{tree}")
	g.Run(root, "update-ref", "refs/tags/tree-tag", tree)
	return gitoracle.Places{"main": root}
}}

// fixtureLocalConfigIncludes: repository config with an include, an includeIf that never
// matches, odd value syntax, and keys whose case differs from the lookup.
var fixtureLocalConfigIncludes = gitoracle.Fixture{Name: "config-rich", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	extra := filepath.Join(root, "extra.cfg")
	if err := os.WriteFile(extra, []byte("[inc]\n\tvalue = from-include\n\toverride = include\n"), 0o600); err != nil {
		g.T.Fatal(err)
	}
	cond := filepath.Join(root, "cond.cfg")
	if err := os.WriteFile(cond, []byte("[cond]\n\tsecret = from-conditional\n"), 0o600); err != nil {
		g.T.Fatal(err)
	}
	cfg := "[Core]\n\tFooBar = 1\n" +
		"[inc]\n\toverride = before\n" +
		"[include]\n\tpath = " + extra + "\n" +
		"[inc]\n\toverride = after\n" +
		"[includeIf \"gitdir:/no/such/place/\"]\n\tpath = " + cond + "\n" +
		"[remote \"Origin\"]\n\turl = git@example.com:o/r.git\n\turl = git@example.com:o/last.git\n" +
		"[remote \"origin\"]\n\turl = lower\n" +
		"[val]\n\tquoted = \"a # b\" ; comment\n\ttab = x\\ty\n\tnl = l1\\nl2\n\tempty =\n\tbare\n\tspaced =   padded   \n\tmulti = one\n\tmulti = two\n" +
		"[branch \"feat/x\"]\n\tmerge = refs/heads/feat/x\n"
	appendConfig(g, root, cfg)
	return gitoracle.Places{"main": root}
}}

func appendConfig(g gitoracle.Git, root, text string) {
	g.T.Helper()
	f, err := os.OpenFile(filepath.Join(root, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		g.T.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		g.T.Fatal(err)
	}
}

// fixtureRemoteHeadDangling: refs/remotes/origin/HEAD points at a branch that is gone.
var fixtureRemoteHeadDangling = gitoracle.Fixture{Name: "dangling-symref", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	g.Run(root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/gone")
	g.Run(root, "symbolic-ref", "refs/heads/alias", "refs/heads/main")
	return gitoracle.Places{"main": root}
}}

// fixtureHeadNames gives every case of branch-name ambiguity its own linked worktree (each has
// its own HEAD), so CurrentBranch's `rev-parse --abbrev-ref` shortening is compared per case.
var fixtureHeadNames = gitoracle.Fixture{Name: "head-names", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	main := filepath.Join(root, "main")
	if err := os.Mkdir(main, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, main)
	commitFile(g, main, "a.txt", "1\n")
	for _, br := range []string{"plain", "bt", "rb", "nested/bt", "tags/x", "alias-target", "UPPER", "feat/ünï", "refs/x", "heads/y", "dup-remote"} {
		g.Run(main, "branch", br)
	}
	g.Run(main, "tag", "bt")
	g.Run(main, "tag", "nested/bt")
	g.Run(main, "tag", "x")
	g.Run(main, "update-ref", "refs/remotes/rb", "HEAD")
	g.Run(main, "update-ref", "refs/remotes/dup-remote/HEAD", "HEAD")
	g.Run(main, "update-ref", "refs/tags/packedtag", "HEAD")
	g.Run(main, "branch", "packedtag")
	g.Run(main, "pack-refs", "--all")
	g.Run(main, "symbolic-ref", "refs/heads/alias", "refs/heads/alias-target")
	places := gitoracle.Places{"main": main}
	cases := map[string]string{
		"plain": "refs/heads/plain", "bt": "refs/heads/bt", "rb": "refs/heads/rb", "nested": "refs/heads/nested/bt", "tagsx": "refs/heads/tags/x",
		"alias": "refs/heads/alias", "upper": "refs/heads/UPPER", "unicode": "refs/heads/feat/ünï", "packed": "refs/heads/packedtag",
		"remote": "refs/remotes/rb", "tag": "refs/tags/bt", "unborn": "refs/heads/not-born-yet", "refsx": "refs/heads/refs/x", "headsy": "refs/heads/heads/y",
	}
	for name, target := range cases {
		path := filepath.Join(root, "wt-"+name)
		g.Run(main, "worktree", "add", "-q", "--detach", path)
		g.Run(path, "symbolic-ref", "HEAD", target)
		places[name] = path
	}
	return places
}}

// fixtureConfigContinuation: a repository config using backslash line continuation, which the
// in-process reader leaves to git (gcfg and git disagree), so every call falls back.
var fixtureConfigContinuation = gitoracle.Fixture{Name: "config-continuation", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	commitFile(g, root, "a.txt", "1\n")
	appendConfig(g, root, "[val]\n\tcont = a\\\n b\n")
	return gitoracle.Places{"main": root}
}}

// fixtureSymrefDepth: alias chains around git's depth limit of 5 reads, from refs and from HEAD.
var fixtureSymrefDepth = gitoracle.Fixture{Name: "symref-depth", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	main := filepath.Join(root, "main")
	if err := os.Mkdir(main, 0o700); err != nil {
		g.T.Fatal(err)
	}
	initMain(g, main)
	commitFile(g, main, "a.txt", "1\n")
	g.Run(main, "symbolic-ref", "refs/heads/a1", "refs/heads/main")
	for i := 2; i <= 7; i++ {
		g.Run(main, "symbolic-ref", fmt.Sprintf("refs/heads/a%d", i), fmt.Sprintf("refs/heads/a%d", i-1))
	}
	places := gitoracle.Places{"main": main}
	for _, n := range []int{3, 4, 5, 6} {
		path := filepath.Join(root, fmt.Sprintf("wt-a%d", n))
		g.Run(main, "worktree", "add", "-q", "--detach", path)
		g.Run(path, "symbolic-ref", "HEAD", fmt.Sprintf("refs/heads/a%d", n))
		places[fmt.Sprintf("head-a%d", n)] = path
	}
	return places
}}

// fixtureMalformed: repositories git rejects or reads differently from lenient parsers. Each
// place is its own repository; the in-process backend must answer like git or fall back.
var fixtureMalformed = gitoracle.Fixture{Name: "malformed", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	places := gitoracle.Places{}
	newRepo := func(name string) string {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0o700); err != nil {
			g.T.Fatal(err)
		}
		initMain(g, dir)
		commitFile(g, dir, "a.txt", "1\n")
		commitFile(g, dir, "b.txt", "2\n")
		g.Run(dir, "tag", "v1", "HEAD~1")
		g.Run(dir, "branch", "side", "HEAD~1")
		places[name] = dir
		return dir
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			g.T.Fatal(err)
		}
	}
	sha := func(dir string) string { return g.Run(dir, "rev-parse", "HEAD") }

	newRepo("main")
	for name, content := range map[string]string{"head-empty": "", "head-garbage": "garbage\n", "head-short-sha": "abc1234\n", "head-bare-name": "ref: heads/main\n"} {
		write(filepath.Join(newRepo(name), ".git", "HEAD"), content)
	}
	d := newRepo("head-upper-sha")
	write(filepath.Join(d, ".git", "HEAD"), strings.ToUpper(sha(d))+"\n")
	d = newRepo("head-no-space")
	write(filepath.Join(d, ".git", "HEAD"), "ref:refs/heads/main\n")
	d = newRepo("head-tab")
	write(filepath.Join(d, ".git", "HEAD"), "ref:\trefs/heads/main\n")

	d = newRepo("head-symlink")
	if err := os.Remove(filepath.Join(d, ".git", "HEAD")); err != nil {
		g.T.Fatal(err)
	}
	if err := os.Symlink("refs/heads/main", filepath.Join(d, ".git", "HEAD")); err != nil {
		g.T.Fatal(err)
	}
	d = newRepo("head-oversize")
	write(filepath.Join(d, ".git", "HEAD"), "ref: refs/heads/main\n"+strings.Repeat(" ", 5000))
	d = newRepo("replace-cycle")
	if err := os.MkdirAll(filepath.Join(d, ".git", "refs", "replace"), 0o700); err != nil {
		g.T.Fatal(err)
	}
	write(filepath.Join(d, ".git", "refs", "replace", sha(d)), sha(d)+"\n")
	d = newRepo("dirfile-conflict")
	g.Run(d, "branch", "a/b")
	g.Run(d, "pack-refs", "--all", "--prune")
	if err := os.MkdirAll(filepath.Join(d, ".git", "refs", "heads"), 0o700); err != nil {
		g.T.Fatal(err)
	}
	write(filepath.Join(d, ".git", "refs", "heads", "a"), sha(d)+"\n")
	d = newRepo("branch-named-head")
	write(filepath.Join(d, ".git", "refs", "heads", "head"), sha(d)+"\n")

	outer := newRepo("nested-outer")
	for name, content := range map[string]string{"empty": "", "garbage": "garbage\n"} {
		sub := filepath.Join(outer, "sub-"+name)
		for _, dir := range []string{"objects", "refs"} {
			if err := os.MkdirAll(filepath.Join(sub, ".git", dir), 0o700); err != nil {
				g.T.Fatal(err)
			}
		}
		write(filepath.Join(sub, ".git", "HEAD"), content)
		places["nested-"+name] = sub
	}

	d = newRepo("loose-broken")
	h := sha(d)
	write(filepath.Join(d, ".git", "refs", "heads", "L6"), " "+h+"\n")
	write(filepath.Join(d, ".git", "refs", "heads", "L14"), h+"a\n")
	write(filepath.Join(d, ".git", "refs", "heads", "L26"), "\n"+h+"\n")
	write(filepath.Join(d, ".git", "refs", "heads", "Lempty"), "")
	write(filepath.Join(d, ".git", "refs", "heads", "Lcrlf"), h+"\r\n")
	write(filepath.Join(d, ".git", "refs", "heads", "Lextra"), h+" trailing words\n")

	packed := func(name, mutate string) {
		d := newRepo(name)
		g.Run(d, "pack-refs", "--all", "--prune")
		path := filepath.Join(d, ".git", "packed-refs")
		data, err := os.ReadFile(path)
		if err != nil {
			g.T.Fatal(err)
		}
		text := string(data)
		switch mutate {
		case "stray":
			text = strings.Replace(text, "\n", "\n# stray\n", 2)
		case "no-newline":
			text = strings.TrimSuffix(text, "\n")
		case "crlf":
			text = strings.ReplaceAll(text, "\n", "\r\n")
		case "unsorted":
			lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			lines = append(lines[:1], append([]string{lines[len(lines)-1]}, lines[1:len(lines)-1]...)...)
			text = strings.Join(lines, "\n") + "\n"
		case "duplicate":
			lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			text = strings.Join(append(lines, lines[1]), "\n") + "\n"
		case "bad-name":
			text += sha(d) + " refs/heads/bad..name\n"
		}
		write(path, text)
	}
	for _, m := range []string{"stray", "no-newline", "crlf", "unsorted", "duplicate", "bad-name"} {
		packed("packed-"+m, m)
	}

	d = newRepo("config-padded-header")
	appendConfig(g, d, "[ padded ]\n\tk = v\n")
	d = newRepo("config-padded-subsection")
	appendConfig(g, d, "[a \"b\" ]\n\tk = v\n")
	d = newRepo("extension-in-v0")
	appendConfig(g, d, "[extensions]\n\tobjectformat = sha1\n")
	return places
}}

// fixturePackedDeltas: large, near-identical commit objects in an aggressively repacked history,
// read from the main checkout and a linked worktree. go-git reopens such delta objects' pack by
// name instead of inflating them, which once resolved the pack path against the wrong root.
var fixturePackedDeltas = gitoracle.Fixture{Name: "packed-deltas", Build: func(g gitoracle.Git, root string) gitoracle.Places {
	initMain(g, root)
	body := strings.Repeat("a long, repetitive commit message body that deltifies well\n", 400)
	for i := 0; i < 6; i++ {
		g.Run(root, "commit", "-q", "--allow-empty", "-m", fmt.Sprintf("c%d\n\n%s", i, body))
	}
	g.Run(root, "branch", "side", "HEAD~2")
	g.Run(root, "worktree", "add", "-q", "--detach", filepath.Join(root, "wt"), "HEAD")
	g.Run(root, "gc", "-q", "--aggressive")
	return gitoracle.Places{"main": root, "linked": filepath.Join(root, "wt")}
}}
