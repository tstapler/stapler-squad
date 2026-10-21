package unfinished

// go-git-only commit-DAG fixtures for the merge-base differential tests. Kept
// apart from the tests so a shared testrepo package can adopt them later.

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

// dagSpec is a commit DAG: Parents[i] lists the parent node indexes of node i
// (each < i, in parent order). Missing nodes are omitted from the object store,
// emulating a shallow clone.
type dagSpec struct {
	Parents [][]int
	Missing map[int]bool
}

// builtDAG maps node index <-> commit hash for a materialized dagSpec.
type builtDAG struct {
	Repo   *git.Repository
	Hashes []plumbing.Hash
	index  map[plumbing.Hash]int
}

func (d *builtDAG) nodeName(h plumbing.Hash) string {
	if i, ok := d.index[h]; ok {
		return itoa(i)
	}
	return "?"
}

func itoa(i int) string { return strconv.Itoa(i) }

// buildDAG materializes spec in an in-memory go-git repo. Wrap, when non-nil,
// decorates the storer (e.g. with a counting wrapper) before the repo opens it.
func buildDAG(t testing.TB, spec dagSpec, wrap func(storage.Storer) storage.Storer) *builtDAG {
	t.Helper()
	st := memory.NewStorage()
	d := &builtDAG{Hashes: make([]plumbing.Hash, len(spec.Parents)), index: map[plumbing.Hash]int{}}
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, ps := range spec.Parents {
		c := &object.Commit{
			Author:    object.Signature{Name: "T", Email: "t@example.com", When: base.Add(time.Duration(i) * time.Second)},
			Committer: object.Signature{Name: "T", Email: "t@example.com", When: base.Add(time.Duration(i) * time.Second)},
			Message:   "n" + itoa(i),
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

// linearForkSpec is a main chain of mainLen commits plus forkLen fork commits
// off the node `behind` steps before main's tip. Returns head=fork tip, base=main tip.
func linearForkSpec(mainLen, forkLen, behind int) (spec dagSpec, head, base int) {
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

// randomLinearFork draws a linear fork shape.
func randomLinearFork(rng *rand.Rand) (dagSpec, int, int) {
	mainLen := 2 + rng.Intn(40)
	return linearForkSpec(mainLen, rng.Intn(9), rng.Intn(mainLen))
}

// randomTree draws a merge-free tree of n nodes (every node has one parent).
func randomTree(rng *rand.Rand) (dagSpec, int, int) {
	n := 5 + rng.Intn(36)
	spec := dagSpec{Parents: [][]int{nil}}
	for i := 1; i < n; i++ {
		spec.Parents = append(spec.Parents, []int{rng.Intn(i)})
	}
	return spec, rng.Intn(n), rng.Intn(n)
}

// randomSingleMerge draws a tree of n nodes plus exactly one extra parent edge,
// so the DAG has exactly one merge commit; the head/base pair is random.
func randomSingleMerge(rng *rand.Rand) (dagSpec, int, int) {
	n := 5 + rng.Intn(36)
	var spec dagSpec
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

// randomGeneral draws a merge-rich DAG: each node has 1-3 distinct earlier parents.
func randomGeneral(rng *rand.Rand) (dagSpec, int, int) {
	n := 6 + rng.Intn(25)
	var spec dagSpec
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

// truncated marks every node below cutoff as absent from the object store.
func (s dagSpec) truncated(cutoff int) dagSpec {
	s.Missing = map[int]bool{}
	for i := 0; i < cutoff && i < len(s.Parents); i++ {
		s.Missing[i] = true
	}
	return s
}

// bestSet returns object.Commit.MergeBase's best merge bases of a and b as node names.
func (d *builtDAG) bestSet(a, b plumbing.Hash) (map[string]bool, error) {
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
		out[d.nodeName(c.Hash)] = true
	}
	return out, nil
}
