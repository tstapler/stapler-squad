package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTreeRSSCountsDescendantsOnce(t *testing.T) {
	procs := []Proc{
		{1, 0, 100, "init"},
		{10, 1, 2000, "java --output_base=/ob/a server"},
		{11, 10, 300, "go compile"},
		{12, 11, 50, "asm"},
		{20, 1, 4000, "java --output_base=/ob/b server"},
		{30, 1, 999, "unrelated"},
		{40, 12, 5, "nested --output_base=/ob/a"}, // matched and already reachable from 10
	}
	kb, n := TreeRSS(procs, []string{"--output_base=/ob/a"})
	if kb != 2355 || n != 4 {
		t.Fatalf("a only: kb=%d n=%d", kb, n)
	}
	kb, _ = TreeRSS(procs, []string{"--output_base=/ob/a", "--output_base=/ob/b"})
	if kb != 6355 {
		t.Fatalf("a+b: %d", kb)
	}
	if kb, n = TreeRSS(procs, []string{""}); kb != 0 || n != 0 {
		t.Fatal("empty match must select nothing")
	}
}

func TestPeakAndMedian(t *testing.T) {
	var p Peak
	for _, v := range []int64{5, 9, 3} {
		p.Add(v)
	}
	if p.MaxKB != 9 || p.Samples != 3 {
		t.Fatal(p)
	}
	if Median([]float64{3, 1, 2}) != 2 || Median([]float64{1, 2, 3, 4}) != 2.5 || Median(nil) != -1 {
		t.Fatal("median")
	}
}

func TestParsers(t *testing.T) {
	if ParseLoad1("1.75 2 3 1/2 3") != 1.75 || ParseLoad1("x") != -1 || ParseLoad1("") != -1 {
		t.Fatal("load")
	}
	if g := ParseMemAvailGB("MemTotal: 1 kB\nMemAvailable:   2097152 kB\n"); g != 2 {
		t.Fatal(g)
	}
	if ParseMemAvailGB("nothing") != -1 {
		t.Fatal("absent")
	}
}

func TestAllowNoisyIsOptIn(t *testing.T) {
	if AllowNoisy(false, "") || AllowNoisy(false, "0") || AllowNoisy(false, "true") {
		t.Fatal("default and non-1 env must keep the quiet-host gate")
	}
	if !AllowNoisy(true, "") || !AllowNoisy(false, "1") {
		t.Fatal("flag or env=1 enables the override")
	}
}

func TestIsNoisy(t *testing.T) {
	if IsNoisy(1.9, 2.0) {
		t.Fatal("<=2.0 both ends is quiet")
	}
	if !IsNoisy(1.0, 2.1) || !IsNoisy(4, 1) || !IsNoisy(-1, 1) {
		t.Fatal("either end above 2.0, or unknown, is noisy")
	}
}

func TestNoisyEntryAndUpdateInputs(t *testing.T) {
	bs := []Build{{LoadPre: 4, LoadPost: 5, MemAvailPre: 40, MemAvailPost: 30, NotQuiet: true}, {LoadPre: 1, LoadPost: 1, MemAvailPre: 35, MemAvailPost: 36}}
	e := NoisyEntry(bs, "src")
	if e["not_quiet"] != true || e["load1_pre_max"] != 4.0 || e["load1_post_max"] != 5.0 || e["mem_avail_gb_min"] != 30.0 {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(p, []byte(`{"keep":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateInputs(p, map[string]any{"a": 2.0}, true, e); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	b, _ := os.ReadFile(p)
	_ = json.Unmarshal(b, &got)
	side, _ := got["noisy_inputs"].(map[string]any)
	if got["keep"] != 1.0 || got["a"] != 2.0 || side["a"] == nil {
		t.Fatal(got)
	}
	if err := UpdateInputs(p, map[string]any{"a": 3.0}, false, nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	got = nil
	_ = json.Unmarshal(b, &got)
	if _, has := got["noisy_inputs"]; has {
		t.Fatal("a quiet re-measurement must clear the tag", got)
	}
}
