package unfinished

import (
	"bytes"
	"sync"
)

// MyersEditCountsBytes is MyersEditCounts over byte-slice lines, the shape the
// production diff path works in (lines are subslices of the file buffers).
func MyersEditCountsBytes(a, b [][]byte) EditCounts { return countEditsBytes(a, b, false) }

// countEditsBytes returns exact edit counts. With hybrid set, Myers bails out
// once D passes hybridMaxD and the exact DP finishes instead: Myers is
// O((n+m)*D), which loses to the O(n*m) DP when almost no lines match (ADR-005:
// 2,000 disjoint lines cost 37.7 ms Myers against 7.9 ms DP).
func countEditsBytes(a, b [][]byte, hybrid bool) EditCounts {
	for len(a) > 0 && len(b) > 0 && bytes.Equal(a[0], b[0]) {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && bytes.Equal(a[len(a)-1], b[len(b)-1]) {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return EditCounts{Insertions: m, Deletions: n}
	}
	maxD := -1
	if hybrid {
		maxD = hybridMaxD(n, m)
	}
	d := myersCore(n, m, func(x, y int) bool { return bytes.Equal(a[x], b[y]) }, maxD)
	if d < 0 {
		// The caller already enforced maxLCSLines on the untrimmed inputs.
		lcs := lcsLengthBytes(a, b)
		return EditCounts{Insertions: m - lcs, Deletions: n - lcs}
	}
	return EditCounts{Insertions: (d + m - n) / 2, Deletions: (d - m + n) / 2}
}

// hybridMaxD is the edit distance past which the DP is cheaper than finishing
// Myers: about sqrt(n*m)/2. Measured per-cell cost is ~2 ns for the DP and
// ~4.6 ns for Myers (ADR-005 worst-case row), so the wasted Myers prefix is at
// most ~D^2/2 cells = n*m/8, bounding the total near 2.6 ns * n*m.
func hybridMaxD(n, m int) int {
	d := 1
	for (d+1)*(d+1)*4 <= n*m {
		d++
	}
	return d
}

// lineScratch pools the [][]byte line tables so splitting does not allocate
// per diff; the cap check keeps one huge file from pinning memory in the pool.
var lineScratch = sync.Pool{New: func() any {
	s := make([][]byte, 0, 512)
	return &s
}}

const maxPooledLines = 1 << 15

// splitLinesBytesInto splits data like splitLinesBytes (subslices, trailing
// empty element dropped) but appends into dst[:0].
func splitLinesBytesInto(dst [][]byte, data []byte) [][]byte {
	dst = dst[:0]
	if len(data) == 0 {
		return dst
	}
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			dst = append(dst, data)
			break
		}
		dst = append(dst, data[:i])
		data = data[i+1:]
	}
	if len(dst[len(dst)-1]) == 0 {
		dst = dst[:len(dst)-1]
	}
	return dst
}

func releaseLines(p *[][]byte, lines [][]byte) {
	if cap(lines) > maxPooledLines {
		return
	}
	*p = lines[:0]
	lineScratch.Put(p)
}

// linesDiffMyers is linesDiffBytes backed by the hybrid Myers counter. It keeps
// the maxLCSLines "fully replaced" approximation so results stay identical.
func linesDiffMyers(old, newData []byte) (insertions, deletions int) {
	pa, pb := lineScratch.Get().(*[][]byte), lineScratch.Get().(*[][]byte)
	a := splitLinesBytesInto(*pa, old)
	b := splitLinesBytesInto(*pb, newData)
	defer releaseLines(pa, a)
	defer releaseLines(pb, b)
	if len(a) > maxLCSLines || len(b) > maxLCSLines {
		return len(b), len(a)
	}
	c := countEditsBytes(a, b, true)
	return c.Insertions, c.Deletions
}
