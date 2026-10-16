package unfinished

// Reference edit counts for the diff algorithm swap (Story 0.1.3b). There is
// no EditCounts type in the package yet, so the oracle returns a test-local
// struct; later stories (3.2.1/3.2.2) reuse countsOracle and randomLines.

import (
	"math/rand"
	"strings"
	"testing"
)

type editCounts struct{ Insertions, Deletions int }

// countsOracle derives insertion/deletion counts from the retained lcsLength.
func countsOracle(a, b []string) editCounts {
	lcs := lcsLength(a, b)
	return editCounts{Insertions: len(b) - lcs, Deletions: len(a) - lcs}
}

// randomLines draws a line slice of up to maxN lines from an alphabet of the
// given size (small alphabets give repeated lines; huge ones give disjoint).
func randomLines(rng *rand.Rand, maxN, alphabet int, prefix string) []string {
	out := make([]string, rng.Intn(maxN+1))
	for i := range out {
		out[i] = prefix + itoa(rng.Intn(alphabet))
	}
	return out
}

// lineSliceCase is one named input pair for diff oracle tests.
type lineSliceCase struct {
	name string
	a, b []string
}

// lineSliceCorpus is the seeded corpus: edge shapes plus random pairs up to maxN lines.
func lineSliceCorpus(maxN int) []lineSliceCase {
	rng := rand.New(rand.NewSource(424242))
	same := randomLines(rng, maxN, 50, "s")
	cs := make([]lineSliceCase, 0, 157)
	cs = append(cs,
		lineSliceCase{"both-empty", nil, nil},
		lineSliceCase{"a-empty", nil, []string{"x", "y"}},
		lineSliceCase{"b-empty", []string{"x", "y"}, nil},
		lineSliceCase{"identical", same, append([]string(nil), same...)},
		lineSliceCase{"disjoint", randomLines(rng, maxN, 10, "a"), randomLines(rng, maxN, 10, "b")},
		lineSliceCase{"all-repeated", make([]string, 40), make([]string, 25)},
		lineSliceCase{"max-size", randomLines(rand.New(rand.NewSource(1)), 2000, 30, "m"), randomLines(rand.New(rand.NewSource(2)), 2000, 30, "m")},
	)
	for i := range 150 {
		r := rand.New(rand.NewSource(int64(7000 + i)))
		alpha := []int{2, 5, 40, 1000}[i%4]
		cs = append(cs, lineSliceCase{"random-" + itoa(i), randomLines(r, maxN, alpha, "r"), randomLines(r, maxN, alpha, "r")})
	}
	return cs
}

func TestCountsOracle_ShouldReturnOneInsertionOneDeletion_WhenSingleLineSwapped(t *testing.T) {
	a, b := []string{"x", "y", "z"}, []string{"x", "z", "w"}
	if got := lcsLength(a, b); got != 2 {
		t.Errorf("lcsLength = %d, want 2", got)
	}
	if got, want := countsOracle(a, b), (editCounts{Insertions: 1, Deletions: 1}); got != want {
		t.Errorf("countsOracle = %+v, want %+v", got, want)
	}
}

func TestCountsOracle_ShouldMatchLinesDiff_WhenCorpusRuns(t *testing.T) {
	for _, c := range lineSliceCorpus(200) {
		want := countsOracle(c.a, c.b)
		ins, del := LinesDiff(strings.Join(c.a, "\n"), strings.Join(c.b, "\n"))
		// splitLines drops a trailing empty element, so rebuild the expectation
		// from the lines splitLines actually sees.
		if len(c.a) == 0 || len(c.b) == 0 || strings.Join(c.a, "\n") == "" || strings.Join(c.b, "\n") == "" {
			want = countsOracle(splitLines(strings.Join(c.a, "\n")), splitLines(strings.Join(c.b, "\n")))
		}
		if got := (editCounts{ins, del}); got != want {
			t.Errorf("%s: LinesDiff = %+v, oracle = %+v", c.name, got, want)
		}
	}
}

func TestCountsOracle_ShouldSatisfyInvariants_WhenCorpusRuns(t *testing.T) {
	for _, c := range lineSliceCorpus(2000) {
		got := countsOracle(c.a, c.b)
		if got.Insertions < 0 || got.Deletions < 0 || got.Insertions-got.Deletions != len(c.b)-len(c.a) {
			t.Errorf("%s: inconsistent counts %+v for len a=%d b=%d", c.name, got, len(c.a), len(c.b))
		}
		if swapped := countsOracle(c.b, c.a); swapped.Insertions != got.Deletions || swapped.Deletions != got.Insertions {
			t.Errorf("%s: not symmetric: %+v vs %+v", c.name, got, swapped)
		}
	}
}

func TestMyersCounter_ShouldEqualLcsOracle_When10000SeededPairs(t *testing.T) {
	for i := range spikePairs {
		a, b := spikePair(i)
		o := countsOracle(a, b)
		if got, want := MyersEditCounts(a, b), (EditCounts{Insertions: o.Insertions, Deletions: o.Deletions}); got != want {
			t.Fatalf("pair %d: myers = %+v, oracle = %+v", i, got, want)
		}
	}
	for _, c := range lineSliceCorpus(400) {
		o := countsOracle(c.a, c.b)
		if got, want := MyersEditCounts(c.a, c.b), (EditCounts{Insertions: o.Insertions, Deletions: o.Deletions}); got != want {
			t.Errorf("%s: myers = %+v, oracle = %+v", c.name, got, want)
		}
	}
}
