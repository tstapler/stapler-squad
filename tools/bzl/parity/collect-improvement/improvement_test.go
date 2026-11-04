package main

import (
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

type fakeReplay struct{ sel map[string][]string }

func (f fakeReplay) TotalTests() (int, error) { return 100, nil }
func (f fakeReplay) Select(ns string) ([]string, error) {
	for k, v := range f.sel {
		if strings.Contains(ns, k) {
			return v, nil
		}
	}
	return []string{"//...", "job:x"}, nil
}

func TestT1MedianOfReplayRatios(t *testing.T) {
	s := PRSample{}
	s.Partial = append(s.Partial, struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	}{1, []string{"a.go"}}, struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	}{2, []string{"b.go"}}, struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	}{3, []string{"gone.go"}})
	f := fakeReplay{sel: map[string][]string{"M\ta.go": {"//a:t", "//b:t"}, "M\tb.go": {"//a:t", "job:lint", "//c:t", "//d:t", "//e:t", "//f:t"}, "D\tgone.go": {"//...", "job:x"}}}
	d := collect.NewDoc()
	if err := MeasureT1(d, f, s, func(p string) bool { return p != "gone.go" }); err != nil {
		t.Fatal(err)
	}
	// ratios 0.02, 0.05 and 1.0 (a //... selection is the whole graph): median 0.05
	if v, ok := d.Value("T1"); !ok || v != 0.05 {
		t.Fatalf("%v %v", v, ok)
	}
}

func TestT1ReplayErrorPropagates(t *testing.T) {
	d := collect.NewDoc()
	s := PRSample{}
	s.Partial = append(s.Partial, struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	}{1, nil})
	if err := MeasureT1(d, errReplay{}, s, func(string) bool { return true }); err == nil {
		t.Fatal("expected error")
	}
}

type errReplay struct{}

func (errReplay) TotalTests() (int, error)        { return 1, nil }
func (errReplay) Select(string) ([]string, error) { return nil, errors.New("bazel failed") }

func TestT2ClassesFromFixture(t *testing.T) {
	gh, err := ghci.LoadFake("testdata/gh.json")
	if err != nil {
		t.Fatal(err)
	}
	d := collect.NewDoc()
	if err := MeasureT2(d, gh, "o/r", []string{"bazel.yml", "lint.yml"}, 30, 1, `^(MODULE\.bazel|go\.mod)$`); err != nil {
		t.Fatal(err)
	}
	// aaa: ttg 900 s, 20 runner-min; bbb (global, MODULE.bazel): ttg 1200 s, 25 runner-min
	want := map[string]float64{"T2-partial-ttg": 900, "T2-partial-runner-min": 20, "T2-all-ttg": 1050, "T2-all-runner-min": 22.5}
	for k, w := range want {
		if v, ok := d.Value(k); !ok || v != w {
			t.Errorf("%s = %v (%v), want %v", k, v, ok, w)
		}
	}
	d = collect.NewDoc()
	_ = MeasureT2(d, gh, "o/r", []string{"bazel.yml", "lint.yml"}, 30, 20, `^MODULE\.bazel$`)
	if _, ok := d.Value("T2-all-ttg"); ok || !strings.Contains(d.Meta("T2-all-ttg").Reason, "need 20") {
		t.Fatal("fewer than 20 runs must be UNMEASURED")
	}
}

func TestT3QuietAndNoisy(t *testing.T) {
	base, _ := collect.LoadWorktreeBaseline("testdata/baseline-worktree.json")
	g := collect.NewGate(constHost(0.2), true)
	q, _ := collect.LoadS6("testdata/s6-quiet.json")
	d := collect.NewDoc()
	MeasureT3(d, q, base, g, "fixture")
	if v, ok := d.Value("T3-second-cold"); !ok || v != 15.0/200 {
		t.Fatal(v, ok)
	}
	n, _ := collect.LoadS6("testdata/s6-noisy.json")
	d = collect.NewDoc()
	MeasureT3(d, n, base, g, "fixture")
	if _, ok := d.Value("T3-second-cold"); ok {
		t.Fatal("noisy result must be UNMEASURED")
	}
}

type fakeEdit struct {
	edits    int
	restored bool
	d        time.Duration
}

func (f *fakeEdit) Edit(int) error               { f.edits++; return nil }
func (f *fakeEdit) Test() (time.Duration, error) { return f.d, nil }
func (f *fakeEdit) Restore() error               { f.restored = true; return nil }

func TestT4MedianAndRestore(t *testing.T) {
	f := &fakeEdit{d: 4 * time.Second}
	d := collect.NewDoc()
	MeasureT4(d, f, collect.NewGate(constHost(0.3), true), 5, "src")
	if v, ok := d.Value("T4"); !ok || v != 4 || !f.restored || f.edits != 6 {
		t.Fatalf("v=%v ok=%v restored=%v edits=%d", v, ok, f.restored, f.edits)
	}
}

func TestT4NoisyHostNeverTouchesFile(t *testing.T) {
	f := &fakeEdit{d: time.Second}
	d := collect.NewDoc()
	MeasureT4(d, f, collect.NewGate(constHost(9), true), 5, "src")
	if _, ok := d.Value("T4"); ok || f.edits != 0 || !strings.Contains(d.Meta("T4").Reason, "NOT QUIET") {
		t.Fatalf("edits=%d meta=%+v", f.edits, d.Meta("T4"))
	}
}

func TestFileEditRunnerRestoresOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.go")
	if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &FileEditRunner{Path: p, Dir: t.TempDir(), Argv: []string{"true"}}
	MeasureT4(collect.NewDoc(), r, collect.NewGate(constHost(0.1), true), 2, "")
	if b, _ := os.ReadFile(p); string(b) != "package x\n" {
		t.Fatalf("file not restored: %q", b)
	}
}

func TestT6CountsAndIsUnmeasuredPreFlip(t *testing.T) {
	d := collect.NewDoc()
	if err := MeasureT6(d, "testdata/transcripts/*.jsonl", time.Time{}, 30, 0); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Value("T6"); ok {
		t.Fatal("pre-flip must be UNMEASURED")
	}
	s, err := ScanTranscript("testdata/transcripts/s1.jsonl")
	if err != nil || s.Steps != 2 || !s.BuildActive {
		t.Fatalf("%+v %v", s, err)
	}
	if err := MeasureT6(d, "testdata/transcripts/*.jsonl", time.Unix(0, 0), 30, 0); err != nil {
		t.Fatal(err)
	}
	// sessions: s1 = 2 steps, s2 = 0 -> median 1; degenerate-baseline note is recorded
	if v, ok := d.Value("T6"); !ok || v != 1 || !strings.Contains(d.Meta("T6").Source, "DEGENERATE") {
		t.Fatalf("%v %v %+v", v, ok, d.Meta("T6"))
	}
}

func TestT7FoldsViolationsIntoValue(t *testing.T) {
	b, _ := loadHermeticity("testdata/hermeticity-bazel.json")
	m, _ := loadHermeticity("testdata/hermeticity-make.json")
	g := collect.NewGate(constHost(0.1), true)
	d := collect.NewDoc()
	MeasureT7(d, b, m, map[string]bool{"/bin/sh": true, "/usr/bin/gcc": true, "/usr/bin/ld": true, "/usr/bin/as": true}, 0, g, "fixture")
	// bazel: 0 host executables after the allow-list; make: 7 after excluding /bin/sh only -> 0.0, no violations
	if v, ok := d.Value("T7"); !ok || v != 0 {
		t.Fatalf("%v %v", v, ok)
	}
	b.H1Network, b.H2OK = 2, false
	d = collect.NewDoc()
	MeasureT7(d, b, m, nil, 0, g, "fixture")
	if v, _ := d.Value("T7"); v < 2 { // ratio 4/8 + two violated clauses
		t.Fatalf("violations must push T7 above any ratio target: %v", v)
	}
}

func TestT7RefusesWithoutRecordedQuietLoad(t *testing.T) {
	b, _ := loadHermeticity("testdata/hermeticity-bazel.json")
	m, _ := loadHermeticity("testdata/hermeticity-make.json")
	g := collect.NewGate(constHost(0.1), true)
	loud := 4.0
	b.LoadAvg1End = &loud
	d := collect.NewDoc()
	MeasureT7(d, b, m, nil, 0, g, "fixture")
	if _, ok := d.Value("T7"); ok {
		t.Fatal("end load 4.0 must void T7")
	}
	b.LoadAvg1, b.LoadAvg1End = nil, nil
	d = collect.NewDoc()
	MeasureT7(d, b, m, nil, 0, g, "fixture")
	if _, ok := d.Value("T7"); ok || !strings.Contains(d.Meta("T7").Reason, "NOT QUIET") {
		t.Fatal("missing load record must be UNMEASURED")
	}
}

func TestT8Wiring(t *testing.T) {
	pass := "measured_prs=5 baseline_median_min=113.0 selective_median_min=40.0 ratio=0.35 e2e_escapes=0 \nT8: PASS\n"
	d := collect.NewDoc()
	MeasureT8(d, pass)
	if v, _ := d.Value("T8"); v != 0.35 {
		t.Fatal(v)
	}
	d = collect.NewDoc()
	MeasureT8(d, strings.Replace(strings.Replace(pass, "e2e_escapes=0", "e2e_escapes=2", 1), "PASS", "FAIL", 1))
	if v, _ := d.Value("T8"); v != 2.35 {
		t.Fatalf("escapes must exceed the target: %v", v)
	}
	d = collect.NewDoc()
	MeasureT8(d, "measured_prs=0 baseline_median_min=0.0 selective_median_min=0.0 ratio=0.00 e2e_escapes=0 no sampled PR\nT8: UNMEASURED\n")
	if _, ok := d.Value("T8"); ok {
		t.Fatal("UNMEASURED stays unmeasured")
	}
}

func TestRunSubcommandsEndToEnd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "m.json")
	var so, se strings.Builder
	if c := run([]string{"t3", "-s6", "testdata/s6-quiet.json", "-baseline-worktree", "testdata/baseline-worktree.json", "-out", out}, &so, &se, constHost(0.1), nil); c != 0 {
		t.Fatal(c, se.String())
	}
	n, err := collect.Numbers(out)
	if err != nil || n["T3-second-cold"] != 0.075 {
		t.Fatalf("%v %v", n, err)
	}
	if run([]string{"bogus"}, &so, &se, constHost(0), nil) != 2 {
		t.Fatal("unknown subcommand")
	}
}
