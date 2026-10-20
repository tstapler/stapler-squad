package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// newSourceRepo builds a small go-git-only repo (no native git) with tracked
// files, one linked worktree of its own (which BuildCorpus must drop), and
// returns its path.
func newSourceRepo(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"main.go": "package main\n", "README.md": "# " + name + "\n", "pkg/a.go": "package pkg\n"}
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if _, err := wt.Commit("c", &git.CommitOptions{Author: signature(when.Add(time.Duration(i) * time.Hour)), AllowEmptyCommits: true}); err != nil {
			t.Fatal(err)
		}
	}
	// The owner's own linked worktree: its admin files name absolute source paths.
	if err := addLinkedWorktree(dir, "owner-wt", "owner-branch", filepath.Join(parent, name+"-owner-wt")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func buildTestCorpus(t *testing.T, srcs []string, root string, w int) Manifest {
	t.Helper()
	m, err := BuildCorpus(CorpusOptions{Sources: srcs, Root: root, Worktrees: w})
	if err != nil {
		t.Fatalf("BuildCorpus: %v", err)
	}
	return m
}

func TestBuildCorpus_NoCopiedWorktreeFileMentionsSourcePath(t *testing.T) {
	srcParent := t.TempDir()
	src := newSourceRepo(t, srcParent, "alpha")
	root := t.TempDir()
	m := buildTestCorpus(t, []string{src}, root, 3)

	if len(m.Repos) != 1 || len(m.Repos[0].Worktrees) != 3 {
		t.Fatalf("manifest = %+v", m)
	}
	adminRoot := filepath.Join(m.Repos[0].Path, ".git", "worktrees")
	entries, err := os.ReadDir(adminRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("admin dirs = %d, want exactly the 3 corpus worktrees (source's owner-wt dropped)", len(entries))
	}
	for _, e := range entries {
		if e.Name() == "owner-wt" {
			t.Fatal("source worktree admin dir was copied")
		}
	}
	srcReal, _ := filepath.EvalSymlinks(srcParent)
	err = filepath.WalkDir(adminRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, needle := range []string{srcParent, srcReal} {
			if strings.Contains(string(b), needle) {
				t.Errorf("%s mentions source path %s", p, needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildCorpus_SourceUntouchedAndWorktreesOpen(t *testing.T) {
	src := newSourceRepo(t, t.TempDir(), "beta")
	before, err := os.ReadFile(filepath.Join(src, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	m := buildTestCorpus(t, []string{src}, root, 2)

	if _, err := os.Stat(filepath.Join(src, ".git", "worktrees", "owner-wt")); err != nil {
		t.Fatalf("source lost its worktree admin dir: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(src, ".git", "HEAD"))
	if !bytes.Equal(before, after) {
		t.Fatal("source HEAD changed")
	}
	for _, w := range m.Repos[0].Worktrees {
		repo, err := openRepo(w)
		if err != nil {
			t.Fatalf("open %s: %v", w, err)
		}
		if _, err := repo.Head(); err != nil {
			t.Fatalf("head %s: %v", w, err)
		}
		if _, err := os.Stat(filepath.Join(w, "pkg", "a.go")); err != nil {
			t.Fatalf("worktree not populated: %v", err)
		}
	}
}

func TestBuildCorpus_SkipsAbsentOversizedAndAlternates(t *testing.T) {
	parent := t.TempDir()
	big := newSourceRepo(t, parent, "big")
	alt := newSourceRepo(t, parent, "alt")
	info := filepath.Join(alt, ".git", "objects", "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info, "alternates"), []byte("/elsewhere/objects\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var logBuf bytes.Buffer
	m, err := BuildCorpus(CorpusOptions{
		Sources: []string{filepath.Join(parent, "missing"), alt, big},
		Root:    t.TempDir(), Worktrees: 1, MaxRepoMB: 1, Log: &logBuf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Repos) != 1 || m.Repos[0].Name != "big" || len(m.Skipped) != 2 {
		t.Fatalf("repos=%v skipped=%v", m.Repos, m.Skipped)
	}
	if !strings.Contains(logBuf.String(), "alternates") || !strings.Contains(logBuf.String(), "absent") {
		t.Fatalf("skip reasons not logged: %q", logBuf.String())
	}

	// Padding past the threshold gets the repo skipped with a logged reason.
	if err := os.WriteFile(filepath.Join(big, "pad.bin"), make([]byte, 2<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	logBuf.Reset()
	m2, err := BuildCorpus(CorpusOptions{Sources: []string{big}, Root: t.TempDir(), MaxRepoMB: 1, Log: &logBuf})
	if err != nil || len(m2.Repos) != 0 || !strings.Contains(logBuf.String(), "exceeds -max-repo-mb") {
		t.Fatalf("m2=%+v err=%v log=%q", m2, err, logBuf.String())
	}
}

func TestSeedAndVerifyPinnedRepos(t *testing.T) {
	src := newSourceRepo(t, t.TempDir(), "gamma")
	cfgA, cfgB := t.TempDir(), t.TempDir()
	rootA, rootB := filepath.Join(cfgA, "corpus"), filepath.Join(cfgB, "corpus")
	for _, c := range []struct{ cfg, root string }{{cfgA, rootA}, {cfgB, rootB}} {
		m := buildTestCorpus(t, []string{src}, c.root, 1)
		if err := SeedState(c.cfg, m.RepoPaths()); err != nil {
			t.Fatal(err)
		}
	}
	if err := VerifyPinned(cfgA, rootA, cfgB, rootB); err != nil {
		t.Fatalf("VerifyPinned: %v", err)
	}
	// Auto-spider and watch dirs stay off so only the corpus is scanned.
	store, err := PinnedRepos(cfgA)
	if err != nil || len(store) != 1 {
		t.Fatalf("pinned=%v err=%v", store, err)
	}

	// A divergent instance fails verification.
	other := newSourceRepo(t, t.TempDir(), "delta")
	cfgC := t.TempDir()
	rootC := filepath.Join(cfgC, "corpus")
	m := buildTestCorpus(t, []string{other}, rootC, 1)
	if err := SeedState(cfgC, m.RepoPaths()); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPinned(cfgA, rootA, cfgC, rootC); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestResolveConfigDir(t *testing.T) {
	t.Setenv("HOME", "/h")
	for _, c := range []struct{ dir, inst, want string }{
		{"/x", "ignored", "/x"},
		{"", "A", "/h/.stapler-squad/instances/A"},
		{"", "shared", "/h/.stapler-squad"},
	} {
		got, err := ResolveConfigDir(c.dir, c.inst)
		if err != nil || got != c.want {
			t.Errorf("ResolveConfigDir(%q,%q)=%q,%v want %q", c.dir, c.inst, got, err, c.want)
		}
	}
	if _, err := ResolveConfigDir("", ""); err == nil {
		t.Error("expected error with neither flag")
	}
}

func TestMeasureCadence(t *testing.T) {
	src := newSourceRepo(t, t.TempDir(), "eps")
	now := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC) // commits at 00:00, 01:00, 02:00 on Jan 1
	c, err := MeasureCadence(src, 10, now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.TotalCommits != 3 || c.WindowCommits != 3 || c.TrackedFiles != 3 || c.CommitsPerDay != 0.3 {
		t.Fatalf("cadence = %+v", c)
	}
}
