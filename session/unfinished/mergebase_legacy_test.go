package unfinished

// Test-only oracle: the stack-based mark-then-walk findMergeBase/countCommitsTo
// exactly as committed at 39fa33a11 (session/unfinished/gogit_vcs_reader.go
// lines 2190-2278), renamed with a legacyStack prefix. They share the live
// mergeBaseBFSLimit so differential tests run both walks under the same cap.
// Do not "fix" these: their quirks are the thing being compared against.

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// legacyStackFindMergeBase returns the most-recent common ancestor of h1 and h2 using BFS.
// It first marks ancestors of h1 (up to mergeBaseBFSLimit), then walks ancestors
// of h2 until it finds one that is already marked. If no merge base is found
// within the depth limit, a sentinel error is returned.
func legacyStackFindMergeBase(repo *git.Repository, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}

	// Mark ancestors of h1 (bounded).
	anc := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q := []plumbing.Hash{h1}
	visited := 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if anc[h] {
			continue
		}
		anc[h] = true
		visited++
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue // object missing from shallow clone or pack — skip
		}
		q = append(q, c.ParentHashes...)
	}

	// Walk from h2 breadth-first; first ancestor in anc is the nearest merge base.
	seen := make(map[plumbing.Hash]bool, mergeBaseBFSLimit)
	q = []plumbing.Hash{h2}
	visited = 0
	for len(q) > 0 && visited < mergeBaseBFSLimit {
		h := q[0]
		q = q[1:]
		if seen[h] {
			continue
		}
		seen[h] = true
		visited++
		if anc[h] {
			return h, nil
		}
		c, err := repo.CommitObject(h)
		if err != nil {
			if !errors.Is(err, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
			}
			continue
		}
		q = append(q, c.ParentHashes...)
	}
	return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", mergeBaseBFSLimit)
}

// legacyStackCountCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive). Bounded at
// mergeBaseBFSLimit for the same reason findMergeBase is: an unbounded walk on a
// long-lived branch that never merged into base would otherwise decode every
// commit back to the repo root. When the bound is hit the count is capped rather
// than exact — callers display it as "mergeBaseBFSLimit+" ahead/behind, which is
// the correct signal for "diverged a lot" without paying for an exact count.
func legacyStackCountCommitsTo(repo *git.Repository, start, stop plumbing.Hash) (int, error) {
	seen := make(map[plumbing.Hash]bool, 32)
	q := []plumbing.Hash{start}
	n := 0
	for len(q) > 0 && n < mergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if seen[h] || h == stop {
			continue
		}
		seen[h] = true
		n++
		c, err := repo.CommitObject(h)
		if err != nil {
			return 0, err
		}
		for _, p := range c.ParentHashes {
			if !seen[p] && p != stop {
				q = append(q, p)
			}
		}
	}
	return n, nil
}

// ---- tests ----

// walkOutcome is the comparable result of one findMergeBase + ahead/behind run.
type walkOutcome struct{ MB, Ahead, Behind string }

func (o walkOutcome) String() string {
	return "mb=" + o.MB + " ahead=" + o.Ahead + " behind=" + o.Behind
}

func outcomeFor(d *builtDAG, mb plumbing.Hash, mbErr error, count func(start plumbing.Hash) (int, error), head, base plumbing.Hash) walkOutcome {
	if mbErr != nil {
		if strings.Contains(mbErr.Error(), "merge base not found") {
			return walkOutcome{"none", "-", "-"}
		}
		return walkOutcome{"err", "-", "-"}
	}
	o := walkOutcome{MB: d.nodeName(mb), Ahead: "err", Behind: "err"}
	if n, err := count(head); err == nil {
		o.Ahead = itoa(n)
	}
	if n, err := count(base); err == nil {
		o.Behind = itoa(n)
	}
	return o
}

func legacyOutcome(d *builtDAG, head, base plumbing.Hash) walkOutcome {
	mb, err := legacyStackFindMergeBase(d.Repo, head, base)
	return outcomeFor(d, mb, err, func(s plumbing.Hash) (int, error) { return legacyStackCountCommitsTo(d.Repo, s, mb) }, head, base)
}

func lockstepOutcome(d *builtDAG, head, base plumbing.Hash) walkOutcome {
	pm := newParentMemo(d.Repo)
	mb, err := findMergeBase(pm, head, base)
	return outcomeFor(d, mb, err, func(s plumbing.Hash) (int, error) { return countCommitsTo(pm, s, mb) }, head, base)
}

func withMergeBaseLimit(t *testing.T, limit int) {
	t.Helper()
	orig := mergeBaseBFSLimit
	mergeBaseBFSLimit = limit
	t.Cleanup(func() { mergeBaseBFSLimit = orig })
}

// Sentinel: the copied oracle answers the merge-base cases that the pre-lockstep
// tests (limits_test.go countCommitsTo cap; mergebase_test.go fork cases) asserted.
func TestLegacyStackWalk_ShouldReturnSameAsPreLockstepTests_WhenSentinelCasesRun(t *testing.T) {
	t.Run("fork point and ahead/behind", func(t *testing.T) {
		spec, head, base := linearForkSpec(300, 2, 1)
		d := buildDAG(t, spec, nil)
		want := walkOutcome{MB: itoa(298), Ahead: "2", Behind: "1"}
		if got := legacyOutcome(d, d.Hashes[head], d.Hashes[base]); got != want {
			t.Errorf("legacy = %v, want %v", got, want)
		}
	})
	t.Run("fast-forward and identical", func(t *testing.T) {
		spec, head, base := linearForkSpec(20, 3, 0)
		d := buildDAG(t, spec, nil)
		for _, c := range []struct{ a, b, want int }{{head, base, base}, {base, head, base}, {head, head, head}} {
			mb, err := legacyStackFindMergeBase(d.Repo, d.Hashes[c.a], d.Hashes[c.b])
			if err != nil || mb != d.Hashes[c.want] {
				t.Errorf("legacy(%d,%d) = %s err=%v, want node %d", c.a, c.b, d.nodeName(mb), err, c.want)
			}
		}
	})
	t.Run("not found within limit", func(t *testing.T) {
		withMergeBaseLimit(t, shortFork)
		spec, head, base := linearForkSpec(12, 4, 6)
		d := buildDAG(t, spec, nil)
		if _, err := legacyStackFindMergeBase(d.Repo, d.Hashes[head], d.Hashes[base]); err == nil {
			t.Error("expected an error when the merge base is beyond mergeBaseBFSLimit")
		}
	})
	t.Run("countCommitsTo capped at the bound", func(t *testing.T) {
		withMergeBaseLimit(t, 3)
		spec, head, _ := linearForkSpec(6, 0, 0) // 6 commits, stop = oldest
		d := buildDAG(t, spec, nil)
		n, err := legacyStackCountCommitsTo(d.Repo, d.Hashes[head], d.Hashes[0])
		if err != nil || n != 3 {
			t.Errorf("legacyStackCountCommitsTo = %d err=%v, want 3", n, err)
		}
	})
}

func requireSameOutcome(t *testing.T, name string, d *builtDAG, a, b int) {
	t.Helper()
	l, k := legacyOutcome(d, d.Hashes[a], d.Hashes[b]), lockstepOutcome(d, d.Hashes[a], d.Hashes[b])
	if l != k {
		t.Errorf("%s pair (%d,%d): legacy {%v} != lockstep {%v}", name, a, b, l, k)
	}
}

// Linear fork shapes and merge-free trees must be identical old vs lockstep (600 DAGs).
// Single-merge DAGs are NOT required identical: lockstep diverges there (see the
// golden file and ADR-003, "Behaviour change introduced by the lockstep commit").
func TestFindMergeBase_ShouldEqualLegacyStack_WhenLinearOrSingleMergeDAG(t *testing.T) {
	const perKind = 300
	for i := range perKind {
		rng := rand.New(rand.NewSource(int64(1000 + i)))
		spec, head, base := randomLinearFork(rng)
		d := buildDAG(t, spec, nil)
		requireSameOutcome(t, "linear", d, head, base)
		requireSameOutcome(t, "linear", d, base, head)
	}
	for i := range perKind {
		rng := rand.New(rand.NewSource(int64(5000 + i)))
		spec, h, b := randomTree(rng)
		d := buildDAG(t, spec, nil)
		requireSameOutcome(t, "tree", d, h, b)
		requireSameOutcome(t, "tree", d, b, h)
		for range 3 {
			requireSameOutcome(t, "tree", d, rng.Intn(len(spec.Parents)), rng.Intn(len(spec.Parents)))
		}
	}
}

var updateMergeBaseGolden = flag.Bool("update-mergebase-golden", false, "rewrite testdata/mergebase_lockstep_divergences.golden")

const mergeBaseGoldenPath = "testdata/mergebase_lockstep_divergences.golden"

// divergenceCase is one DAG + pair + cap evaluated in the golden test.
type divergenceCase struct {
	name       string
	spec       dagSpec
	head, base int
	limit      int
}

func namedDivergenceCases() []divergenceCase {
	// criss-cross: 3=merge(1,2), 4=merge(2,1); heads 5,6 have best bases {1,2}.
	crissCross := dagSpec{Parents: [][]int{nil, {0}, {0}, {1, 2}, {2, 1}, {3}, {4}}}
	// octopus: node 4 merges 1,2,3; heads descend from it and from 3.
	octopus := dagSpec{Parents: [][]int{nil, {0}, {0}, {0}, {1, 2, 3}, {4}, {3}}}
	// shallow: a criss-cross whose root commits are missing from the store.
	shallow := crissCross.truncated(1)
	cs := []divergenceCase{
		{"crisscross", crissCross, 5, 6, 2000},
		{"crisscross-rev", crissCross, 6, 5, 2000},
		{"octopus", octopus, 5, 6, 2000},
		{"octopus-rev", octopus, 6, 5, 2000},
		{"shallow-crisscross", shallow, 5, 6, 2000},
		{"shallow-octopus", octopus.truncated(2), 5, 6, 2000},
	}
	for limit := 2; limit <= 8; limit++ {
		spec, head, base := linearForkSpec(12, 4, 3)
		cs = append(cs, divergenceCase{"cap-linear-limit" + itoa(limit), spec, head, base, limit})
	}
	return cs
}

func seededDivergenceCases() []divergenceCase {
	cs := make([]divergenceCase, 0, 1200)
	for i := range 200 {
		rng := rand.New(rand.NewSource(int64(9000 + i)))
		spec, h, b := randomGeneral(rng)
		for _, limit := range []int{2000, 4} {
			c := divergenceCase{"general-seed" + itoa(9000+i) + "-limit" + itoa(limit), spec, h, b, limit}
			cs = append(cs, c)
			sh := c
			sh.name = "shallow-" + c.name
			sh.spec = spec.truncated(len(spec.Parents) / 3)
			cs = append(cs, sh)
		}
	}
	for i := range 200 {
		rng := rand.New(rand.NewSource(int64(13000 + i)))
		spec, h, b := randomSingleMerge(rng)
		cs = append(cs,
			divergenceCase{"single-merge-seed" + itoa(13000+i) + "-fwd", spec, h, b, 2000},
			divergenceCase{"single-merge-seed" + itoa(13000+i) + "-rev", spec, b, h, 2000})
	}
	return cs
}

func divergenceLines(t *testing.T, cases []divergenceCase) (lines []string, total int) {
	for _, c := range cases {
		withMergeBaseLimit(t, c.limit)
		d := buildDAG(t, c.spec, nil)
		l, k := legacyOutcome(d, d.Hashes[c.head], d.Hashes[c.base]), lockstepOutcome(d, d.Hashes[c.head], d.Hashes[c.base])
		total++
		reportBestAgreement(t, d, c, l, k)
		if l != k {
			lines = append(lines, fmt.Sprintf("%s head=%d base=%d | legacy: %v | lockstep: %v", c.name, c.head, c.base, l, k))
		}
	}
	sort.Strings(lines)
	return lines, total
}

// reportBestAgreement logs (reporting only) whether each walk's merge base is in
// object.Commit.MergeBase's best set, tallied per case-name family.
var bestAgreement = map[string]*[3]int{} // family -> {cases, legacyBest, lockstepBest}

func reportBestAgreement(t *testing.T, d *builtDAG, c divergenceCase, l, k walkOutcome) {
	if len(c.spec.Missing) > 0 || c.limit != 2000 {
		return
	}
	best, err := d.bestSet(d.Hashes[c.head], d.Hashes[c.base])
	if err != nil {
		return
	}
	fam := strings.SplitN(c.name, "-seed", 2)[0]
	tally := bestAgreement[fam]
	if tally == nil {
		tally = new([3]int)
		bestAgreement[fam] = tally
	}
	tally[0]++
	for i, o := range []walkOutcome{l, k} {
		if best[o.MB] {
			tally[i+1]++
		}
	}
}

// Divergences old vs lockstep on criss-cross, octopus, shallow and cap-boundary
// DAGs are an accepted, recorded behaviour change (ADR-003); the checked-in
// golden file must match exactly. Regenerate with -update-mergebase-golden.
func TestFindMergeBase_ShouldRecordGoldenDivergence_WhenCrissCrossOctopusOrCapBoundary(t *testing.T) {
	lines, total := divergenceLines(t, append(namedDivergenceCases(), seededDivergenceCases()...))
	fams := make([]string, 0, len(bestAgreement))
	for fam := range bestAgreement {
		fams = append(fams, fam)
	}
	sort.Strings(fams)
	for _, fam := range fams {
		v := bestAgreement[fam]
		t.Logf("best-merge-base agreement (object.Commit.MergeBase oracle) %-14s cases=%d legacy-in-best=%d lockstep-in-best=%d", fam, v[0], v[1], v[2])
	}
	got := fmt.Sprintf("# old committed walk (39fa33a11) vs lockstep; %d cases evaluated, %d diverge\n%s\n", total, len(lines), strings.Join(lines, "\n"))
	if *updateMergeBaseGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mergeBaseGoldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(mergeBaseGoldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with -update-mergebase-golden)", err)
	}
	if string(want) != got {
		t.Errorf("divergence set changed; diff the golden file or rerun with -update-mergebase-golden\n--- got ---\n%s", got)
	}
}

type fakeParents map[plumbing.Hash]commitInfo

func (f fakeParents) info(h plumbing.Hash) (commitInfo, error) { return f[h], nil }
func (f fakeParents) get(h plumbing.Hash) ([]plumbing.Hash, error) {
	return f[h].parents, nil
}

// R (newest timestamp, clock skew) and O are both common to h1 and h2, and R is
// an ancestor of O through a chain. Committer-time pruning in the redundancy
// check (the paint-down's own assumption) cannot see through that skew, so R is
// returned; it is still a valid common ancestor. What must never happen is an
// error: a bounded or pruned check answers "not redundant" instead of failing.
func TestFindMergeBase_ShouldReturnCommonAncestorWithoutError_WhenSkewedRedundancyCheckIsPrunedOrBounded(t *testing.T) {
	hh := func(s string) plumbing.Hash { return plumbing.NewHash(fmt.Sprintf("%040s", s)) }
	r, m3, m2, m1, o, h1, h2 := hh("a1"), hh("a2"), hh("a3"), hh("a4"), hh("a5"), hh("a6"), hh("a7")
	pm := fakeParents{
		r: {when: 100}, m3: {parents: []plumbing.Hash{r}}, m2: {parents: []plumbing.Hash{m3}},
		m1: {parents: []plumbing.Hash{m2}}, o: {parents: []plumbing.Hash{m1}},
		h1: {parents: []plumbing.Hash{r, o}, when: 2}, h2: {parents: []plumbing.Hash{r, o}, when: 3},
	}
	for _, limit := range []int{2000, 3} {
		withMergeBaseLimit(t, limit)
		got, err := findMergeBase(pm, h1, h2)
		if err != nil || (got != r && got != o) {
			t.Fatalf("limit %d: got %s err=%v, want %s or %s with no error", limit, got, err, r, o)
		}
	}
}
