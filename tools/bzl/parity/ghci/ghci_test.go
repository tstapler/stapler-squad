package ghci

import (
	"math"
	"testing"
)

func fixture(t *testing.T) *FakeGH {
	t.Helper()
	f, err := LoadFake("testdata/gh.json")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestTimeToGreenGroupsBySHAAndSkipsRed(t *testing.T) {
	runs, err := FetchRuns(fixture(t), "o/r", "pull_request", 3)
	if err != nil {
		t.Fatal(err)
	}
	got := TimeToGreen(runs, []string{"bazel.yml", "lint.yml"}, 0)
	if len(got) != 1 {
		t.Fatalf("want only the all-green SHA, got %d: %+v", len(got), got)
	}
	s := got[0]
	if s.HeadSHA != "aaa" || s.TTGS != 900 || s.PerWF["bazel.yml"] != 600 || len(s.PRs) != 1 || s.PRs[0] != 7 {
		t.Fatalf("%+v", s)
	}
}

func TestLatestRunPerWorkflowWins(t *testing.T) {
	runs, _ := FetchRuns(fixture(t), "o/r", "pull_request", 3)
	// sha "aaa" has a failed run_number 1 and a green run_number 2 of lint.yml; only the latest counts.
	got := TimeToGreen(runs, []string{"lint.yml"}, 0)
	if len(got) != 1 || got[0].HeadSHA != "aaa" || got[0].TTGS != 600 {
		t.Fatalf("%+v", got)
	}
}

func TestRunnerMinutes(t *testing.T) {
	m, err := RunnerMinutes(fixture(t), "o/r", []int64{1})
	if err != nil || math.Abs(m-15) > 1e-9 {
		t.Fatalf("%v %v", m, err)
	}
}

func TestMissingFixtureResponseIsError(t *testing.T) {
	if _, err := fixture(t).API("nope"); err == nil {
		t.Fatal("expected error")
	}
}
