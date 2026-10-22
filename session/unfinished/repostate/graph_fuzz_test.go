package repostate

import (
	"context"
	"testing"
)

func FuzzGraph_ShouldEqualMapReference_WhenRandomDAGs(f *testing.F) {
	for _, seed := range []int64{0, 1, 2, 3, 42, 1 << 40} {
		f.Add(seed, uint16(150))
	}
	f.Fuzz(func(t *testing.T, seed int64, n uint16) {
		size := int(n%400) + 1
		d := newDAG(seed, size)
		g := NewGraph(Options{MaxGraphCommits: size * 8}).Load()
		src := &mapSource{d: d}
		var buf []uint32
		for i := len(d.hashes) - 1; i >= 0; i-- { // reverse order: parents are interned before their own fill
			h := d.hashes[i]
			var st RowState
			var err error
			if buf, st, err = g.Fill(context.Background(), mustIntern(t, g, h), src, 0, buf[:0]); err != nil || st != Filled {
				t.Fatalf("Fill(%d) = %v, %v", i, st, err)
			}
			if !equalHashes(hashesOf(g, buf), d.parents[h]) {
				t.Fatalf("commit %d parents differ from reference", i)
			}
		}
	})
}
