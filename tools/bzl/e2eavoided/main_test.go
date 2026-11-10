package main

import (
	"bytes"
	"strings"
	"testing"
)

func verdictLine(t *testing.T, fixture string) (int, string) {
	t.Helper()
	var out, errw bytes.Buffer
	code := run([]string{"-fixture", fixture}, &out, &errw)
	return code, out.String()
}

func TestFixtures(t *testing.T) {
	for fixture, want := range map[string]string{
		"testdata/t8-pass.json":       "T8: PASS",
		"testdata/t8-escape.json":     "T8: FAIL",
		"testdata/t8-unmeasured.json": "T8: UNMEASURED",
	} {
		code, out := verdictLine(t, fixture)
		if strings.Count(out, want+"\n") != 1 {
			t.Errorf("%s: want one %q line, got:\n%s", fixture, want, out)
		}
		if (want == "T8: PASS") != (code == 0) {
			t.Errorf("%s: exit code %d", fixture, code)
		}
	}
}

func TestRatioAboveTargetFails(t *testing.T) {
	a, b := 100.0, 80.0
	v := evaluate(Input{Samples: []Sample{{PR: 1, BaselineFullMin: &a, SelectiveMin: &b}}}, 0.5)
	if v.Status != "FAIL" || v.Ratio != 0.8 {
		t.Fatalf("%+v", v)
	}
}

func TestShardMinutesSumsMatchingJobs(t *testing.T) {
	js := `{"jobs":[
	 {"name":"e2e (Bun build) shard 1","started_at":"2026-11-03T00:00:00Z","completed_at":"2026-11-03T00:30:00Z"},
	 {"name":"e2e (Bun build) shard 2","started_at":"2026-11-03T00:00:00Z","completed_at":"2026-11-03T00:15:00Z"},
	 {"name":"lint","started_at":"2026-11-03T00:00:00Z","completed_at":"2026-11-03T01:00:00Z"},
	 {"name":"e2e (Bun build) shard 3","started_at":"2026-11-03T00:00:00Z","completed_at":""}]}`
	m, err := shardMinutes([]byte(js), "e2e (Bun build) shard")
	if err != nil || m != 45 {
		t.Fatalf("m=%v err=%v", m, err)
	}
}

func TestCountEscapes(t *testing.T) {
	n, err := countEscapes([]byte(`[{"spec":"a","selected":false},{"spec":"b","selected":true},{"spec":"c"}]`))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
