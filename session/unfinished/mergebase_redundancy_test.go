package unfinished

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/unfinished/commitwalk"
	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

// crissCrossOverChain is a linear history of chainLen commits whose tip is
// forked into two tips x and y, then merged both ways: head=(x,y), base=(y,x).
// x and y are both best common ancestors, neither an ancestor of the other.
func crissCrossOverChain(chainLen int) (spec testrepo.DagSpec, head, base, x, y int) {
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
	spec, head, base, x, y := crissCrossOverChain(commitwalk.MergeBaseBFSLimit + 500)
	d := testrepo.BuildDAG(t, spec, nil)
	h, b := d.Hashes[head], d.Hashes[base]

	got, err := commitwalk.FindMergeBase(commitwalk.NewParentMemo(d.Repo), h, b)
	if err != nil {
		t.Fatalf("plain walk errored on a criss-cross over a deep history: %v", err)
	}
	if got != d.Hashes[x] && got != d.Hashes[y] {
		t.Fatalf("plain walk base = node %s, want %d or %d", d.NodeName(got), x, y)
	}
	idGot := idOutcome(t, d, newTestRepoState(), false, h, b)
	if idGot.MB != d.NodeName(got) {
		t.Errorf("id walk base = %s, plain walk = %s", idGot.MB, d.NodeName(got))
	}
	best, err := d.BestSet(h, b)
	if err != nil || !best[idGot.MB] {
		t.Errorf("id walk base %s not in oracle best set %v (err %v)", idGot.MB, best, err)
	}
}
