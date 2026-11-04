package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

var rows = []target{
	{"T1", f(0.30)}, {"T2-partial-ttg", f(2433.6)}, {"T2b", f(0.60)}, {"T3-disk", nil}, {"T5", f(1.0)}, {"T6", f(0)},
}

func TestEvaluateDirections(t *testing.T) {
	v := Evaluate(rows, map[string]float64{"T1": 0.25, "T2-partial-ttg": 2000, "T2b": 0.7, "T5": 1.0})
	for id, want := range map[string]string{"T1": pass, "T2": pass, "T3": unmeasured, "T5": pass, "T6": unmeasured} {
		if v[id] != want {
			t.Errorf("%s: got %s want %s", id, v[id], want)
		}
	}
	v = Evaluate(rows, map[string]float64{"T1": 0.5, "T2b": 0.3, "T2-partial-ttg": 1})
	if v["T1"] != fail || v["T2"] != fail || v["T5"] != unmeasured {
		t.Fatalf("%v", v)
	}
}

func TestMissingTargetValueIsUnmeasuredNotPass(t *testing.T) {
	if v := Evaluate(rows, map[string]float64{"T3-disk": 1}); v["T3"] != unmeasured {
		t.Fatalf("%v", v)
	}
}

func TestFailGroupBeatsUnmeasuredRow(t *testing.T) {
	rs := []target{{"T2-a", f(1)}, {"T2-b", f(1)}}
	if v := Evaluate(rs, map[string]float64{"T2-a": 5}); v["T2"] != fail {
		t.Fatalf("%v", v)
	}
}

func TestRunPrintsEightLinesAndFailsOnFixture(t *testing.T) {
	dir := t.TempDir()
	tp := filepath.Join(dir, "t.yaml")
	os.WriteFile(tp, []byte("targets:\n  - id: T1\n    target_value: 0.30\n"), 0o644)
	mp := filepath.Join(dir, "m.json")
	os.WriteFile(mp, []byte(`{"T1": 0.9}`), 0o644)
	var out, errb bytes.Buffer
	if code := run([]string{"-targets", tp, "-fixture", mp}, &out, &errb); code != 1 {
		t.Fatalf("code %d", code)
	}
	if n := strings.Count(out.String(), ": "); n != 8 || !strings.Contains(out.String(), "T1: FAIL\n") {
		t.Fatalf("%q", out.String())
	}
}

func TestMeasurementsIgnoreMetaObject(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(p, []byte(`{"T1":0.2,"_meta":{"T4":{"status":"UNMEASURED","reason":"NOT QUIET"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := loadMeasurements(p)
	if err != nil || len(m) != 1 || m["T1"] != 0.2 {
		t.Fatalf("%v %v", m, err)
	}
}
