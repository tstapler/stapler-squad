package testrepo

// go-git-only commit-DAG fixtures for the merge-base differential tests.

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"
)

// DagSpec is a commit DAG: Parents[i] lists the parent node indexes of node i
// (each < i, in parent order). Missing nodes are omitted from the object store,
// emulating a shallow clone.
type DagSpec struct {
	Parents [][]int
	Missing map[int]bool
}

// BuiltDAG maps node index <-> commit hash for a materialized DagSpec.
type BuiltDAG struct {
	Repo   *git.Repository
	Hashes []plumbing.Hash
	index  map[plumbing.Hash]int
}

func (d *BuiltDAG) NodeName(h plumbing.Hash) string {
	if i, ok := d.index[h]; ok {
		return Itoa(i)
	}
	return "?"
}

// Itoa names a node index.
func Itoa(i int) string { return strconv.Itoa(i) }

// BuildDAG materializes spec in an in-memory go-git repo. Wrap, when non-nil,
// decorates the storer (e.g. with a counting wrapper) before the repo opens it.
func BuildDAG(t testing.TB, spec DagSpec, wrap func(storage.Storer) storage.Storer) *BuiltDAG {
	t.Helper()
	st := memory.NewStorage()
	d := &BuiltDAG{Hashes: make([]plumbing.Hash, len(spec.Parents)), index: map[plumbing.Hash]int{}}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, ps := range spec.Parents {
		c := &object.Commit{
			Author:    object.Signature{Name: "T", Email: "t@example.com", When: base.Add(time.Duration(i) * time.Second)},
			Committer: object.Signature{Name: "T", Email: "t@example.com", When: base.Add(time.Duration(i) * time.Second)},
			Message:   "n" + Itoa(i),
		}
		for _, p := range ps {
			c.ParentHashes = append(c.ParentHashes, d.Hashes[p])
		}
		obj := st.NewEncodedObject()
		if err := c.Encode(obj); err != nil {
			t.Fatalf("encode node %d: %v", i, err)
		}
		d.Hashes[i] = obj.Hash()
		d.index[obj.Hash()] = i
		if spec.Missing[i] {
			continue
		}
		if _, err := st.SetEncodedObject(obj); err != nil {
			t.Fatalf("store node %d: %v", i, err)
		}
	}
	var s storage.Storer = st
	if wrap != nil {
		s = wrap(st)
	}
	repo, err := git.Init(s, nil)
	if err != nil {
		t.Fatalf("git.Init: %v", err)
	}
	d.Repo = repo
	return d
}

// LinearForkSpec is a main chain of mainLen commits plus forkLen fork commits
// off the node `behind` steps before main's tip. Returns head=fork tip, base=main tip.
func LinearForkSpec(mainLen, forkLen, behind int) (spec DagSpec, head, base int) {
	for i := 0; i < mainLen; i++ {
		if i == 0 {
			spec.Parents = append(spec.Parents, nil)
		} else {
			spec.Parents = append(spec.Parents, []int{i - 1})
		}
	}
	tip := mainLen - 1 - behind
	for range forkLen {
		spec.Parents = append(spec.Parents, []int{tip})
		tip = len(spec.Parents) - 1
	}
	return spec, tip, mainLen - 1
}

// RandomLinearFork draws a linear fork shape.
func RandomLinearFork(rng *rand.Rand) (DagSpec, int, int) {
	mainLen := 2 + rng.Intn(40)
	return LinearForkSpec(mainLen, rng.Intn(9), rng.Intn(mainLen))
}

// RandomTree draws a merge-free tree of n nodes (every node has one parent).
func RandomTree(rng *rand.Rand) (DagSpec, int, int) {
	n := 5 + rng.Intn(36)
	spec := DagSpec{Parents: [][]int{nil}}
	for i := 1; i < n; i++ {
		spec.Parents = append(spec.Parents, []int{rng.Intn(i)})
	}
	return spec, rng.Intn(n), rng.Intn(n)
}

// RandomSingleMerge draws a tree of n nodes plus exactly one extra parent edge,
// so the DAG has exactly one merge commit; the head/base pair is random.
func RandomSingleMerge(rng *rand.Rand) (DagSpec, int, int) {
	n := 5 + rng.Intn(36)
	var spec DagSpec
	spec.Parents = append(spec.Parents, nil)
	for i := 1; i < n; i++ {
		spec.Parents = append(spec.Parents, []int{rng.Intn(i)})
	}
	k := 2 + rng.Intn(n-2)
	if p := rng.Intn(k); p != spec.Parents[k][0] {
		spec.Parents[k] = append(spec.Parents[k], p)
	}
	return spec, rng.Intn(n), rng.Intn(n)
}

// RandomGeneral draws a merge-rich DAG: each node has 1-3 distinct earlier parents.
func RandomGeneral(rng *rand.Rand) (DagSpec, int, int) {
	n := 6 + rng.Intn(25)
	var spec DagSpec
	spec.Parents = append(spec.Parents, nil)
	for i := 1; i < n; i++ {
		want := 1 + rng.Intn(3)
		var ps []int
		for tries := 0; len(ps) < want && tries < 6; tries++ {
			p := rng.Intn(i)
			dup := false
			for _, q := range ps {
				dup = dup || q == p
			}
			if !dup {
				ps = append(ps, p)
			}
		}
		spec.Parents = append(spec.Parents, ps)
	}
	return spec, rng.Intn(n), rng.Intn(n)
}

// Truncated marks every node below cutoff as absent from the object store.
func (s DagSpec) Truncated(cutoff int) DagSpec {
	s.Missing = map[int]bool{}
	for i := 0; i < cutoff && i < len(s.Parents); i++ {
		s.Missing[i] = true
	}
	return s
}

// BestSet returns object.Commit.MergeBase's best merge bases of a and b as node names.
func (d *BuiltDAG) BestSet(a, b plumbing.Hash) (map[string]bool, error) {
	ca, err := d.Repo.CommitObject(a)
	if err != nil {
		return nil, err
	}
	cb, err := d.Repo.CommitObject(b)
	if err != nil {
		return nil, err
	}
	bases, err := ca.MergeBase(cb)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, c := range bases {
		out[d.NodeName(c.Hash)] = true
	}
	return out, nil
}

// CommonAncestors returns the node indexes reachable from both a and b
// (inclusive). A missing (shallow-boundary) node counts as an ancestor, since
// its hash is still named by a child, but its own parents are unreachable.
func (s DagSpec) CommonAncestors(a, b int) map[int]bool {
	reach := func(start int) map[int]bool {
		seen := map[int]bool{}
		stack := []int{start}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			if s.Missing[n] {
				continue
			}
			stack = append(stack, s.Parents[n]...)
		}
		return seen
	}
	ra, rb := reach(a), reach(b)
	out := map[int]bool{}
	for n := range ra {
		if rb[n] {
			out[n] = true
		}
	}
	return out
}
