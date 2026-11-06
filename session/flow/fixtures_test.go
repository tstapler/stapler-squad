package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Self-contained fixture loaders for the golden files in testdata/. Names are
// prefixed `fixture` so they cannot collide with production types in this package.

const fixtureTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

type fixtureEvent struct {
	Seq         int    `json:"seq"`
	ItemID      string `json:"item_id"`
	From        string `json:"from"`
	To          string `json:"to"`
	At          string `json:"at"`
	Note        string `json:"note"`
	TriggeredBy string `json:"triggered_by"`
}

func (e fixtureEvent) instant(t *testing.T) time.Time {
	t.Helper()
	ts, err := time.Parse(fixtureTimeLayout, e.At)
	require.NoError(t, err, "event %d at %q", e.Seq, e.At)
	return ts.UTC()
}

type fixtureEvents struct {
	Items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"items"`
	Events []fixtureEvent `json:"events"`
}

type fixtureCriterion struct {
	CriterionIndex int    `json:"criterion_index"`
	Outcome        string `json:"outcome"`
}

type fixtureVerdict struct {
	ItemID         string             `json:"item_id"`
	SessionID      string             `json:"session_id"`
	CreatedAt      string             `json:"created_at"`
	OverallOutcome string             `json:"overall_outcome"`
	Summary        string             `json:"summary"`
	PerCriterion   []fixtureCriterion `json:"per_criterion"`
	DiffHash       string             `json:"diff_hash"`
	OverrideBy     string             `json:"override_by"`
}

type fixtureVerdicts struct {
	Verdicts []fixtureVerdict `json:"verdicts"`
}

type fixtureStuckInterval struct {
	ItemID          string  `json:"item_id"`
	Reason          string  `json:"reason"`
	FirstDetectedAt string  `json:"first_detected_at"`
	ResolvedAt      *string `json:"resolved_at"`
}

type fixtureStuck struct {
	Intervals []fixtureStuckInterval `json:"intervals"`
}

func loadFixture[T any](t *testing.T, name string) T {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var out T
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func (f fixtureEvents) forItem(id string) []fixtureEvent {
	var out []fixtureEvent
	for _, e := range f.Events {
		if e.ItemID == id {
			out = append(out, e)
		}
	}
	return out
}

// fixtureEpisodes counts flip episodes the way the glossary defines them:
// runs of at least minEvents events with every gap under gap, then adjacent
// runs closer than mergeGap merge. It is a test-local oracle, not the production collapse.
func fixtureEpisodes(t *testing.T, evs []fixtureEvent, gap time.Duration, minEvents int, mergeGap time.Duration) (episodes int, runLens []int) {
	t.Helper()
	type run struct {
		first, last time.Time
		n           int
	}
	var runs []run
	for _, e := range evs {
		at := e.instant(t)
		if n := len(runs); n > 0 && at.Sub(runs[n-1].last) < gap {
			runs[n-1].last, runs[n-1].n = at, runs[n-1].n+1
			continue
		}
		runs = append(runs, run{first: at, last: at, n: 1})
	}
	var eps []run
	for _, r := range runs {
		if r.n < minEvents {
			continue
		}
		if n := len(eps); n > 0 && r.first.Sub(eps[n-1].last) < mergeGap {
			eps[n-1].last, eps[n-1].n = r.last, eps[n-1].n+r.n
			continue
		}
		eps = append(eps, r)
	}
	for _, r := range runs {
		runLens = append(runLens, r.n)
	}
	return len(eps), runLens
}

func fixtureSequence(vs []fixtureVerdict, item string) string {
	letters := map[string]string{"FAIL": "F", "PARTIAL": "R", "UNVERIFIABLE": "U", "PASS": "P"}
	var sb strings.Builder
	for _, v := range vs {
		if v.ItemID == item {
			sb.WriteString(letters[v.OverallOutcome])
		}
	}
	return sb.String()
}

func TestFixtureEventsThrash_should_ReproduceLiveThrashShapes_When_Loaded(t *testing.T) {
	t.Parallel()
	f := loadFixture[fixtureEvents](t, "events_thrash.json")

	t.Run("thrash-a has two ready/in_progress runs of 217 and 62 one-minute events", func(t *testing.T) {
		t.Parallel()
		evs := f.forItem("thrash-a")
		require.Len(t, evs, 279)
		for _, e := range evs {
			assert.Contains(t, []string{"ready", "in_progress"}, e.From)
			assert.Contains(t, []string{"ready", "in_progress"}, e.To)
		}
		_, runLens := fixtureEpisodes(t, evs, 15*time.Minute, 6, time.Hour)
		assert.Equal(t, []int{217, 62}, runLens)
		for i := 1; i < len(evs); i++ {
			d := evs[i].instant(t).Sub(evs[i-1].instant(t))
			if i == 217 {
				assert.Greater(t, d, time.Hour)
				assert.Less(t, d, 6*time.Hour)
				continue
			}
			assert.Equal(t, time.Minute, d, "event %d", i)
		}
		for _, c := range []struct {
			merge time.Duration
			want  int
		}{{15 * time.Minute, 2}, {time.Hour, 2}, {6 * time.Hour, 1}} {
			got, _ := fixtureEpisodes(t, evs, 15*time.Minute, 6, c.merge)
			assert.Equal(t, c.want, got, "merge gap %s", c.merge)
		}
	})

	t.Run("thrash-b has 17 episodes at 15m merging to 5 at 1h", func(t *testing.T) {
		t.Parallel()
		evs := f.forItem("thrash-b")
		for _, e := range evs {
			assert.Empty(t, e.Note, "live a90b99cb has empty notes")
			assert.Contains(t, []string{"pr_pending", "in_progress"}, e.From)
		}
		at15, _ := fixtureEpisodes(t, evs, 15*time.Minute, 6, 0)
		at1h, _ := fixtureEpisodes(t, evs, 15*time.Minute, 6, time.Hour)
		assert.Equal(t, 17, at15)
		assert.Equal(t, 5, at1h)
	})

	t.Run("mixed offsets, self-loops and real review bounces", func(t *testing.T) {
		t.Parallel()
		var minus7, plus0, selfLoops, bounces int
		for _, e := range f.Events {
			switch {
			case strings.HasSuffix(e.At, "-07:00"):
				minus7++
			case strings.HasSuffix(e.At, "+00:00"):
				plus0++
			}
			if e.From == e.To {
				selfLoops++
			}
			if e.From == "review" && e.To == "in_progress" {
				bounces++
			}
		}
		assert.Equal(t, 4, minus7)
		assert.Positive(t, plus0)
		assert.Equal(t, 2, selfLoops)
		assert.Equal(t, 17, bounces)
	})

	t.Run("seq is a strict ordering key and instants are non-decreasing", func(t *testing.T) {
		t.Parallel()
		for i := 1; i < len(f.Events); i++ {
			assert.Greater(t, f.Events[i].Seq, f.Events[i-1].Seq)
			assert.False(t, f.Events[i].instant(t).Before(f.Events[i-1].instant(t)), "seq %d", f.Events[i].Seq)
		}
	})
}

func TestFixtureVerdicts_should_ReproduceLiveVerdictShapes_When_Loaded(t *testing.T) {
	t.Parallel()
	vs := loadFixture[fixtureVerdicts](t, "verdicts.json").Verdicts

	assert.Equal(t, "FRUUURURRURRRP", fixtureSequence(vs, "0791f5b4"))
	assert.Equal(t, "FFFFFFFFFRUFRFFFFFFFFUFP", fixtureSequence(vs, "5eaaa473"))

	var lastOfOverridden fixtureVerdict
	for _, v := range vs {
		if v.ItemID == "5eaaa473" {
			lastOfOverridden = v
		}
	}
	assert.Equal(t, "PASS", lastOfOverridden.OverallOutcome)
	assert.NotEmpty(t, lastOfOverridden.OverrideBy, "5eaaa473 ends in a human override")

	count := func(outcome, prefix string) int {
		n := 0
		for _, v := range vs {
			if v.OverallOutcome == outcome && strings.HasPrefix(v.Summary, prefix) {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 1, count("FAIL", "Review blocked: no committed changes"))
	assert.Equal(t, 1, count("UNVERIFIABLE", "Review UNVERIFIABLE: codebase-read capability self-check failed"))
	assert.Equal(t, 1, count("UNVERIFIABLE", "Review blocked: no diff could be computed"))
	assert.Equal(t, 1, count("PASS", "Auto-verified by reconcileBouncingItems"))

	sessions := map[string]bool{}
	for _, v := range vs {
		assert.False(t, sessions[v.SessionID], "one verdict per session: %s", v.SessionID)
		sessions[v.SessionID] = true
	}

	byItem := map[string][]fixtureVerdict{}
	for _, v := range vs {
		byItem[v.ItemID] = append(byItem[v.ItemID], v)
	}
	failing := func(v fixtureVerdict) map[int]bool {
		out := map[int]bool{}
		for _, c := range v.PerCriterion {
			if c.Outcome != "PASS" {
				out[c.CriterionIndex] = true
			}
		}
		return out
	}
	var repeatedCriterionItems, unchangedDiffItems []string
	for item, list := range byItem {
		for i := 0; i+2 < len(list); i++ {
			for c := range failing(list[i]) {
				if failing(list[i+1])[c] && failing(list[i+2])[c] {
					repeatedCriterionItems = append(repeatedCriterionItems, item)
				}
			}
		}
		for i := 0; i+1 < len(list); i++ {
			if list[i].DiffHash != "" && list[i].DiffHash == list[i+1].DiffHash {
				unchangedDiffItems = append(unchangedDiffItems, item)
			}
		}
	}
	sort.Strings(repeatedCriterionItems)
	assert.Equal(t, []string{"rep-crit"}, repeatedCriterionItems, "exactly one run repeats a failing criterion in 3 consecutive verdicts")
	assert.Equal(t, []string{"same-diff"}, unchangedDiffItems, "exactly one run keeps a non-empty diff_hash between consecutive verdicts")
}

func TestFixtureStuck_should_LoadClosedReasonIntervals_When_Loaded(t *testing.T) {
	t.Parallel()
	s := loadFixture[fixtureStuck](t, "stuck.json")
	require.NotEmpty(t, s.Intervals)

	var open int
	reasons := map[string]bool{}
	for _, iv := range s.Intervals {
		first, err := time.Parse(fixtureTimeLayout, iv.FirstDetectedAt)
		require.NoError(t, err)
		reasons[iv.Reason] = true
		if iv.ResolvedAt == nil {
			open++
			continue
		}
		resolved, err := time.Parse(fixtureTimeLayout, *iv.ResolvedAt)
		require.NoError(t, err)
		assert.True(t, resolved.After(first), "%s resolves after it opens", iv.Reason)
	}
	assert.Equal(t, 2, open)
	assert.True(t, reasons["multiple_reasons"])
	assert.GreaterOrEqual(t, len(reasons), 5)
}
