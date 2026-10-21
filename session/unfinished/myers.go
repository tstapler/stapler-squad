package unfinished

import "sync"

// EditCounts is the exact insertion/deletion count between two line
// sequences: Insertions = len(new)-LCS, Deletions = len(old)-LCS.
type EditCounts struct{ Insertions, Deletions int }

// myersScratch holds the reusable V array. sync.Pool is per-P and lock-free on
// the fast path; the pointer indirection keeps Put from allocating.
var myersScratch = sync.Pool{New: func() any {
	v := make([]int, 0, 1024)
	return &v
}}

// MyersEditCounts returns the exact EditCounts of a -> b using Myers' greedy
// O((n+m)*D) forward algorithm, after trimming the common prefix and suffix.
// It needs only the edit distance D (LCS = (n+m-D)/2), so no trace is kept
// and the steady-state cost is zero heap allocations.
//
// Unlike lcsLength it has no maxLCSLines cap: callers keep that cap and its
// "fully replaced" approximation at the call site.
func MyersEditCounts(a, b []string) EditCounts {
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return EditCounts{Insertions: m, Deletions: n}
	}
	d := myersDistance(a, b)
	return EditCounts{Insertions: (d + m - n) / 2, Deletions: (d - m + n) / 2}
}

// myersDistance returns the shortest edit script length (insertions plus
// deletions) between non-empty a and b.
func myersDistance(a, b []string) int {
	return myersCore(len(a), len(b), func(x, y int) bool { return a[x] == b[y] }, -1)
}

// myersCore is Myers' greedy forward algorithm over index-addressed sequences
// of lengths n, m >= 1, with same(x, y) reporting a[x] == b[y]. It returns -1
// once the distance exceeds maxD (maxD < 0 means unbounded), which lets
// callers bail out to a different algorithm.
func myersCore(n, m int, same func(x, y int) bool, maxD int) int {
	limit := n + m
	off := limit + 1

	vp, v := acquireMyersV(2*limit + 3)
	defer myersScratch.Put(vp)

	// Every cell read in round d was written in round d-1, so stale pool
	// contents are never observed; only the seed cell needs setting.
	v[off+1] = 0
	delta := n - m
	for d := 0; d <= limit; d++ {
		if maxD >= 0 && d > maxD {
			return -1
		}
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && same(x, y) {
				x++
				y++
			}
			v[off+k] = x
			if k == delta && x >= n {
				return d
			}
		}
	}
	return limit
}

// acquireMyersV returns a pooled scratch slice of length need; the pointer is
// what goes back to the pool, with any growth retained.
func acquireMyersV(need int) (*[]int, []int) {
	vp := myersScratch.Get().(*[]int)
	if cap(*vp) < need {
		*vp = make([]int, need)
	}
	*vp = (*vp)[:cap(*vp)]
	return vp, (*vp)[:need]
}
