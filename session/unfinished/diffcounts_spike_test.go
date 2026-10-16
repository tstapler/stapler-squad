package unfinished

// ADR-005 spike (Story 3.2.1): candidates (a) in-package Myers and (b)
// diffmatchpatch.DiffMainRunes, each compared with the lcsLength oracle.

import (
	"math/rand"
	"runtime"
	"testing"

	"github.com/sergi/go-diff/diffmatchpatch"
)

const spikePairs = 10_000

// spikePair derives the i-th seeded pair: independent random slices for even
// i, and a mutated copy (a few inserts/deletes/replacements) for odd i, which
// is the shape real file edits take.
func spikePair(i int) (a, b []string) {
	rng := rand.New(rand.NewSource(int64(90_000 + i)))
	alpha := []int{2, 5, 40, 1000}[i%4]
	a = randomLines(rng, 60, alpha, "p")
	if i%2 == 0 {
		return a, randomLines(rng, 60, alpha, "p")
	}
	b = append([]string(nil), a...)
	for range rng.Intn(8) {
		switch pos := rng.Intn(len(b) + 1); rng.Intn(3) {
		case 0:
			b = append(b[:pos], append([]string{"ins" + itoa(rng.Intn(9))}, b[pos:]...)...)
		case 1:
			if pos < len(b) {
				b = append(b[:pos], b[pos+1:]...)
			}
		default:
			if pos < len(b) {
				b[pos] = "rep" + itoa(rng.Intn(9))
			}
		}
	}
	return a, b
}

// dmpEditCounts is candidate (b): lines interned to runes, DiffTimeout=0 so
// the half-match shortcut is off, no semantic cleanup.
func dmpEditCounts(a, b []string) EditCounts {
	ids := map[string]rune{}
	toRunes := func(lines []string) []rune {
		out := make([]rune, len(lines))
		for i, l := range lines {
			id, ok := ids[l]
			if !ok {
				id = rune(len(ids) + 1)
				ids[l] = id
			}
			out[i] = id
		}
		return out
	}
	dmp := diffmatchpatch.New()
	dmp.DiffTimeout = 0
	var c EditCounts
	for _, d := range dmp.DiffMainRunes(toRunes(a), toRunes(b), false) {
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			c.Insertions += len([]rune(d.Text))
		case diffmatchpatch.DiffDelete:
			c.Deletions += len([]rune(d.Text))
		}
	}
	return c
}

func TestDiffCandidates_ShouldReportMismatchCountsVsOracle_When10000PairsEach(t *testing.T) {
	var mismatchMyers, mismatchDMP int
	for i := range spikePairs {
		a, b := spikePair(i)
		o := countsOracle(a, b)
		want := EditCounts{Insertions: o.Insertions, Deletions: o.Deletions}
		if got := MyersEditCounts(a, b); got != want {
			mismatchMyers++
			t.Errorf("pair %d: myers = %+v, oracle = %+v", i, got, want)
		}
		if got := dmpEditCounts(a, b); got != want {
			mismatchDMP++
		}
	}
	t.Logf("ADR-005 spike: pairs=%d mismatches: (a) myers=%d, (b) diffmatchpatch=%d", spikePairs, mismatchMyers, mismatchDMP)
}

// spikeFixture is the G5 shape: 2,000 lines with 15 replaced lines.
func spikeFixture() (a, b []string) {
	a = make([]string, 2000)
	for i := range a {
		a[i] = "line " + itoa(i) + " of the file"
	}
	b = append([]string(nil), a...)
	for k := range 15 {
		b[k*130+7] = "changed " + itoa(k)
	}
	return a, b
}

// allocDelta reports per-call TotalAlloc and Mallocs over n calls after one
// warm-up call; single goroutine, so concurrent allocators do not leak in.
func allocDelta(n int, f func()) (bytesPerOp, allocsPerOp float64) {
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range n {
		f()
	}
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(n), float64(after.Mallocs-before.Mallocs) / float64(n)
}

func TestDiffCandidates_ShouldReportAllocationsVsLcsLength_When2000Lines15Changes(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	a, b := spikeFixture()
	type row struct {
		name string
		n    int
		f    func()
	}
	rows := []row{
		{"lcsLength", 5, func() { _ = lcsLength(a, b) }},
		{"myers", 200, func() { _ = MyersEditCounts(a, b) }},
		{"diffmatchpatch", 5, func() { _ = dmpEditCounts(a, b) }},
	}
	for _, r := range rows {
		bytesOp, allocsOp := allocDelta(r.n, r.f)
		t.Logf("%-15s B/op=%10.0f allocs/op=%8.1f", r.name, bytesOp, allocsOp)
	}
}

// Advisory timing; the allocation numbers above are the ADR-005 evidence.
func BenchmarkDiffCandidates(b *testing.B) {
	fixtures := map[string][2][]string{}
	x, y := spikeFixture()
	fixtures["2000lines15changes"] = [2][]string{x, y}
	dx, dy := randomLines(rand.New(rand.NewSource(5)), 2000, 1<<30, "d"), randomLines(rand.New(rand.NewSource(6)), 2000, 1<<30, "e")
	fixtures["2000lines-disjoint"] = [2][]string{dx, dy}
	for name, f := range fixtures {
		a, c := f[0], f[1]
		b.Run(name+"/lcsLength", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = lcsLength(a, c)
			}
		})
		b.Run(name+"/myers", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = MyersEditCounts(a, c)
			}
		})
		b.Run(name+"/diffmatchpatch", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = dmpEditCounts(a, c)
			}
		})
	}
}
