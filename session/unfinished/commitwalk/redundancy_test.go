package commitwalk

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// capBFS lowers the walk's visit bound for one test.
func capBFS(t *testing.T, limit int) {
	t.Helper()
	orig := MergeBaseBFSLimit
	MergeBaseBFSLimit = limit
	t.Cleanup(func() { MergeBaseBFSLimit = orig })
}

// skewedSource serves a hand-made graph so committer times can disagree with
// topology, which the real-commit fixture cannot express.
type skewedSource map[plumbing.Hash]CommitInfo

func (s skewedSource) Info(h plumbing.Hash) (CommitInfo, error) {
	ci, ok := s[h]
	if !ok {
		return CommitInfo{}, plumbing.ErrObjectNotFound
	}
	return ci, nil
}

func (s skewedSource) Get(h plumbing.Hash) ([]plumbing.Hash, error) {
	ci, err := s.Info(h)
	return ci.Parents, err
}

func TestIsAncestorOfOther_ShouldTreatCandidateAsNonRedundant_WhenBoundExhausted(t *testing.T) {
	capBFS(t, 3)
	hash := func(b byte) plumbing.Hash { return plumbing.Hash{b} }
	c, o := hash(1), hash(2)
	// o sits on a chain of newer commits longer than the bound that never reaches c.
	src := skewedSource{c: {When: 10}}
	prev := hash(3)
	src[o] = CommitInfo{Parents: []plumbing.Hash{prev}, When: 100}
	for b := byte(3); b < 12; b++ {
		next := hash(b + 1)
		src[hash(b)] = CommitInfo{Parents: []plumbing.Hash{next}, When: 100}
	}
	redundant, err := isAncestorOfOther(src, c, []plumbing.Hash{c, o})
	if err != nil || redundant {
		t.Fatalf("isAncestorOfOther = %v, %v; want false, nil at the bound", redundant, err)
	}
}

// A bound hit and a pruned miss are the two ways the redundancy check answers
// "not redundant" without finding the candidate; both are counted.
func TestIsAncestorOfOther_ShouldCountBoundAndPrunedMiss_WhenNotRedundantWithoutProof(t *testing.T) {
	capBFS(t, 3)
	hash := func(b byte) plumbing.Hash { return plumbing.Hash{b} }
	c, o := hash(1), hash(2)
	read := func() (bound, pruned int64) {
		return mergeBaseRedundancyTotal[RedundancyBound].Load(), mergeBaseRedundancyTotal[RedundancyPrunedMiss].Load()
	}

	// Bound: a newer chain longer than the limit that never reaches c.
	src := skewedSource{c: {When: 10}, o: {Parents: []plumbing.Hash{hash(3)}, When: 100}}
	for b := byte(3); b < 12; b++ {
		src[hash(b)] = CommitInfo{Parents: []plumbing.Hash{hash(b + 1)}, When: 100}
	}
	b0, p0 := read()
	redundant, err := isAncestorOfOther(src, c, []plumbing.Hash{c, o})
	b1, p1 := read()
	if err != nil || redundant || b1-b0 != 1 || p1-p0 != 0 {
		t.Fatalf("bound case: redundant=%v err=%v bound+%d pruned+%d; want false, nil, +1, +0", redundant, err, b1-b0, p1-p0)
	}

	// Pruned miss: o's whole history is older than c, so the walk prunes it and ends.
	older := skewedSource{c: {When: 100}, o: {Parents: []plumbing.Hash{hash(3)}, When: 5}, hash(3): {When: 4}}
	redundant, err = isAncestorOfOther(older, c, []plumbing.Hash{c, o})
	b2, p2 := read()
	if err != nil || redundant || b2-b1 != 0 || p2-p1 != 1 {
		t.Fatalf("pruned case: redundant=%v err=%v bound+%d pruned+%d; want false, nil, +0, +1", redundant, err, b2-b1, p2-p1)
	}
}

// Documents the clock-skew limit: c really is an ancestor of o, but c carries a
// newer committer time than o, so o is pruned before its parents are read and
// c stays a (valid, non-best) merge base. The id walk (Walk.reaches) answers
// the same; see TestWalk_ShouldAnswerNotRedundant_WhenAncestorHasNewerCommitterTime.
func TestIsAncestorOfOther_ShouldMissSkewedNewerAncestor_WhenCommitterClockSkewed(t *testing.T) {
	hash := func(b byte) plumbing.Hash { return plumbing.Hash{b} }
	c, o := hash(1), hash(2)
	src := skewedSource{c: {When: 200}, o: {Parents: []plumbing.Hash{c}, When: 100}}

	redundant, err := isAncestorOfOther(src, c, []plumbing.Hash{c, o})

	if err != nil || redundant {
		t.Fatalf("isAncestorOfOther = %v, %v; want false, nil (the documented skew limit)", redundant, err)
	}
	src[o] = CommitInfo{Parents: []plumbing.Hash{c}, When: 300} // same topology, sane clocks
	if redundant, err = isAncestorOfOther(src, c, []plumbing.Hash{c, o}); err != nil || !redundant {
		t.Fatalf("with ordered clocks isAncestorOfOther = %v, %v; want true, nil", redundant, err)
	}
}
