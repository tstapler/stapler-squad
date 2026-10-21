package unfinished

import (
	"strings"
	"testing"
)

func toLines(ss []string) [][]byte {
	out := make([][]byte, len(ss))
	for i, s := range ss {
		out[i] = []byte(s)
	}
	return out
}

func TestMyersBytes_ShouldEqualLcsLengthBytes_WhenSeededPairs(t *testing.T) {
	check := func(name string, a, b []string) {
		t.Helper()
		ab, bb := toLines(a), toLines(b)
		lcs := lcsLengthBytes(ab, bb)
		want := EditCounts{Insertions: len(bb) - lcs, Deletions: len(ab) - lcs}
		if o := EditCounts(countsOracle(a, b)); o != want {
			t.Fatalf("%s: string oracle %+v disagrees with lcsLengthBytes %+v", name, o, want)
		}
		if got := MyersEditCountsBytes(ab, bb); got != want {
			t.Fatalf("%s: pure myers = %+v, oracle = %+v", name, got, want)
		}
		if got := countEditsBytes(ab, bb, true); got != want {
			t.Fatalf("%s: hybrid myers = %+v, oracle = %+v", name, got, want)
		}
	}
	for i := range spikePairs {
		a, b := spikePair(i)
		check("pair "+itoa(i), a, b)
	}
	for _, c := range lineSliceCorpus(400) {
		check(c.name, c.a, c.b)
	}
}

func TestHybridMyers_ShouldBoundWorkAndStayExact_WhenLinesAreDisjoint(t *testing.T) {
	const n = 2000
	a, b := make([]string, n), make([]string, n)
	for i := range n {
		a[i], b[i] = "old "+itoa(i), "new "+itoa(i)
	}
	ab, bb := toLines(a), toLines(b)

	var calls int
	d := myersCore(n, n, func(x, y int) bool { calls++; return string(ab[x]) == string(bb[y]) }, hybridMaxD(n, n))
	if d != -1 {
		t.Fatalf("myersCore with the hybrid bound returned %d, want -1 (bail out) on disjoint input", d)
	}
	// Unbounded Myers visits ~ (n+m)^2/2 = 8M cells here; the bound caps it near n*m/8.
	if limit := n * n / 4; calls > limit {
		t.Errorf("bounded Myers made %d comparisons, want <= %d", calls, limit)
	}
	if got, want := countEditsBytes(ab, bb, true), (EditCounts{Insertions: n, Deletions: n}); got != want {
		t.Errorf("hybrid on disjoint input = %+v, want %+v", got, want)
	}
}

func TestHybridMyers_ShouldNotBailOut_WhenFewEdits(t *testing.T) {
	a, b := spikeFixture()
	ab, bb := toLines(a), toLines(b)
	if d := myersCore(len(ab), len(bb), func(x, y int) bool { return string(ab[x]) == string(bb[y]) }, hybridMaxD(len(ab), len(bb))); d != 30 {
		t.Errorf("distance = %d, want 30 (15 replacements) without bailing out", d)
	}
}

func TestLinesDiffMyers_ShouldMatchLinesDiffBytes_WhenContentVaries(t *testing.T) {
	cases := []struct{ name, old, new string }{
		{"both-empty", "", ""},
		{"old-empty", "", "a\nb\n"},
		{"new-empty", "a\nb\n", ""},
		{"no-trailing-newline", "a\nb", "a\nc"},
		{"blank-lines", "\n\n\n", "\n"},
		{"lone-newline", "\n", "x\n"},
		{"crlf", "a\r\nb\r\n", "a\r\nc\r\n"},
		{"reorder", "1\n2\n3\n4\n", "4\n3\n2\n1\n"},
	}
	for _, c := range cases {
		wi, wd := linesDiffBytes([]byte(c.old), []byte(c.new))
		if gi, gd := linesDiffMyers([]byte(c.old), []byte(c.new)); gi != wi || gd != wd {
			t.Errorf("%s: myers = (%d,%d), legacy = (%d,%d)", c.name, gi, gd, wi, wd)
		}
	}
}

func TestMyersCounter_ShouldReturnFullReplaceApproximation_WhenOver20000Lines(t *testing.T) {
	big := strings.Repeat("x\n", maxLCSLines+1)
	small := strings.Repeat("x\n", 10)
	for _, c := range []struct {
		name, old, new string
		ins, del       int
	}{
		{"old-over-cap", big, small, 10, maxLCSLines + 1},
		{"new-over-cap", small, big, maxLCSLines + 1, 10},
	} {
		gi, gd := linesDiffMyers([]byte(c.old), []byte(c.new))
		li, ld := linesDiffBytes([]byte(c.old), []byte(c.new))
		if gi != c.ins || gd != c.del || li != c.ins || ld != c.del {
			t.Errorf("%s: myers = (%d,%d), legacy = (%d,%d), want (%d,%d)", c.name, gi, gd, li, ld, c.ins, c.del)
		}
	}
}

func assertBytesPathAllocCeiling(tb gateTB, newIterations int, newImpl func(old, new []byte)) {
	tb.Helper()
	a, b := spikeFixture()
	oldData, newData := []byte(strings.Join(a, "\n")+"\n"), []byte(strings.Join(b, "\n")+"\n")
	oldBytes, _ := allocDelta(5, func() { linesDiffBytes(oldData, newData) })
	newBytes, _ := allocDelta(newIterations, func() { newImpl(oldData, newData) })
	if newBytes > 0.25*oldBytes {
		tb.Errorf("G5 (bytes path): B/op = %.0f, ceiling = 25%% of linesDiffBytes %.0f", newBytes, oldBytes)
	}
}

func TestLinesDiffMyers_ShouldUseAtMost25PctBytesOfLinesDiffBytes_When2000Lines15Changes(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	assertBytesPathAllocCeiling(t, 200, func(o, n []byte) { linesDiffMyers(o, n) })
}

func TestBytesPathGate_ShouldFail_WhenLegacyIsWiredAsNew(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	rec := &failRecorder{}
	assertBytesPathAllocCeiling(rec, 5, func(o, n []byte) { linesDiffBytes(o, n) })
	if rec.failures == 0 {
		t.Error("gate passed with linesDiffBytes wired as the new implementation")
	}
}
