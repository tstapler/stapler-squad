package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMedian(t *testing.T) {
	if got := median([]float64{5, 1, 3}); got != 3 {
		t.Fatalf("odd: %v", got)
	}
	if got := median([]float64{4, 1, 3, 2}); got != 2.5 {
		t.Fatalf("even: %v", got)
	}
	if !math.IsNaN(median(nil)) {
		t.Fatal("empty must be NaN")
	}
}

func TestRatio(t *testing.T) {
	if r := Ratio([]string{"//a:t", "//b:t", "job:lint"}, 10); r != 0.2 {
		t.Fatalf("jobs must not count: %v", r)
	}
	if r := Ratio([]string{"//..."}, 10); r != 1 {
		t.Fatalf("whole graph: %v", r)
	}
	if r := Ratio(nil, 10); r != 0 {
		t.Fatalf("empty: %v", r)
	}
	if !math.IsNaN(Ratio([]string{"//a:t"}, 0)) {
		t.Fatal("zero total must be NaN")
	}
}

func TestVerdictsMirrorEconomicGate(t *testing.T) {
	cases := []struct {
		name string
		got  Verdict
		pass bool
	}{
		{"t1 clear", EvalT1(0.2, 0.2587), true},
		{"t1 trips cap", EvalT1(0.31, 0.2587), false},
		{"t1 trips 1.5x ra1", EvalT1(0.1, 0.05), false},
		{"t3 clear", EvalT3(100, 400), true},
		{"t3 trips", EvalT3(101, 400), false},
		{"t4 clear slack", EvalT4(7.9, 3), true},
		{"t4 trips", EvalT4(8.5, 3), false},
		{"t4 factor", EvalT4(12, 10), true},
		{"unmeasured is FAIL", EvalT3(math.NaN(), 400), false},
	}
	for _, c := range cases {
		if c.got.Pass != c.pass {
			t.Errorf("%s: pass=%v detail=%s", c.name, c.got.Pass, c.got.Detail)
		}
	}
	if (Verdict{ID: "T1", Pass: true}).Line() != "T1: PASS" || (Verdict{ID: "T4"}).Line() != "T4: FAIL" {
		t.Fatal("line format")
	}
}

func TestNameStatus(t *testing.T) {
	got := NameStatus([]string{"a.go", "gone.go"}, func(p string) bool { return p == "a.go" })
	if got != "M\ta.go\nD\tgone.go\n" {
		t.Fatalf("%q", got)
	}
}

func TestLoadAndQuiet(t *testing.T) {
	l, err := LoadAvg1("2.07 6.20 9.57 1/2 3")
	if err != nil || l != 2.07 {
		t.Fatal(l, err)
	}
	if IsQuiet(l) || !IsQuiet(2.0) {
		t.Fatal("quiet threshold is <= 2.0")
	}
	if _, err := LoadAvg1(""); err == nil {
		t.Fatal("empty must error")
	}
}

func TestMergeInputsKeepsOtherKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(p, []byte(`{"keep":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MergeInputs(p, map[string]any{"mid_t1_ratio": 0.1}); err != nil {
		t.Fatal(err)
	}
	m := readJSON(p)
	if Num(m, "keep") != 1 || Num(m, "mid_t1_ratio") != 0.1 {
		t.Fatalf("%v", m)
	}
}

func TestNoisyOverride(t *testing.T) {
	if AllowNoisy(false, "") || AllowNoisy(false, "yes") || !AllowNoisy(true, "") || !AllowNoisy(false, "1") {
		t.Fatal("override must be opt-in via flag or BZL_ALLOW_NOISY_HOST=1")
	}
	if IsNoisy(2.0, 1.0) || !IsNoisy(2.01, 1.0) || !IsNoisy(1.0, 3.0) || !IsNoisy(math.NaN(), 1.0) {
		t.Fatal("IsNoisy limit")
	}
}

func TestMergeNoisyAndTag(t *testing.T) {
	p := filepath.Join(t.TempDir(), "in.json")
	e := map[string]any{"not_quiet": true}
	if err := MergeNoisy(p, map[string]any{"mid_edit_test_s": 7.5}, true, e); err != nil {
		t.Fatal(err)
	}
	m := readJSON(p)
	if NoisyTag(m, "mid_edit_test_s") != " [NOISY]" || noisyTagFor("T4", m) != " [NOISY]" || noisyTagFor("T3", m) != "" {
		t.Fatal(m)
	}
	if err := MergeNoisy(p, map[string]any{"mid_edit_test_s": 5.0}, false, nil); err != nil {
		t.Fatal(err)
	}
	if NoisyTag(readJSON(p), "mid_edit_test_s") != "" {
		t.Fatal("quiet re-measure clears the tag")
	}
}
