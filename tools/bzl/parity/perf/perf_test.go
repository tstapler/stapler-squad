package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"github.com/tstapler/stapler-squad/tools/bzl/parity/ghci"
)

type constHost float64

func (c constHost) Load1() (float64, error) { return float64(c), nil }

type fakeBuilder struct {
	calls int
	d     time.Duration
	err   error
}

func (f *fakeBuilder) Build(string) (time.Duration, error) { f.calls++; return f.d, f.err }

func TestWarmQuietUsesMedianOfFive(t *testing.T) {
	b := &fakeBuilder{d: 3 * time.Second}
	d := collect.NewDoc()
	MeasureWarm(d, b, collect.NewGate(constHost(0.4), true), "//:x", 5)
	if v, ok := d.Value("warm_noop_s"); !ok || v != 3 || b.calls != 6 {
		t.Fatalf("v=%v ok=%v calls=%d (1 priming + 5 timed)", v, ok, b.calls)
	}
}

func TestWarmRefusesOnNoisyHostWithoutBuilding(t *testing.T) {
	b := &fakeBuilder{d: time.Second}
	d := collect.NewDoc()
	MeasureWarm(d, b, collect.NewGate(constHost(6), true), "//:x", 5)
	if _, ok := d.Value("warm_noop_s"); ok || b.calls != 0 {
		t.Fatalf("a noisy host must not be loaded nor yield a number (calls=%d)", b.calls)
	}
	if m := d.Meta("warm_noop_s"); !strings.Contains(m.Reason, "NOT QUIET") {
		t.Fatalf("%+v", m)
	}
}

func TestWarmBuildFailureIsUnmeasured(t *testing.T) {
	d := collect.NewDoc()
	MeasureWarm(d, &fakeBuilder{err: errors.New("boom")}, collect.NewGate(constHost(0.4), true), "//:x", 5)
	if _, ok := d.Value("warm_noop_s"); ok {
		t.Fatal("failed build must not yield a number")
	}
}

func TestDiskRatioAgainstBaseline(t *testing.T) {
	var base collect.WorktreeBaseline
	base.DiskKB.Median = 1000
	d := collect.NewDoc()
	MeasureDisk(d, func(string) (int64, error) { return 1200, nil }, "/ob", base)
	if r, _ := d.Value("disk_ratio"); r != 1.2 {
		t.Fatal(r)
	}
	MeasureDisk(d, func(string) (int64, error) { return 0, errors.New("nope") }, "/ob", base)
	if _, ok := d.Value("output_base_disk_kb"); ok {
		t.Fatal("du error must be UNMEASURED")
	}
}

func TestWorktreeQuietAndNoisy(t *testing.T) {
	base, _ := collect.LoadWorktreeBaseline("testdata/baseline-worktree.json")
	g := collect.NewGate(constHost(0.1), true)
	q, _ := collect.LoadS6("testdata/s6-quiet.json")
	d := collect.NewDoc()
	MeasureWorktree(d, q, base, g, "fixture")
	if r, ok := d.Value("second_cold_ratio_make"); !ok || r < 0.26 || r > 0.28 {
		t.Fatalf("%v %v", r, ok)
	}
	if r, _ := d.Value("second_cold_ratio_first"); r != 15.0/200 {
		t.Fatal(r)
	}
	n, _ := collect.LoadS6("testdata/s6-noisy.json")
	d = collect.NewDoc()
	MeasureWorktree(d, n, base, g, "fixture")
	if _, ok := d.Value("second_cold_s"); ok || !strings.Contains(d.Meta("second_cold_s").Reason, "NOT QUIET") {
		t.Fatalf("noisy s6 result must be UNMEASURED: %+v", d.Meta("second_cold_s"))
	}
}

func TestTTGFromFixtureGH(t *testing.T) {
	gh, err := ghci.LoadFake("testdata/gh.json")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := collect.LoadMeasured("testdata/measured.json")
	d := collect.NewDoc()
	if err := MeasureTTG(d, gh, "o/r", []string{"bazel.yml", "lint.yml"}, 30, base); err != nil {
		t.Fatal(err)
	}
	if v, _ := d.Value("ttg_p50_s"); v != 900 {
		t.Fatal(v)
	}
	if v, _ := d.Value("ttg_ratio"); v != 1.5 {
		t.Fatal(v)
	}
	// lint.yml p50 600 vs 500 baseline = 1.2, bazel.yml 600 vs 500 = 1.2
	if v, ok := d.Value("ttg_wf_ratio_max"); !ok || v < 1.19 || v > 1.21 {
		t.Fatal(v, ok)
	}
}

func perfOf(t *testing.T, build func(d *collect.Doc)) perfFile {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.json")
	d := collect.NewDoc()
	build(d)
	if err := collect.Merge(p, d); err != nil {
		t.Fatal(err)
	}
	pf, err := loadPerf(p)
	if err != nil {
		t.Fatal(err)
	}
	return pf
}

func TestCheckPassFailUnmeasured(t *testing.T) {
	pf := perfOf(t, func(d *collect.Doc) {
		d.Set("warm_noop_s", 4, collect.Meta{})
		d.Set("second_cold_ratio_make", 0.5, collect.Meta{})
	})
	var out bytes.Buffer
	if c := Check(pf, []string{"warm"}, &out); c != exitPass {
		t.Fatal(c, out.String())
	}
	if c := Check(pf, []string{"cold"}, &out); c != exitFail {
		t.Fatal(c, out.String())
	}
	if c := Check(pf, []string{"disk"}, &out); c != exitUnmeasured {
		t.Fatal(c, out.String())
	}
	if c := Check(pf, []string{"all"}, &out); c != exitFail {
		t.Fatal("FAIL beats UNMEASURED:", c)
	}
}

func TestCheckRefusesRowMarkedNotQuiet(t *testing.T) {
	pf := perfOf(t, func(d *collect.Doc) {
		q := false
		d.Set("warm_noop_s", 1, collect.Meta{Quiet: &q}) // a number that slipped in with quiet=false
	})
	var out bytes.Buffer
	if c := Check(pf, []string{"warm"}, &out); c != exitUnmeasured || !strings.Contains(out.String(), "NOT QUIET") {
		t.Fatal(c, out.String())
	}
}

func TestTTGCheckNeedsTwentySamples(t *testing.T) {
	few := perfOf(t, func(d *collect.Doc) {
		d.Set("ttg_n", 5, collect.Meta{})
		d.Set("ttg_ratio", 0.9, collect.Meta{})
		d.Set("ttg_wf_ratio_max", 0.9, collect.Meta{})
	})
	var out bytes.Buffer
	if c := Check(few, []string{"ttg"}, &out); c != exitUnmeasured {
		t.Fatal(c, out.String())
	}
	enough := perfOf(t, func(d *collect.Doc) {
		d.Set("ttg_n", 25, collect.Meta{})
		d.Set("ttg_ratio", 0.9, collect.Meta{})
		d.Set("ttg_wf_ratio_max", 1.2, collect.Meta{})
	})
	if c := Check(enough, []string{"ttg"}, &out); c != exitFail {
		t.Fatal(c, out.String())
	}
}

func TestRunCheckMissingFileIsUnmeasured(t *testing.T) {
	var out, errb bytes.Buffer
	if c := run([]string{"check", "-perf", filepath.Join(t.TempDir(), "absent.json")}, &out, &errb, constHost(0), nil); c != exitUnmeasured {
		t.Fatal(c)
	}
}

func TestRunWarmOnNoisyHostWritesUnmeasuredRow(t *testing.T) {
	out := filepath.Join(t.TempDir(), "p.json")
	var so, se bytes.Buffer
	if c := run([]string{"warm", "-out", out}, &so, &se, constHost(7), nil); c != 0 {
		t.Fatal(c, se.String())
	}
	b, _ := os.ReadFile(out)
	n, _ := collect.Numbers(out)
	if _, has := n["warm_noop_s"]; has || !strings.Contains(string(b), "NOT QUIET") {
		t.Fatalf("%s", b)
	}
	if !strings.Contains(so.String(), "UNMEASURED") {
		t.Fatal(so.String())
	}
}

func TestRunWorktreeFromFixtureEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "p.json")
	var so, se bytes.Buffer
	args := []string{"worktree", "-s6", "testdata/s6-quiet.json", "-baseline-worktree", "testdata/baseline-worktree.json", "-out", out}
	if c := run(args, &so, &se, constHost(0.2), nil); c != 0 {
		t.Fatal(c, se.String())
	}
	pf, err := loadPerf(out)
	if err != nil {
		t.Fatal(err)
	}
	if c := Check(pf, []string{"cold"}, &so); c != exitPass {
		t.Fatal(c, so.String())
	}
}
