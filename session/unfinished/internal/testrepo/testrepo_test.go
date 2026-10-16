package testrepo_test

import (
	"bufio"
	"fmt"
	"go/build"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

func TestTestrepoAge_ShouldChangeMtimeNotContent_WhenBackdated(t *testing.T) {
	dir := testrepo.TempDir(t)
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(file)

	testrepo.Age(t, file, time.Hour)
	testrepo.Age(t, dir, 2*time.Hour)

	after, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Since(after.ModTime()); got < 59*time.Minute || got > 61*time.Minute {
		t.Errorf("file mtime age = %v, want about 1h", got)
	}
	if !after.ModTime().Before(before.ModTime()) {
		t.Errorf("mtime did not move back: before %v after %v", before.ModTime(), after.ModTime())
	}
	if content, _ := os.ReadFile(file); string(content) != "payload" || after.Size() != before.Size() {
		t.Errorf("content changed: %q size %d->%d", content, before.Size(), after.Size())
	}
	dirInfo, _ := os.Stat(dir)
	if got := time.Since(dirInfo.ModTime()); got < 119*time.Minute || got > 121*time.Minute {
		t.Errorf("dir mtime age = %v, want about 2h", got)
	}
}

func TestNewLinkedWorktree_ShouldProduceIndexedWorktree_WhenBuiltWithGoGitOnly(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 2)
	testrepo.Commit(t, repo, map[string]string{"pkg/a.go": "package pkg\n"}, "add pkg", testrepo.Epoch.Add(time.Hour))

	linked := testrepo.NewLinkedWorktree(t, dir, "wt1", "feature")

	admin := filepath.Join(dir, ".git", "worktrees", "wt1")
	for _, name := range []string{"HEAD", "commondir", "gitdir", "index"} {
		if _, err := os.Stat(filepath.Join(admin, name)); err != nil {
			t.Errorf("missing %s in linked admin dir: %v", name, err)
		}
	}
	pointer, _ := os.ReadFile(filepath.Join(linked, ".git"))
	if want := "gitdir: " + admin + "\n"; string(pointer) != want {
		t.Errorf(".git pointer = %q, want %q", pointer, want)
	}
	for _, f := range []string{"linear.txt", "pkg/a.go"} {
		if _, err := os.Stat(filepath.Join(linked, filepath.FromSlash(f))); err != nil {
			t.Errorf("file %s not populated in linked worktree: %v", f, err)
		}
	}

	lrepo := testrepo.OpenLinked(t, linked)
	head, err := lrepo.Head()
	if err != nil {
		t.Fatalf("linked Head: %v", err)
	}
	if head.Name().Short() != "feature" {
		t.Errorf("linked HEAD = %s, want feature", head.Name())
	}
	mainHead, _ := repo.Head()
	if head.Hash() != mainHead.Hash() || mainHead.Name().Short() != testrepo.DefaultBranch {
		t.Errorf("linked head %s, main head %s %s", head.Hash(), mainHead.Name(), mainHead.Hash())
	}
	if hashes[0] == head.Hash() {
		t.Fatalf("fixture sanity: branch should not start at the first commit")
	}

	wt, _ := lrepo.Worktree()
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("linked Status: %v", err)
	}
	if !status.IsClean() {
		t.Errorf("linked worktree not clean: %s", status)
	}
	idx, err := lrepo.Storer.Index()
	if err != nil || len(idx.Entries) != 2 {
		t.Errorf("per-worktree index entries = %d (err %v), want 2", len(idx.Entries), err)
	}
}

func TestCrissCross_ShouldHaveTwoMergeBases_WhenDAndEMergeBAndCInOppositeOrder(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	h := testrepo.CrissCross(t, repo)

	d, _ := repo.CommitObject(h.D)
	e, _ := repo.CommitObject(h.E)
	bases, err := d.MergeBase(e)
	if err != nil {
		t.Fatalf("MergeBase: %v", err)
	}
	got := hashStrings(bases)
	want := []string{h.B.String(), h.C.String()}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("merge bases = %v, want %v", got, want)
	}
}

func hashStrings(commits []*object.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.Hash.String())
	}
	sort.Strings(out)
	return out
}

func TestLinear_ShouldBuildChainWithIncreasingTimes_WhenNCommits(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 5)
	if len(hashes) != 5 {
		t.Fatalf("got %d hashes", len(hashes))
	}
	for i, h := range hashes {
		c, err := repo.CommitObject(h)
		if err != nil {
			t.Fatal(err)
		}
		wantParents := 1
		if i == 0 {
			wantParents = 0
		}
		if c.NumParents() != wantParents {
			t.Errorf("commit %d parents = %d, want %d", i, c.NumParents(), wantParents)
		}
		if want := testrepo.Epoch.Add(time.Duration(i) * time.Minute); !c.Committer.When.Equal(want) {
			t.Errorf("commit %d time = %v, want %v", i, c.Committer.When, want)
		}
	}
}

func TestBranchy_ShouldBuildMergeWithTwoParents_WhenFeatureMergedIntoMain(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	h := testrepo.Branchy(t, repo)

	merge, err := repo.CommitObject(h.Merge)
	if err != nil {
		t.Fatal(err)
	}
	if want := []plumbing.Hash{h.MainTip, h.FeatureTip}; len(merge.ParentHashes) != 2 ||
		merge.ParentHashes[0] != want[0] || merge.ParentHashes[1] != want[1] {
		t.Errorf("merge parents = %v, want %v", merge.ParentHashes, want)
	}
	head, _ := repo.Head()
	if head.Name().Short() != testrepo.DefaultBranch || head.Hash() != h.Merge {
		t.Errorf("HEAD = %s %s, want main at merge", head.Name(), head.Hash())
	}
	feature, _ := repo.CommitObject(h.FeatureTip)
	base, _ := repo.CommitObject(h.Base)
	if ok, _ := base.IsAncestor(feature); !ok {
		t.Errorf("feature tip should descend from base")
	}
	if wt, _ := repo.Worktree(); true {
		if st, _ := wt.Status(); !st.IsClean() {
			t.Errorf("worktree not clean after Branchy: %s", st)
		}
	}
}

func TestShallow_ShouldWriteShallowFile_WhenBoundarySet(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 3)
	testrepo.Shallow(t, repo, hashes[1])

	fh, err := os.Open(filepath.Join(dir, ".git", "shallow"))
	if err != nil {
		t.Fatalf("shallow file: %v", err)
	}
	defer fh.Close()
	var lines []string
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 1 || lines[0] != hashes[1].String() {
		t.Errorf("shallow file lines = %v, want [%s]", lines, hashes[1])
	}
}

func TestRepack_ShouldPackObjectsAndKeepThemReadable_WhenLooseObjectsExist(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 4)

	testrepo.Repack(t, repo)

	objects := filepath.Join(dir, ".git", "objects")
	loose, _ := filepath.Glob(filepath.Join(objects, "??", "*"))
	packs, _ := filepath.Glob(filepath.Join(objects, "pack", "*.pack"))
	if len(loose) != 0 || len(packs) != 1 {
		t.Errorf("loose objects = %d, packs = %d, want 0 and 1", len(loose), len(packs))
	}
	reopened, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hashes {
		if _, err := reopened.CommitObject(h); err != nil {
			t.Errorf("commit %s unreadable after repack: %v", h, err)
		}
	}
}

// TestTestrepoDeps_ShouldNotImportExecOrSafeexec_WhenResolvedWithGoBuild walks
// imports in-process (go/build.ImportDir reads directories, it does not run
// `go list`). It follows testrepo and every in-module package it reaches.
// Limitation: go-git itself transitively imports os/exec (plumbing/transport/file)
// and module-cache packages are not resolved here, so external dependencies
// are only checked at the point they are imported by an in-module package.
func TestTestrepoDeps_ShouldNotImportExecOrSafeexec_WhenResolvedWithGoBuild(t *testing.T) {
	root, module := moduleRoot(t)
	forbidden := func(p string) bool {
		return p == "os/exec" || strings.HasSuffix(p, "/executor/safeexec") || strings.HasSuffix(p, "/executor")
	}
	banned := []string{
		module + "/session/unfinished",
		module + "/session/unfinished/gogitstore",
		module + "/session/unfinished/repostate",
	}

	seen := map[string]bool{}
	var visit func(pkgPath string)
	visit = func(pkgPath string) {
		if seen[pkgPath] {
			return
		}
		seen[pkgPath] = true
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkgPath, module+"/")))
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			t.Errorf("ImportDir %s: %v", pkgPath, err)
			return
		}
		for _, imp := range pkg.Imports {
			if forbidden(imp) {
				t.Errorf("%s imports %s", pkgPath, imp)
			}
			for _, b := range banned {
				if imp == b {
					t.Errorf("%s imports %s (import cycle risk)", pkgPath, imp)
				}
			}
			if strings.HasPrefix(imp, module+"/") {
				visit(imp)
			}
		}
	}
	visit(module + "/session/unfinished/internal/testrepo")
	if len(seen) == 0 {
		t.Fatal("walk visited nothing")
	}
}

func moduleRoot(t *testing.T) (root, module string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if rest, ok := strings.CutPrefix(line, "module "); ok {
					return dir, strings.TrimSpace(rest)
				}
			}
			t.Fatal("go.mod has no module line")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestRawLinear_ShouldChainCommitsWithoutTouchingRefs_WhenParentGiven(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	tree := testrepo.EmptyTree(t, repo)
	hashes := testrepo.RawLinear(t, repo, tree, plumbing.ZeroHash, 5, "raw")
	if len(hashes) != 5 {
		t.Fatalf("got %d commits, want 5", len(hashes))
	}
	for i := 1; i < len(hashes); i++ {
		c, err := repo.CommitObject(hashes[i])
		if err != nil {
			t.Fatal(err)
		}
		if len(c.ParentHashes) != 1 || c.ParentHashes[0] != hashes[i-1] {
			t.Errorf("commit %d parents = %v, want [%s]", i, c.ParentHashes, hashes[i-1])
		}
	}
	if _, err := repo.Head(); err == nil {
		t.Error("RawLinear must leave HEAD unborn")
	}
}

func TestNewRepo_ShouldBeUnborn_WhenFreshlyInitialised(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	if _, err := repo.Head(); err != plumbing.ErrReferenceNotFound {
		t.Fatalf("Head() err = %v, want ErrReferenceNotFound", err)
	}
}

func TestLinkedWorktrees_ShouldCreateNDistinctWorktrees_WhenMainHasCommit(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a\n"}, "init", testrepo.Epoch)
	dirs := testrepo.LinkedWorktrees(t, dir, 3)
	if len(dirs) != 3 {
		t.Fatalf("got %d worktrees, want 3", len(dirs))
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if seen[d] {
			t.Errorf("duplicate worktree dir %s", d)
		}
		seen[d] = true
		if _, err := os.Stat(filepath.Join(d, "a.txt")); err != nil {
			t.Errorf("worktree %s not populated: %v", d, err)
		}
	}
}

func TestSymlink_ShouldResolveToTarget_WhenCreated(t *testing.T) {
	_, dir := testrepo.NewRepo(t)
	link := testrepo.Symlink(t, dir)
	got, err := filepath.EvalSymlinks(link)
	if err != nil || got != dir {
		t.Fatalf("EvalSymlinks(%s) = %q, %v; want %q", link, got, err, dir)
	}
}

func TestAddSubmodule_ShouldStageGitlinkAndCommitSubmoduleMode_WhenPathGiven(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	first := testrepo.Commit(t, repo, map[string]string{"a.txt": "a\n"}, "init", testrepo.Epoch)
	testrepo.AddSubmodule(t, repo, "vendor/sub", first)

	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	e, err := idx.Entry("vendor/sub")
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != filemode.Submodule || e.Hash != first {
		t.Fatalf("entry = mode %v hash %s, want Submodule/%s", e.Mode, e.Hash, first)
	}
}

func TestSparseCheckout_ShouldSetConfigPatternsAndSkipBit_WhenEnabled(t *testing.T) {
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"keep/a.txt": "a\n", "drop/b.txt": "b\n"}, "init", testrepo.Epoch)
	testrepo.EnableSparseCheckout(t, repo, dir, "keep/")
	testrepo.SkipWorktree(t, repo, "drop/b.txt")

	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Raw.Section("core").Option("sparseCheckout"); got != "true" {
		t.Errorf("core.sparseCheckout = %q, want true", got)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".git", "info", "sparse-checkout"))
	if err != nil || string(body) != "keep/\n" {
		t.Errorf("sparse-checkout = %q, %v", body, err)
	}
	idx, _ := repo.Storer.Index()
	if e, _ := idx.Entry("drop/b.txt"); e == nil || !e.SkipWorktree {
		t.Error("drop/b.txt must carry skip-worktree")
	}
	if e, _ := idx.Entry("keep/a.txt"); e == nil || e.SkipWorktree {
		t.Error("keep/a.txt must not carry skip-worktree")
	}
}

func TestRepack_ShouldSucceedTwice_WhenCalledOnSameHandle(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	testrepo.Linear(t, repo, 5)
	testrepo.Repack(t, repo)
	testrepo.Commit(t, repo, map[string]string{"more.txt": "x\n"}, "after first repack", testrepo.Epoch.Add(time.Hour))
	testrepo.Repack(t, repo)

	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	if err := iter.ForEach(func(*object.Commit) error { n++; return nil }); err != nil {
		t.Fatalf("log after second repack: %v", err)
	}
	if n != 6 {
		t.Errorf("log length = %d, want 6", n)
	}
}

func TestDiverged_ShouldPlaceForkAsMergeBase_WhenAheadAndBehindGiven(t *testing.T) {
	repo, dir, h := testrepo.Diverged(t, 50, 3, 2)
	head, _ := repo.CommitObject(h.Head)
	base, _ := repo.CommitObject(h.Base)
	bases, err := head.MergeBase(base)
	if err != nil || len(bases) != 1 || bases[0].Hash != h.Fork {
		t.Fatalf("MergeBase = %v, %v; want [%s]", bases, err, h.Fork)
	}
	n := 0
	for c := head; c.Hash != h.Fork; n++ {
		c, _ = c.Parents().Next()
	}
	if n != 3 {
		t.Errorf("ahead = %d, want 3", n)
	}
	// The checkout is consistent with main so linked worktrees can be added.
	dirs := testrepo.LinkedWorktrees(t, dir, 2)
	if _, err := os.Stat(filepath.Join(dirs[1], "README.md")); err != nil {
		t.Errorf("linked worktree not populated: %v", err)
	}
}

func TestCommitAll_ShouldStageEveryFileInOneCommit_WhenManyFilesGiven(t *testing.T) {
	repo, _ := testrepo.NewRepo(t)
	files := map[string]string{}
	for i := range 200 {
		files[fmt.Sprintf("d%d/f%d.txt", i%7, i)] = "x\n"
	}
	h := testrepo.CommitAll(t, repo, files, "bulk", testrepo.Epoch)
	c, _ := repo.CommitObject(h)
	tree, _ := c.Tree()
	n := 0
	_ = tree.Files().ForEach(func(*object.File) error { n++; return nil })
	if n != len(files) {
		t.Errorf("committed %d files, want %d", n, len(files))
	}
}
