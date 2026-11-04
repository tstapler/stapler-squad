package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const metricsA = "# HELP a_total x\n# TYPE a_total counter\na_total 1\nb_seconds_bucket{le=\"1\"} 2\nb_seconds_bucket{le=\"2\"} 3\n"

func write(t *testing.T, dir, name, body string) string {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMetricNamesSortedUnique(t *testing.T) {
	got := MetricNames(strings.NewReader(metricsA))
	if strings.Join(got, ",") != "a_total,b_seconds_bucket" {
		t.Fatalf("%v", got)
	}
}

func TestPassCountNeedsStats(t *testing.T) {
	if n, err := PassCount(strings.NewReader(`{"stats":{"expected":42}}`)); err != nil || n != 42 {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := PassCount(strings.NewReader(`{}`)); err == nil {
		t.Fatal("a report without stats must be an error, not zero")
	}
}

func runWith(t *testing.T, nb, nm, mb, mm string) (int, string) {
	d := t.TempDir()
	var out, errb bytes.Buffer
	code := run([]string{
		"-bazel-report", write(t, d, "b.json", nb), "-make-report", write(t, d, "m.json", nm),
		"-bazel-metrics", write(t, d, "b.txt", mb), "-make-metrics", write(t, d, "m.txt", mm)}, &out, &errb)
	return code, out.String() + errb.String()
}

func TestEqualPasses(t *testing.T) {
	code, out := runWith(t, `{"stats":{"expected":9}}`, `{"stats":{"expected":9}}`, metricsA, metricsA)
	if code != 0 || !strings.Contains(out, "e2e_pass_bazel=9 e2e_pass_make=9 metrics_equal=true") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestCountMismatchAndMetricDriftFail(t *testing.T) {
	if code, _ := runWith(t, `{"stats":{"expected":9}}`, `{"stats":{"expected":8}}`, metricsA, metricsA); code != 1 {
		t.Fatal("count mismatch must fail")
	}
	code, out := runWith(t, `{"stats":{"expected":9}}`, `{"stats":{"expected":9}}`, metricsA+"extra_total 1\n", metricsA)
	if code != 1 || !strings.Contains(out, "metric only in bazel build: extra_total") {
		t.Fatalf("%d %s", code, out)
	}
}

func TestZeroPassesFailEvenIfEqual(t *testing.T) {
	if code, _ := runWith(t, `{"stats":{"expected":0}}`, `{"stats":{"expected":0}}`, metricsA, metricsA); code != 1 {
		t.Fatal("an empty suite proves nothing")
	}
}
