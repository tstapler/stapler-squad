package unfinished

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// crissCrossOverChain is a linear history of chainLen commits whose tip is
// forked into two tips x and y, then merged both ways: head=(x,y), base=(y,x).
// x and y are both best common ancestors, neither an ancestor of the other.
func crissCrossOverChain(chainLen int) (spec dagSpec, head, base, x, y int) {
	for i := 0; i < chainLen; i++ {
		if i == 0 {
			spec.Parents = append(spec.Parents, nil)
		} else {
			spec.Parents = append(spec.Parents, []int{i - 1})
		}
	}
	tip := chainLen - 1
	x, y = chainLen, chainLen+1
	spec.Parents = append(spec.Parents, []int{tip}, []int{tip}, []int{x, y}, []int{y, x})
	return spec, chainLen + 2, chainLen + 3, x, y
}

func TestFindMergeBase_ShouldReturnBase_WhenCrissCrossHistoryExceedsBound(t *testing.T) {
	spec, head, base, x, y := crissCrossOverChain(mergeBaseBFSLimit + 500)
	d := buildDAG(t, spec, nil)
	h, b := d.Hashes[head], d.Hashes[base]

	got, err := findMergeBase(newParentMemo(d.Repo), h, b)
	if err != nil {
		t.Fatalf("plain walk errored on a criss-cross over a deep history: %v", err)
	}
	if got != d.Hashes[x] && got != d.Hashes[y] {
		t.Fatalf("plain walk base = node %s, want %d or %d", d.nodeName(got), x, y)
	}
	idGot := idOutcome(t, d, newTestRepoState(), false, h, b)
	if idGot.MB != d.nodeName(got) {
		t.Errorf("id walk base = %s, plain walk = %s", idGot.MB, d.nodeName(got))
	}
	best, err := d.bestSet(h, b)
	if err != nil || !best[idGot.MB] {
		t.Errorf("id walk base %s not in oracle best set %v (err %v)", idGot.MB, best, err)
	}
}

// skewedSource serves a hand-made graph so committer times can disagree with
// topology, which the real-commit fixture cannot express.
type skewedSource map[plumbing.Hash]commitInfo

func (s skewedSource) info(h plumbing.Hash) (commitInfo, error) {
	ci, ok := s[h]
	if !ok {
		return commitInfo{}, plumbing.ErrObjectNotFound
	}
	return ci, nil
}

func (s skewedSource) get(h plumbing.Hash) ([]plumbing.Hash, error) {
	ci, err := s.info(h)
	return ci.parents, err
}

func TestIsAncestorOfOther_ShouldTreatCandidateAsNonRedundant_WhenBoundExhausted(t *testing.T) {
	withMergeBaseLimit(t, 3)
	hash := func(b byte) plumbing.Hash { return plumbing.Hash{b} }
	c, o := hash(1), hash(2)
	// o sits on a chain of newer commits longer than the bound that never reaches c.
	src := skewedSource{c: {when: 10}}
	prev := hash(3)
	src[o] = commitInfo{parents: []plumbing.Hash{prev}, when: 100}
	for b := byte(3); b < 12; b++ {
		next := hash(b + 1)
		src[hash(b)] = commitInfo{parents: []plumbing.Hash{next}, when: 100}
	}
	redundant, err := isAncestorOfOther(src, c, []plumbing.Hash{c, o})
	if err != nil || redundant {
		t.Fatalf("isAncestorOfOther = %v, %v; want false, nil at the bound", redundant, err)
	}
}
