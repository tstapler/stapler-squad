package unfinished

// Parity of the integer-id walks (repostate.Walk) with the retained oracles
// (Story 2.1.3): the plain walk that stays in production (findMergeBase and
// countCommitsTo over parentMemo), the committed-old legacyStack* walk through
// the recorded golden divergence set, and object.Commit.MergeBase.

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// idOutcome runs the id walk for one case on rs and returns the comparable
// outcome. shallow selects whether an absent commit is remembered as a shallow
// boundary (truncated) or surfaces as a not-found error on every read.
func idOutcome(t testing.TB, d *builtDAG, rs *repostate.RepoState, shallow bool, head, base plumbing.Hash) walkOutcome {
	t.Helper()
	w := rs.NewWalk(context.Background(), newGraphSource(d.Repo.Storer, shallow), 0, mergeBaseBFSLimit)
	defer w.Close()
	mb, err := w.FindMergeBase(head, base)
	if errors.Is(err, repostate.ErrFallback) {
		t.Fatalf("unexpected fallback: %v", err)
	}
	return outcomeFor(d, mb, err, func(s plumbing.Hash) (int, error) {
		n, cerr := w.CountCommitsTo(s, mb)
		if errors.Is(cerr, repostate.ErrFallback) {
			t.Fatalf("unexpected fallback: %v", cerr)
		}
		return n, cerr
	}, head, base)
}

func newTestRepoState() *repostate.RepoState {
	return repostate.NewRepoState(repostate.Options{}, 1<<30)
}

func TestIDWalk_ShouldEqualLockstep_WhenRandom500DAGsOnAndOffFlag(t *testing.T) {
	cases := oracleCorpus()
	if len(cases) < 500 {
		t.Fatalf("corpus has %d cases, want >= 500", len(cases))
	}
	for _, c := range cases {
		withMergeBaseLimit(t, c.limit)
		d := buildDAG(t, c.spec, nil)
		h, b := d.Hashes[c.head], d.Hashes[c.base]
		want := lockstepOutcome(d, h, b) // flag off: the plain walk
		for _, shallow := range []bool{len(c.spec.Missing) > 0, false} {
			rs := newTestRepoState() // flag on
			cold := idOutcome(t, d, rs, shallow, h, b)
			warm := idOutcome(t, d, rs, shallow, h, b)
			if cold != want || warm != want {
				t.Errorf("%s shallow=%v: id walk cold=%v warm=%v, plain walk %v", c.name, shallow, cold, warm, want)
			}
		}
		if len(c.spec.Missing) == 0 && c.limit == mergeBaseBFSLimitDefault {
			assertSameOracleVerdict(t, c, d, h, b, want, idOutcome(t, d, newTestRepoState(), false, h, b))
		}
	}
}

const mergeBaseBFSLimitDefault = 2000

// assertSameOracleVerdict is the second check: against the object.Commit.MergeBase
// best set the id walk classifies exactly as the plain walk, never a non-ancestor.
func assertSameOracleVerdict(t *testing.T, c divergenceCase, d *builtDAG, h, b plumbing.Hash, plain, id walkOutcome) {
	t.Helper()
	best, err := d.bestSet(h, b)
	if err != nil {
		return
	}
	common := c.spec.commonAncestors(c.head, c.base)
	vp, vi := oracleVerdict(plain.MB, best, common), oracleVerdict(id.MB, best, common)
	if vi == "NOT-ANCESTOR" || vi != vp {
		t.Errorf("%s: MergeBase oracle verdict id=%s plain=%s", c.name, vi, vp)
	}
}

func goldenDivergenceLines(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(mergeBaseGoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l != "" && !strings.HasPrefix(l, "#") {
			lines = append(lines, l)
		}
	}
	return lines
}

func TestIDWalk_ShouldAddNoDivergenceBeyondGolden_WhenComparedToLegacyStack(t *testing.T) {
	var got []string
	for _, c := range append(namedDivergenceCases(), seededDivergenceCases()...) {
		withMergeBaseLimit(t, c.limit)
		d := buildDAG(t, c.spec, nil)
		h, b := d.Hashes[c.head], d.Hashes[c.base]
		l, k := legacyOutcome(d, h, b), idOutcome(t, d, newTestRepoState(), len(c.spec.Missing) > 0, h, b)
		if l != k {
			got = append(got, fmt.Sprintf("%s head=%d base=%d | legacy: %v | lockstep: %v", c.name, c.head, c.base, l, k))
		}
	}
	sort.Strings(got)
	if want := goldenDivergenceLines(t); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("id walk vs legacy divergence set differs from the golden (%d lines, golden %d)\n--- got ---\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
}

func TestIDWalk_ShouldKeepBFSOrder_WhenDFSAndBFSDifferAtCap(t *testing.T) {
	for seed := range 300 {
		rng := rand.New(rand.NewSource(int64(31000 + seed)))
		spec, h, b := randomGeneral(rng)
		for _, limit := range []int{2, 3, 5, 8} {
			withMergeBaseLimit(t, limit)
			d := buildDAG(t, spec, nil)
			want := lockstepOutcome(d, d.Hashes[h], d.Hashes[b])
			if got := idOutcome(t, d, newTestRepoState(), false, d.Hashes[h], d.Hashes[b]); got != want {
				t.Fatalf("seed %d limit %d: id walk %v, plain walk %v", seed, limit, got, want)
			}
		}
	}
}

func TestIDWalk_ShouldFallBackToPlainWalkWithSameResult_WhenErrGraphFullMidWalk(t *testing.T) {
	_, dir, _ := testrepo.Diverged(t, 40, 3, 2)
	const wantA, wantB = 3, 2

	r := readerWithFlags(Flags{SharedCommitState: true}, &GoGitVCSReader{repoStateMaxGraphCommits: 8})
	assertRefsBalanced(t, r)
	gotA, gotB, err := r.AheadBehind(context.Background(), dir, "base")
	if err != nil {
		t.Fatalf("AheadBehind with a tiny graph: %v", err)
	}
	if gotA != wantA || gotB != wantB {
		t.Errorf("fallback result (%d,%d), want (%d,%d)", gotA, gotB, wantA, wantB)
	}
	entry, _ := r.openRepoEntry(dir)
	if full := r.repoState(entry).Graph().Stats().GraphFull; full == 0 {
		t.Error("graph never reported full: the fallback path was not exercised")
	}
}
