package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

const cleanBEP = `{"id":{"started":{}},"started":{"uuid":"a"}}
{"progress":{"stderr":"INFO: Build completed successfully\n"}}
{"id":{"buildMetrics":{}},"buildMetrics":{"actionSummary":{"runnerCount":[{"name":"remote cache hit","count":"6"},{"name":"linux-sandbox","count":"3"},{"name":"internal","count":"50"}]},"timingMetrics":{"wallTimeInMs":"12000"}}}
`

const authBEP = `{"progress":{"stderr":"WARNING: Error reading from the remote cache:\nHTTP 403 Forbidden\n"}}
{"id":{"buildMetrics":{}},"buildMetrics":{"actionSummary":{"runnerCount":[{"name":"disk cache hit","count":"1"},{"name":"linux-sandbox","count":"3"}]},"timingMetrics":{"wallTimeInMs":"8000"}}}
`

func summarize(t *testing.T, files ...string) Summary {
	t.Helper()
	var s Summary
	for _, f := range files {
		if err := s.ParseBEP(strings.NewReader(f)); err != nil {
			t.Fatal(err)
		}
	}
	s.Finish()
	return s
}

func TestCleanFixtureHasZeroAuthFailures(t *testing.T) {
	s := summarize(t, cleanBEP)
	if s.AuthFailures != 0 || !strings.Contains(s.Prom(), "bazel_cache_auth_failures_total 0\n") {
		t.Fatalf("%+v\n%s", s, s.Prom())
	}
	if s.TimeToGreenS != 12 || s.CacheHitRatio != 6.0/9.0 || s.Attempts != 1 || s.EvictionRetries != 0 {
		t.Fatalf("%+v", s)
	}
	if strings.Contains(s.Prom(), "::warning") {
		t.Fatal("no warning expected")
	}
}

func TestAuthFixturePrintsOneAndWarns(t *testing.T) {
	s := summarize(t, authBEP)
	out := s.Prom()
	if !strings.Contains(out, "bazel_cache_auth_failures_total 1\n") || !strings.Contains(out, "::warning") {
		t.Fatalf("%s", out)
	}
}

func TestRetriesSumAttempts(t *testing.T) {
	s := summarize(t, authBEP, cleanBEP)
	if s.Attempts != 2 || s.EvictionRetries != 1 || s.TimeToGreenS != 20 {
		t.Fatalf("%+v", s)
	}
}

func TestLogScanCountsGrpcSignatures(t *testing.T) {
	var s Summary
	if err := s.AddLog(strings.NewReader("ok\nUNAUTHENTICATED: bad\nPERMISSION_DENIED x\nnothing\n")); err != nil {
		t.Fatal(err)
	}
	if s.AuthFailures != 2 {
		t.Fatalf("got %d", s.AuthFailures)
	}
}

func TestMalformedBEPIsError(t *testing.T) {
	var s Summary
	if err := s.ParseBEP(strings.NewReader("{not json\n")); err == nil {
		t.Fatal("want error")
	}
}

func TestMedianLastN(t *testing.T) {
	if m, _ := Median([]float64{9, 1, 2, 3}, 3); m != 2 {
		t.Fatalf("got %v", m)
	}
	if _, ok := Median(nil, 3); ok {
		t.Fatal("empty has no median")
	}
}

func TestHitRatioFloorOnlyWarmMain(t *testing.T) {
	rows := []TrendRow{
		{Ref: "refs/heads/main", Warm: true, Summary: Summary{CacheHitRatio: 0.7}},
		{Ref: "refs/heads/main", Warm: true, Summary: Summary{CacheHitRatio: 0.5}},
		{Ref: "refs/heads/x", Warm: true, Summary: Summary{CacheHitRatio: 0.0}},
		{Ref: "refs/heads/main", Warm: false, Summary: Summary{CacheHitRatio: 0.0}},
	}
	m, ok, enough := HitRatioFloor(rows, 2, 0.6)
	if m != 0.6 || !ok || !enough {
		t.Fatalf("%v %v %v", m, ok, enough)
	}
	if _, _, enough := HitRatioFloor(rows, 20, 0.6); enough {
		t.Fatal("two rows are not enough for n=20")
	}
}

func TestRunJSONOutput(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/bep.json"
	if err := writeFile(p, cleanBEP); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"-affected-target-count", "7", "-append", dir + "/trend.jsonl", "-run-id", "1", p}, &out, &errb); code != 0 {
		t.Fatalf("%d %s", code, errb.String())
	}
	for _, k := range []string{`"time_to_green_s":12`, `"cache_hit_ratio"`, `"affected_target_count":7`} {
		if !strings.Contains(out.String(), k) {
			t.Fatalf("missing %s in %s", k, out.String())
		}
	}
}

func writeFile(p, s string) error { return os.WriteFile(p, []byte(s), 0o644) }
