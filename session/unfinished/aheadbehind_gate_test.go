package unfinished

// Decode-count gates for the shared commit graph (Story 2.1.2, gates G1, G2,
// G2b, G9, G10). Every shared-path test asserts engagement (flag on and the
// graph's Hits+Misses > 0) and each gate has a flag-off or legacy-wired control.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/tstapler/stapler-squad/session/git/native"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// G10 is hang detection, not a speed gate (validation GA9).
const deadlockDeadline = 30 * time.Second

// assertSharedPathEngaged fails unless the flag is on and the repo's graph
// served or filled at least one row, so a test that forgot to enable the flag
// cannot pass vacuously on the legacy path. It returns the repo's RepoState.
func assertSharedPathEngaged(tb testing.TB, g *GoGitVCSReader, path string) *repostate.RepoState {
	tb.Helper()
	if !g.effectiveFlags().SharedCommitState {
		tb.Fatal("shared_commit_state is off: the test would exercise the legacy path")
	}
	entry, err := g.openRepoEntry(path)
	if err != nil {
		tb.Fatal(err)
	}
	rs := g.repoState(entry)
	if rs == nil {
		tb.Fatal("no RepoState attached to the worktree's storer")
	}
	if st := rs.Graph().CacheStats(); st.Hits+st.Misses == 0 {
		tb.Fatal("graph Hits+Misses == 0: the shared path was not engaged")
	}
	return rs
}

// filledRows counts the rows a walk actually decoded and published.
func filledRows(rs *repostate.RepoState) (n int) {
	gen := rs.Graph().Load()
	for id := range gen.Len() {
		if gen.Row(uint32(id), gen.StampOrd()) == repostate.Filled {
			n++
		}
	}
	return n
}

// oracleDecodes runs a retained walk cold over dir's repository through a
// counting storer and returns its commit-object reads.
func oracleDecodes(tb testing.TB, dir string, walk func(repo *git.Repository)) int64 {
	tb.Helper()
	plain, err := native.OpenRepo(dir)
	if err != nil {
		tb.Fatal(err)
	}
	cs := &countingStorer{Storer: plain.Storer}
	repo, err := git.Open(cs, nil)
	if err != nil {
		tb.Fatal(err)
	}
	walk(repo)
	return cs.DecodeCount()
}

func legacyStackDecodes(tb testing.TB, dir string, head, base plumbing.Hash) int64 {
	return oracleDecodes(tb, dir, func(repo *git.Repository) {
		mb, err := legacyStackFindMergeBase(repo, head, base)
		if err != nil {
			tb.Fatal(err)
		}
		_, _ = legacyStackCountCommitsTo(repo, head, mb)
		_, _ = legacyStackCountCommitsTo(repo, base, mb)
	})
}

func lockstepDecodes(tb testing.TB, dir string, head, base plumbing.Hash) int64 {
	return oracleDecodes(tb, dir, func(repo *git.Repository) {
		_, _, err := aheadBehindFrom(newParentMemo(repo), head, base)
		if err != nil {
			tb.Fatal(err)
		}
	})
}

// assertG1 is gate G1: the new walk decodes no more than the committed-old
// walk and the plain (paint-down) walk.
func assertG1(tb gateTB, newDecodes, legacy, lockstep int64) {
	tb.Helper()
	if newDecodes > legacy || newDecodes > lockstep {
		tb.Errorf("DecodeCount new=%d, committed-old=%d, plain walk=%d; want new <= both", newDecodes, legacy, lockstep)
	}
}

// divergedClocked is testrepo.Diverged with committer times that increase along
// every chain, so the paint-down walk stops near the fork like a real history
// (Diverged restarts its ahead/behind chains at minute 1, which makes the walk
// read the whole trunk, as git would with that clock skew).
func divergedClocked(tb testing.TB, depth, ahead, behind int) (*git.Repository, string, testrepo.DivergedHashes) {
	tb.Helper()
	repo, dir := testrepo.NewRepo(tb)
	var h testrepo.DivergedHashes
	h.Root = testrepo.Commit(tb, repo, map[string]string{"README.md": "hello\n"}, "root", testrepo.Epoch)
	root, err := repo.CommitObject(h.Root)
	if err != nil {
		tb.Fatal(err)
	}
	clock := 0
	chain := func(from plumbing.Hash, n int, tag string) plumbing.Hash {
		for i := range n {
			clock++
			from = testrepo.RawCommit(tb, repo, root.TreeHash, []plumbing.Hash{from}, fmt.Sprintf("%s %d", tag, i), testrepo.Epoch.Add(time.Duration(clock)*time.Minute))
		}
		return from
	}
	h.Fork = chain(h.Root, depth, "trunk")
	h.Head, h.Base = chain(h.Fork, ahead, "ahead"), chain(h.Fork, behind, "behind")
	testrepo.SetBranch(tb, repo, testrepo.DefaultBranch, h.Head)
	testrepo.SetBranch(tb, repo, "base", h.Base)
	return repo, dir, h
}

// clockedWorktrees is divergedWorktrees over divergedClocked.
func clockedWorktrees(tb testing.TB, w int) []string {
	tb.Helper()
	_, dir, _ := divergedClocked(tb, 2500, 3, 2)
	return append([]string{dir}, testrepo.LinkedWorktrees(tb, dir, w-1)...)
}

func TestAheadBehind_ShouldDecodeNoMoreThanLegacyAndLockstep_WhenCold2500LinearW1(t *testing.T) {
	_, dir, h := divergedClocked(t, 2500, 3, 2)
	legacy, lockstep := legacyStackDecodes(t, dir, h.Head, h.Base), lockstepDecodes(t, dir, h.Head, h.Base)

	r := NewCountingReader(Flags{SharedCommitState: true})
	r.Track(t, dir)
	assertRefsBalanced(t, r.GoGitVCSReader)
	ahead, behind, err := r.AheadBehind(context.Background(), dir, "base")
	if err != nil || ahead != 3 || behind != 2 {
		t.Fatalf("AheadBehind = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, dir)
	got := r.DecodeCount()
	t.Logf("G1 cold 2500-commit linear, W=1: DecodeCount shared=%d committed-old=%d plain walk=%d graph rows=%d filled=%d",
		got, legacy, lockstep, rs.Graph().Load().Len(), filledRows(rs))
	assertG1(t, got, legacy, lockstep)

	if n := filledRows(rs); int64(n) != got {
		t.Errorf("filled rows = %d, DecodeCount = %d: every decode must publish exactly one row", n, got)
	}
	if rows := int64(rs.Graph().Load().Len()); rows > got+4 {
		t.Errorf("graph rows = %d, want <= DecodeCount+4 = %d: no ancestor beyond the walk may be ingested", rows, got+4)
	}
}

func TestG1Gate_ShouldFail_WhenLegacyStackWalkIsWiredAsNew(t *testing.T) {
	_, dir, h := divergedClocked(t, 2500, 3, 2)
	legacy, lockstep := legacyStackDecodes(t, dir, h.Head, h.Base), lockstepDecodes(t, dir, h.Head, h.Base)
	rec := &failRecorder{}
	assertG1(rec, legacy, legacy, lockstep)
	if rec.failures == 0 {
		t.Errorf("G1 passed with the legacy walk wired as new (legacy=%d, plain=%d)", legacy, lockstep)
	}
}

func TestAheadBehind_ShouldDecodeOnce_WhenThreeWorktreesShareHeadAndBase(t *testing.T) {
	paths := clockedWorktrees(t, 3)
	repoHead, repoBase := headAndBase(t, paths[0])
	legacyW1 := legacyStackDecodes(t, paths[0], repoHead, repoBase)
	w1 := coldAheadBehindDecodes(t, Flags{SharedCommitState: true}, paths[:1])
	w3 := coldAheadBehindDecodes(t, Flags{SharedCommitState: true}, paths)
	t.Logf("G2 shared head and base: DecodeCount shared W=1=%d W=3=%d, committed-old W=1=%d", w1, w3, legacyW1)
	if w3 > legacyW1 {
		t.Errorf("DecodeCount(new, W=3) = %d, want <= DecodeCount(committed-old, W=1) = %d", w3, legacyW1)
	}
}

// headAndBase resolves a worktree's HEAD and the "base" branch.
func headAndBase(tb testing.TB, dir string) (head, base plumbing.Hash) {
	tb.Helper()
	repo, err := native.OpenRepo(dir)
	if err != nil {
		tb.Fatal(err)
	}
	ref, err := repo.Head()
	if err != nil {
		tb.Fatal(err)
	}
	if base, err = resolveRef(repo, "base"); err != nil {
		tb.Fatal(err)
	}
	return ref.Hash(), base
}

func TestAheadBehind_ShouldDecodeWTimes_WhenSharedCommitStateFlagOff(t *testing.T) {
	paths := clockedWorktrees(t, 3)
	w1 := coldAheadBehindDecodes(t, Flags{SharedCommitState: false}, paths[:1])
	w3 := coldAheadBehindDecodes(t, Flags{SharedCommitState: false}, paths)
	if w3 < 3*w1 {
		t.Errorf("flag off: DecodeCount W=3 = %d, want >= 3 x W=1 = %d (legacy decodes per worktree)", w3, 3*w1)
	}
	r := readerWithFlags(Flags{SharedCommitState: false}, &GoGitVCSReader{})
	if _, _, err := r.AheadBehind(context.Background(), paths[0], "base"); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.repoStateTotals(); n != 0 {
		t.Errorf("flag off allocated %d RepoStates, want none (the extension slot must stay untouched)", n)
	}
}

// distinctTipWorktrees builds three worktrees of one repo with three distinct
// tips (4, 5 and 6 commits ahead of one fork) over shared history, and returns
// the worktree paths with each tip.
func distinctTipWorktrees(t *testing.T, depth int) (paths []string, tips []plumbing.Hash, fork, base plumbing.Hash) {
	t.Helper()
	repo, dir, h := divergedClocked(t, depth, 4, 2)
	root, err := repo.CommitObject(h.Root)
	if err != nil {
		t.Fatal(err)
	}
	paths = append([]string{dir}, testrepo.LinkedWorktrees(t, dir, 2)...)
	tips = make([]plumbing.Hash, 1, 3)
	tips[0] = h.Head
	for i, branch := range []string{"wt-1", "wt-2"} {
		tip := testrepo.RawLinear(t, repo, root.TreeHash, h.Fork, 5+i, "tip"+branch)
		testrepo.SetBranch(t, repo, branch, tip[len(tip)-1])
		tips = append(tips, tip[len(tip)-1])
	}
	return paths, tips, h.Fork, h.Base
}

func TestAheadBehind_ShouldDecodeExactlyDistinct_WhenWalksSequencedByLatches(t *testing.T) {
	paths, tips, _, base := distinctTipWorktrees(t, 2500)
	var oracleSum int64
	for _, tip := range tips {
		oracleSum += legacyStackDecodes(t, paths[0], tip, base)
	}

	r := NewCountingReader(Flags{SharedCommitState: true})
	for _, p := range paths {
		r.Track(t, p)
	}
	assertRefsBalanced(t, r.GoGitVCSReader)
	for _, p := range paths {
		if _, _, err := r.AheadBehind(context.Background(), p, "base"); err != nil {
			t.Fatal(err)
		}
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, paths[0])
	distinct, got := int64(filledRows(rs)), r.DecodeCount()
	st := rs.Graph().Stats()
	t.Logf("G2b sequential: distinct=%d DecodeCount=%d committed-old per-tip sum=%d lostCAS=%d lostDecode=%d reprobe=%d",
		distinct, got, oracleSum, st.FillLostCAS, st.FillLostDecode, st.Reprobe)
	if got != distinct || got > oracleSum {
		t.Errorf("DecodeCount = %d, want == distinct %d and <= oracle sum %d", got, distinct, oracleSum)
	}
	if st.FillLostCAS != 0 || st.FillLostDecode != 0 || st.Reprobe != 0 {
		t.Errorf("sequenced walks lost a claim or re-probed: %+v", st)
	}
}

func TestAheadBehind_ShouldBoundDecodes_WhenThreeDistinctTipsRace(t *testing.T) {
	paths, _, _, _ := distinctTipWorktrees(t, 2500)
	r := NewCountingReader(Flags{SharedCommitState: true})
	for _, p := range paths {
		r.Track(t, p)
	}
	assertRefsBalanced(t, r.GoGitVCSReader)
	jobs := make([]func(), len(paths))
	for i, p := range paths {
		jobs[i] = func() {
			if _, _, err := r.AheadBehind(context.Background(), p, "base"); err != nil {
				t.Errorf("AheadBehind %s: %v", p, err)
			}
		}
	}
	if hung := runWithDeadline(deadlockDeadline, jobs...); hung {
		t.Fatal("racing walks did not finish")
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, paths[0])
	distinct, got := int64(filledRows(rs)), r.DecodeCount()
	st := rs.Graph().Stats()
	t.Logf("G2b racing: distinct=%d DecodeCount=%d lostCAS=%d lostDecode=%d reprobe=%d", distinct, got, st.FillLostCAS, st.FillLostDecode, st.Reprobe)
	if upper := distinct + int64(st.FillLostDecode+st.Reprobe); got < distinct || got > upper {
		t.Errorf("DecodeCount = %d, want within [distinct %d, %d]", got, distinct, upper)
	}
}

// runWithDeadline runs jobs concurrently and reports whether any was still
// running at the deadline. Completion is signalled through a channel, never a sleep.
func runWithDeadline(deadline time.Duration, jobs ...func()) (hung bool) {
	done := make(chan struct{}, len(jobs))
	for _, j := range jobs {
		go func() {
			j()
			done <- struct{}{}
		}()
	}
	timeout := time.After(deadline)
	for range jobs {
		select {
		case <-done:
		case <-timeout:
			return true
		}
	}
	return false
}

func TestAheadBehind_ShouldNotDeadlock_WhenThreeWorktreesAndCommitMessagesRunConcurrentlyUnderRace(t *testing.T) {
	paths := clockedWorktrees(t, 3)
	r := readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{})
	assertRefsBalanced(t, r)
	jobs := make([]func(), 0, 2+len(paths)-1)
	jobs = append(jobs,
		func() {
			if _, _, err := r.AheadBehind(context.Background(), paths[0], "base"); err != nil {
				t.Errorf("AheadBehind: %v", err)
			}
		},
		func() {
			if _, err := r.CommitMessages(context.Background(), paths[0], "base", 20); err != nil {
				t.Errorf("CommitMessages: %v", err)
			}
		},
	)
	for _, p := range paths[1:] {
		jobs = append(jobs, func() {
			if _, _, err := r.AheadBehind(context.Background(), p, "base"); err != nil {
				t.Errorf("AheadBehind %s: %v", p, err)
			}
		})
	}
	start := time.Now()
	if hung := runWithDeadline(deadlockDeadline, jobs...); hung {
		t.Fatalf("calls still running after %s: lock inversion", deadlockDeadline)
	}
	t.Logf("G10: four concurrent calls finished in %s (deadline %s)", time.Since(start).Round(time.Millisecond), deadlockDeadline)
	assertSharedPathEngaged(t, r, paths[0])
}

// lockingSource is the broken adapter of the earlier design: it takes entry.mu
// inside the walk.
type lockingSource struct {
	inner repostate.ParentsSource
	mu    *sync.Mutex
}

func (s lockingSource) Parents(ctx context.Context, h plumbing.Hash) ([]plumbing.Hash, int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Parents(ctx, h)
}

func TestDeadlockHarness_ShouldDetectHang_WhenAdapterTakesEntryMuInsideWalk(t *testing.T) {
	_, dir, h := divergedClocked(t, 40, 3, 2)
	r := readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{})
	assertRefsBalanced(t, r)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	rs := r.repoState(entry)

	// The caller holds entry.mu, as the old design's AheadBehind did across the walk.
	entry.mu.Lock()
	src := lockingSource{inner: newGraphSource(entry.repo.Storer, false), mu: &entry.mu}
	hung := runWithDeadline(200*time.Millisecond, func() {
		_, _, _ = aheadBehindOnGraph(context.Background(), rs, src, 0, h.Head, h.Base)
	})
	entry.mu.Unlock() // releases the leaked goroutine
	if !hung {
		t.Error("harness reported no hang for an adapter that takes entry.mu inside the walk")
	}
}

func TestGraphSource_ShouldTakeNoEntryMu_WhenReadingParents(t *testing.T) {
	_, dir, h := divergedClocked(t, 40, 3, 2)
	r := readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{})
	assertRefsBalanced(t, r)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	src := newGraphSource(entry.repo.Storer, false)
	var parents []plumbing.Hash
	if hung := runWithDeadline(deadlockDeadline, func() { parents, _, _, _ = src.Parents(context.Background(), h.Head) }); hung {
		t.Fatal("Parents blocked while entry.mu was held")
	}
	if len(parents) != 1 {
		t.Errorf("parents of head = %v, want one", parents)
	}
}

// closedStorer fails every read the way a storer closed by pruneRepoCache does.
type closedStorer struct{ storer.EncodedObjectStorer }

var errStoreClosed = errors.New("store closed")

func (closedStorer) EncodedObject(plumbing.ObjectType, plumbing.Hash) (plumbing.EncodedObject, error) {
	return nil, errStoreClosed
}

func TestGraphSource_ShouldTreatClosedStorerAsFallback_WhenStorerClosedMidFill(t *testing.T) {
	_, dir, h := divergedClocked(t, 40, 3, 2)
	r := readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{})
	assertRefsBalanced(t, r)
	entry, err := r.openRepoEntry(dir)
	if err != nil {
		t.Fatal(err)
	}
	rs := r.repoState(entry)
	_, _, abErr := aheadBehindOnGraph(context.Background(), rs, newGraphSource(closedStorer{}, false), 0, h.Head, h.Base)
	if !isFallback(abErr) || !errors.Is(abErr, errStoreClosed) {
		t.Fatalf("err = %v, want a fallback error wrapping the storer failure", abErr)
	}
	if got := filledRows(rs); got != 0 {
		t.Errorf("%d rows published from a failed read, want none (claims must roll back)", got)
	}
	// The same failure through the reader reruns the plain walk, which reads the live repo.
	if ahead, behind, err := r.AheadBehind(context.Background(), dir, "base"); err != nil || ahead != 3 || behind != 2 {
		t.Errorf("AheadBehind after a failed graph walk = (%d, %d, %v), want (3, 2, nil)", ahead, behind, err)
	}
}

func isFallback(err error) bool { return errors.Is(err, repostate.ErrFallback) }

func TestShallowStamp_ShouldStatOncePerColdCallAndBumpOrdinal_WhenShallowFileChanges(t *testing.T) {
	repo, dir, h := divergedClocked(t, 40, 3, 2)
	r := NewCountingReader(Flags{SharedCommitState: true})
	assertRefsBalanced(t, r.GoGitVCSReader)
	if _, _, err := r.AheadBehind(context.Background(), dir, "base"); err != nil {
		t.Fatal(err)
	}
	if got := r.StatCount(); got != 1 {
		t.Errorf("StatCount after one cold shared AheadBehind = %d, want 1 (the shallow stat)", got)
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, dir)
	before := rs.Graph().Load().StampOrd()

	testrepo.Shallow(t, repo, h.Fork) // writes <commondir>/shallow
	r.aheadBehindCache.Range(func(k aheadBehindKey, _ aheadBehindEntry) bool { r.aheadBehindCache.Delete(k); return true })
	if _, _, err := r.AheadBehind(context.Background(), dir, "base"); err != nil {
		t.Fatal(err)
	}
	if after := rs.Graph().Load().StampOrd(); after == before {
		t.Errorf("stamp ordinal stayed %d after the shallow file appeared", before)
	}
}

// herdRepos builds repos x worktrees clocked fixtures; it returns every path.
func herdRepos(tb testing.TB, repos, worktrees int) []string {
	tb.Helper()
	paths := make([]string, 0, repos*worktrees)
	for range repos {
		_, dir, _ := divergedClocked(tb, 2500, 3, 2)
		paths = append(paths, dir)
		paths = append(paths, testrepo.LinkedWorktrees(tb, dir, worktrees-1)...)
	}
	return paths
}

func TestStartupHerd_ShouldDecodeLEOracleTotal_WhenSixReposThreeWorktreesOpenTogether(t *testing.T) {
	paths := herdRepos(t, 6, 3)
	herd := func(r *CountingReader) {
		jobs := make([]func(), len(paths))
		for i, p := range paths {
			jobs[i] = func() {
				if _, _, err := r.AheadBehind(context.Background(), p, "base"); err != nil {
					t.Errorf("AheadBehind %s: %v", p, err)
				}
			}
		}
		if runWithDeadline(deadlockDeadline, jobs...) {
			t.Fatal("startup herd did not finish")
		}
	}
	track := func(r *CountingReader) {
		for _, p := range paths {
			r.Track(t, p)
		}
	}

	legacy := NewCountingReader(Flags{SharedCommitState: false})
	track(legacy)
	herd(legacy)
	legacyTotal := legacy.DecodeCount()
	legacy.ClearCache()

	shared := NewCountingReader(Flags{SharedCommitState: true})
	track(shared)
	assertRefsBalanced(t, shared.GoGitVCSReader)
	herd(shared)
	var distinct, lost int64
	shared.forEachRepoState(func(rs *repostate.RepoState) {
		distinct += int64(filledRows(rs))
		st := rs.Graph().Stats()
		lost += int64(st.FillLostDecode + st.Reprobe)
	})
	assertSharedPathEngaged(t, shared.GoGitVCSReader, paths[0])
	got := shared.DecodeCount()
	t.Logf("G9 6 repos x 3 worktrees: DecodeCount shared=%d legacy=%d distinct=%d lostDecode+reprobe=%d", got, legacyTotal, distinct, lost)
	if got > legacyTotal {
		t.Errorf("shared herd DecodeCount = %d, want <= legacy %d", got, legacyTotal)
	}
	if got < distinct || got > distinct+lost {
		t.Errorf("DecodeCount = %d, want within [distinct %d, %d]", got, distinct, distinct+lost)
	}
}

func TestAheadBehind_ShouldStayWithinG2AndBalanceRefs_When52000CommitsAnd24Worktrees(t *testing.T) {
	_, dir, h := divergedClocked(t, 52000, 3, 2)
	paths := append([]string{dir}, testrepo.LinkedWorktrees(t, dir, 23)...)
	legacyW1 := legacyStackDecodes(t, dir, h.Head, h.Base)

	r := NewCountingReader(Flags{SharedCommitState: true})
	for _, p := range paths {
		r.Track(t, p)
	}
	assertRefsBalanced(t, r.GoGitVCSReader)
	for _, p := range paths {
		if ahead, behind, err := r.AheadBehind(context.Background(), p, "base"); err != nil || ahead != 3 || behind != 2 {
			t.Fatalf("AheadBehind %s = (%d, %d, %v), want (3, 2, nil)", p, ahead, behind, err)
		}
	}
	rs := assertSharedPathEngaged(t, r.GoGitVCSReader, dir)
	got := r.DecodeCount()
	t.Logf("52,000 commits x 24 worktrees: DecodeCount=%d committed-old W=1=%d graph rows=%d", got, legacyW1, rs.Graph().Load().Len())
	if got > legacyW1 {
		t.Errorf("DecodeCount = %d, want <= committed-old W=1 = %d", got, legacyW1)
	}
	if rows := rs.Graph().Load().Len(); rows > rs.Graph().Load().Cap() {
		t.Errorf("graph rows %d exceed MaxGraphCommits %d", rows, rs.Graph().Load().Cap())
	}
}
