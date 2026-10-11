//go:build gitoracle

package gitoracle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

// failureCapture stands in for *testing.T so a test can assert that an Assert* helper fails.
// Fatalf stops the helper the way a real one does.
type failureCapture struct {
	testing.TB
	real   *testing.T
	failed bool
	msg    string
}

func (f *failureCapture) Helper()                        {}
func (f *failureCapture) TempDir() string                { return f.real.TempDir() }
func (f *failureCapture) Skip(...any)                    { f.real.Skip("git not installed") }
func (f *failureCapture) Errorf(format string, a ...any) { f.record(format, a) }
func (f *failureCapture) Fatalf(format string, a ...any) { f.record(format, a); runtime.Goexit() }
func (f *failureCapture) record(format string, a []any) {
	f.failed = true
	f.msg = strings.TrimSpace(strings.ReplaceAll(format, "%", "") + " " + sprint(a))
}

func sprint(a []any) string {
	var parts []string
	for _, v := range a {
		switch x := v.(type) {
		case string:
			parts = append(parts, x)
		case error:
			parts = append(parts, x.Error())
		}
	}
	return strings.Join(parts, " ")
}

func capture(t *testing.T, fn func(testing.TB)) *failureCapture {
	t.Helper()
	fc := &failureCapture{real: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(fc)
	}()
	<-done
	return fc
}

// wrongBackend is a candidate that is deliberately wrong in six distinct ways and correct
// everywhere else, so the harness is proven to detect each class of difference and to stay
// quiet where the candidate agrees with git.
type wrongBackend struct{ backend.Backend }

func (w wrongBackend) CurrentBranch(context.Context, backend.RepoLocation) (backend.BranchName, error) {
	return "wrong-branch", nil // result differs; also swallows the detached-HEAD error
}

func (w wrongBackend) HeadRef(ctx context.Context, loc backend.RepoLocation) (backend.RefName, error) {
	ref, err := w.Backend.HeadRef(ctx, loc)
	if err != nil {
		return "refs/heads/main", nil // error class differs on a detached HEAD
	}
	return ref, nil
}

func (w wrongBackend) ResolveRef(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	sha, err := w.Backend.ResolveRef(ctx, loc, ref)
	if err != nil || sha == "" {
		return sha, err
	}
	flipped := []byte(sha)
	if flipped[len(flipped)-1] == '0' {
		flipped[len(flipped)-1] = '1'
	} else {
		flipped[len(flipped)-1] = '0'
	}
	return backend.CommitSHA(flipped), nil // off by one hex digit
}

func (w wrongBackend) RefExists(ctx context.Context, loc backend.RepoLocation, ref backend.RefName) (bool, error) {
	ok, err := w.Backend.RefExists(ctx, loc, ref)
	return !ok, err
}

func (w wrongBackend) ListWorktrees(ctx context.Context, loc backend.RepoLocation) ([]backend.WorktreeInfo, error) {
	ws, err := w.Backend.ListWorktrees(ctx, loc)
	if len(ws) > 0 {
		ws = ws[:len(ws)-1] // drops the last worktree
	}
	return ws, err
}

func (w wrongBackend) CreateBranch(context.Context, backend.RepoLocation, backend.CreateBranchRequest) error {
	return nil // claims success without writing: only the state comparison can see it
}

func newCandidate(t *testing.T) backend.Backend { return cli.New(Runner{Home: t.TempDir()}) }

func TestRunBothFindsNoDifferenceForTheCLIItself(t *testing.T) {
	RequireGit(t)
	t.Parallel()
	AssertParity(t, newCandidate(t), ReadOps(), StandardFixtures())
	// One write op over two fixtures keeps the fresh-repository-per-side cost small.
	AssertParity(t, newCandidate(t), []Op{CreateBranchOp()}, []Fixture{fixtureLinear, fixtureLinkedWorktrees})
}

func TestRunBothDetectsADeliberatelyWrongCandidate(t *testing.T) {
	RequireGit(t)
	t.Parallel()
	// The overridden operations plus two the candidate gets right (the precision controls).
	var ops []Op
	for _, op := range ReadOps() {
		switch op.Name {
		case "CurrentBranch", "HeadRef", "ResolveRef(HEAD)", "RefExists(main)", "ListWorktrees", "RepoRoot", "Status":
			ops = append(ops, op)
		}
	}
	cand := wrongBackend{newCandidate(t)}
	kinds := map[string]map[DiffKind]bool{}
	record := func(diffs []Diff) {
		for _, d := range diffs {
			if kinds[d.Op] == nil {
				kinds[d.Op] = map[DiffKind]bool{}
			}
			kinds[d.Op][d.Kind] = true
		}
	}
	record(RunOps(t, cand, ops, []Fixture{fixtureDetached, fixtureLinkedWorktrees}))
	record(RunOps(t, cand, []Op{CreateBranchOp()}, []Fixture{fixtureLinear}))

	want := map[string]DiffKind{
		"CurrentBranch":    DiffResult,
		"HeadRef":          DiffError,
		"ResolveRef(HEAD)": DiffResult,
		"RefExists(main)":  DiffResult,
		"ListWorktrees":    DiffResult,
		"CreateBranch":     DiffState,
	}
	for op, kind := range want {
		if !kinds[op][kind] {
			t.Errorf("%s: the %q difference was not detected (found %v)", op, kind, kinds[op])
		}
	}
	var unexpected []string
	for op := range kinds {
		if _, ok := want[op]; !ok {
			unexpected = append(unexpected, op)
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Errorf("differences reported for operations the candidate gets right: %v", unexpected)
	}
}

func TestRunBothReportsAreReadable(t *testing.T) {
	RequireGit(t)
	diffs := RunBoth(t, wrongBackend{newCandidate(t)}, ReadOps()[0], []Fixture{fixtureLinear})
	if len(diffs) != 1 {
		t.Fatalf("diffs = %v", diffs)
	}
	d := diffs[0]
	if d.Fixture != "linear" || d.Place != "main" || d.Op != "CurrentBranch" || d.Kind != DiffResult ||
		!strings.Contains(d.Oracle, "main") || !strings.Contains(d.Candidate, "wrong-branch") {
		t.Fatalf("diff = %+v", d)
	}
}

// A mutating op runs on two separate repositories, so a path in its result differs textually
// between the sides; a clean comparison proves the normalizer replaced it.
func TestNormalizerHidesTempPathsInMutatingResults(t *testing.T) {
	RequireGit(t)
	op := Op{Name: "RepoRootAfterWrite", Mutating: true, Places: []string{"main"},
		Run: func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			if err := b.CreateBranch(ctx, l, backend.CreateBranchRequest{Name: "n", Base: "HEAD"}); err != nil {
				return nil, err
			}
			return b.RepoRoot(ctx, l)
		}}
	AssertParity(t, newCandidate(t), []Op{op}, []Fixture{fixtureLinear})
}

func TestRenderTreatsNilAndEmptySlicesAlike(t *testing.T) {
	n := newNormalizer(Places{"main": "/x"})
	type res struct{ Files []string }
	if a, b := n.render(res{}), n.render(res{Files: []string{}}); a != b {
		t.Fatalf("nil %q != empty %q", a, b)
	}
	if n.render([]string(nil)) != n.render([]string{}) {
		t.Fatal("top-level nil slice differs from empty")
	}
}

// corruptingBackend writes through the real CLI and then plants an object git rejects, so the
// fsck comparison inside RunBoth is the only thing that can notice the corruption.
type corruptingBackend struct {
	backend.Backend
	g Git
}

func (c corruptingBackend) CreateBranch(ctx context.Context, loc backend.RepoLocation, req backend.CreateBranchRequest) error {
	if err := c.Backend.CreateBranch(ctx, loc, req); err != nil {
		return err
	}
	dir := string(loc.(backend.Local).Root)
	bad := filepath.Join(c.g.Home, "bad-tree")
	c.g.Write(c.g.Home, "bad-tree", "100644 \x00"+strings.Repeat("\x01", 20))
	sha := c.g.Run(dir, "hash-object", "-w", "--literally", "-t", "tree", bad)
	c.g.Run(dir, "update-ref", "refs/tags/bad", sha)
	return nil
}

func TestRunBothReportsFsckFailureOfTheCandidateRepository(t *testing.T) {
	RequireGit(t)
	cand := corruptingBackend{newCandidate(t), Git{T: t, Home: t.TempDir()}}
	var kinds []DiffKind
	for _, d := range RunBoth(t, cand, CreateBranchOp(), []Fixture{fixtureLinear}) {
		kinds = append(kinds, d.Kind)
	}
	if !slices.Contains(kinds, DiffFsck) {
		t.Fatalf("kinds = %v, want a DiffFsck", kinds)
	}
}

func TestErrorClass(t *testing.T) {
	cases := map[string]error{
		"":                   nil,
		"ErrUnborn":          &backend.CommandError{Err: backend.ErrUnborn},
		"ErrLocked":          backend.ErrLocked{Path: "x.lock"},
		"CommandError":       &backend.CommandError{Operation: backend.OpFetch},
		"other":              os.ErrNotExist,
		"ErrRefNotFound":     backend.ErrRefNotFound,
		"ErrInvalidArgument": backend.ErrInvalidArgument,
	}
	for want, err := range cases {
		if got := ErrorClass(err); got != want {
			t.Errorf("ErrorClass(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestAssertGitFsckCleanPassesAndFails(t *testing.T) {
	RequireGit(t)
	g := Git{T: t, Home: t.TempDir()}
	root := t.TempDir()
	initRepo(g, root)
	g.Write(root, "a.txt", "one\n")
	g.Commit(root, "first")
	AssertGitFsckClean(t, root)

	// A tag pointing at a tree that git rejects under --strict (an entry with an empty name).
	badTree := filepath.Join(root, "bad-tree")
	g.Write(root, "bad-tree", "100644 \x00"+strings.Repeat("\x01", 20))
	sha := g.Run(root, "hash-object", "-w", "--literally", "-t", "tree", badTree)
	g.Run(root, "update-ref", "refs/tags/bad", sha)
	if fc := capture(t, func(tb testing.TB) { AssertGitFsckClean(tb, root) }); !fc.failed {
		t.Fatal("AssertGitFsckClean accepted a repository git fsck --strict rejects")
	}
}

func TestAssertWorktreeListMatches(t *testing.T) {
	RequireGit(t)
	g := Git{T: t, Home: t.TempDir()}
	places := fixtureLinkedWorktrees.Build(g, mustReal(t, t.TempDir()))
	AssertWorktreeListMatches(t, places["main"], newCandidate(t))
	if fc := capture(t, func(tb testing.TB) { AssertWorktreeListMatches(tb, places["main"], wrongBackend{newCandidate(t)}) }); !fc.failed {
		t.Fatal("AssertWorktreeListMatches accepted a candidate that drops a worktree")
	}
}

func TestParseWorktreeList(t *testing.T) {
	got := ParseWorktreeList("worktree /r\nHEAD aaa\nbranch refs/heads/main\n\nworktree /r/wt\nHEAD bbb\ndetached\nlocked why\nprunable gone\n\nworktree /bare\nbare\n")
	want := []backend.WorktreeInfo{
		{Path: "/r", HeadSHA: "aaa", Branch: "refs/heads/main"},
		{Path: "/r/wt", HeadSHA: "bbb", Detached: true, Locked: true, Prunable: true},
		{Path: "/bare", Bare: true},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
