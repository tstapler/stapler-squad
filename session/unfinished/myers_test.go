package unfinished

import "testing"

func TestMyers_ShouldReturnZeroEdits_WhenInputsIdentical(t *testing.T) {
	a := []string{"a", "b", "c"}
	if got := MyersEditCounts(a, append([]string(nil), a...)); got != (EditCounts{}) {
		t.Errorf("got %+v, want zero", got)
	}
}

func TestMyers_ShouldReturnAllInsertionsOrDeletions_WhenOneSideEmptyOrDisjoint(t *testing.T) {
	x, y := []string{"a", "b", "c"}, []string{"d", "e"}
	cases := []struct {
		name string
		a, b []string
		want EditCounts
	}{
		{"old-empty", nil, x, EditCounts{Insertions: 3}},
		{"new-empty", x, nil, EditCounts{Deletions: 3}},
		{"disjoint", x, y, EditCounts{Insertions: 2, Deletions: 3}},
	}
	for _, c := range cases {
		if got := MyersEditCounts(c.a, c.b); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestMyers_ShouldTrimPrefixSuffix_WhenChangeInMiddle(t *testing.T) {
	a := []string{"p", "q", "old1", "old2", "s", "t"}
	b := []string{"p", "q", "new", "s", "t"}
	if got, want := MyersEditCounts(a, b), (EditCounts{Insertions: 1, Deletions: 2}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// gateTB is the slice of testing.TB the G5 gate needs, so a negative twin can
// run the gate against a recorder.
type gateTB interface {
	Helper()
	Errorf(format string, args ...any)
}

type failRecorder struct{ failures int }

func (*failRecorder) Helper()                 {}
func (f *failRecorder) Errorf(string, ...any) { f.failures++ }

// assertG5 requires newImpl to allocate at most 25% of lcsLength's bytes and
// allocations on the 2,000-line/15-change fixture (plan Gate G5).
func assertG5(tb gateTB, newIterations int, newImpl func(a, b []string)) {
	tb.Helper()
	a, b := spikeFixture()
	// lcsLength costs tens of ms per call on this fixture, so it gets few
	// iterations; both results are per-op and compare directly.
	const oldIterations = 5
	oldBytes, oldAllocs := allocDelta(oldIterations, func() { _ = lcsLength(a, b) })
	newBytes, newAllocs := allocDelta(newIterations, func() { newImpl(a, b) })
	if newBytes > 0.25*oldBytes {
		tb.Errorf("G5: B/op = %.0f, ceiling = 25%% of lcsLength %.0f", newBytes, oldBytes)
	}
	if newAllocs > 0.25*oldAllocs {
		tb.Errorf("G5: allocs/op = %.2f, ceiling = 25%% of lcsLength %.2f", newAllocs, oldAllocs)
	}
}

func TestMyersCounter_ShouldUseAtMost25PctBytesAndAllocsOfLcsLength_When2000Lines15Changes(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	assertG5(t, 200, func(a, b []string) { _ = MyersEditCounts(a, b) })
}

func TestG5Gate_ShouldFail_WhenLcsLengthIsWiredAsNew(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	rec := &failRecorder{}
	assertG5(rec, 5, func(a, b []string) { _ = lcsLength(a, b) })
	if rec.failures == 0 {
		t.Error("G5 gate passed with lcsLength wired as the new implementation")
	}
}
