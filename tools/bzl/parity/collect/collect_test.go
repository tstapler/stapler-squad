package collect

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeHost struct{ loads []float64 }

func (f *fakeHost) Load1() (float64, error) {
	if len(f.loads) == 0 {
		return 0, errors.New("no load")
	}
	l := f.loads[0]
	f.loads = f.loads[1:]
	return l, nil
}

func TestQuietSampleProducesNumber(t *testing.T) {
	g := NewGate(&fakeHost{loads: []float64{0.5, 0.7, 0.6, 0.9}}, true)
	vals, rs, err := Sample(g, 2, func() (float64, error) { return 3, nil })
	if err != nil || len(vals) != 2 || !AllQuiet(rs) {
		t.Fatalf("%v %v %v", vals, rs, err)
	}
	d := NewDoc()
	d.SetTimed("warm", Median(vals), rs, g, "test")
	if v, ok := d.Value("warm"); !ok || v != 3 {
		t.Fatal("quiet series must yield a number")
	}
}

func TestNotQuietRefusesToRunAndMarksUnmeasured(t *testing.T) {
	g := NewGate(&fakeHost{loads: []float64{6.2, 6.2, 6.2}}, true)
	ran := false
	_, rs, err := Sample(g, 5, func() (float64, error) { ran = true; return 1, nil })
	if !errors.Is(err, ErrNotQuiet) || ran {
		t.Fatalf("must refuse before running: err=%v ran=%v", err, ran)
	}
	d := NewDoc()
	d.SetTimed("warm", 1, rs, g, "test")
	if _, ok := d.Value("warm"); ok {
		t.Fatal("NOT QUIET must not yield a number")
	}
	if m := d.Meta("warm"); m.Status != Unmeasured || m.Reason == "" || m.Quiet == nil || *m.Quiet {
		t.Fatalf("%+v", m)
	}
}

func TestPostLoadSpikeTaintsSample(t *testing.T) {
	g := NewGate(&fakeHost{loads: []float64{0.5, 3.1}}, true)
	vals, rs, _ := Sample(g, 1, func() (float64, error) { return 2, nil })
	d := NewDoc()
	d.SetTimed("k", Median(vals), rs, g, "")
	if _, ok := d.Value("k"); ok {
		t.Fatal("load above 2.0 after the sample must void it")
	}
}

func TestRequireQuietOffKeepsAdvisoryOnly(t *testing.T) {
	g := NewGate(&fakeHost{loads: []float64{5, 5}}, false)
	vals, rs, _ := Sample(g, 1, func() (float64, error) { return 9, nil })
	d := NewDoc()
	d.SetTimed("k", Median(vals), rs, g, "")
	if _, ok := d.Value("k"); ok || d.Meta("k").Advisory == nil || *d.Meta("k").Advisory != 9 {
		t.Fatalf("%+v", d.Meta("k"))
	}
}

func TestMergeDropsStaleNumberAndKeepsOthers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(p, []byte(`{"T1":0.2,"T4":3.0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewDoc()
	d.Unmeasured("T4", "NOT QUIET", Meta{})
	d.Set("T3-disk", 5, Meta{})
	if err := Merge(p, d); err != nil {
		t.Fatal(err)
	}
	n, err := Numbers(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := n["T4"]; ok || n["T1"] != 0.2 || n["T3-disk"] != 5 || len(n) != 2 {
		t.Fatalf("%v", n)
	}
}

func TestMedianAndPercentile(t *testing.T) {
	if Median([]float64{3, 1, 2}) != 2 || Median([]float64{1, 2, 3, 4}) != 2.5 {
		t.Fatal("median")
	}
	if Percentile([]float64{10, 20, 30, 40}, 0.9) != 40 {
		t.Fatal("nearest rank")
	}
}

func TestParseLoad1(t *testing.T) {
	if l, err := ParseLoad1("6.18 9.36 8.07 1/2 3\n"); err != nil || l != 6.18 {
		t.Fatal(l, err)
	}
}

func TestS6ReadingsTaintOnLoad(t *testing.T) {
	r := S6Result{Builds: []S6Build{{WallS: 100, Quiet: true, LoadPre: 1, LoadPost: 1}, {WallS: 20, Quiet: true, LoadPre: 1, LoadPost: 4}}}
	if AllQuiet(r.Readings(QuietMaxLoad)) {
		t.Fatal("post load 4 must taint")
	}
	if len(r.SecondWalls()) != 1 {
		t.Fatal("second walls")
	}
}

func TestParseExecLog(t *testing.T) {
	in := `{"runner":"remote cache hit"}
{"runner":"disk cache hit"}
{"runner":"linux-sandbox"}
{"runner":"internal"}
{"runner":"worker","cacheHit":true}
`
	c, err := ParseExecLog(strings.NewReader(in))
	if err != nil || c.Hits != 3 || c.Spawns != 4 {
		t.Fatalf("%+v %v", c, err)
	}
	if r, _ := c.Ratio(); r != 0.75 {
		t.Fatal(r)
	}
}

func TestWarmMainMedianNeedsEnoughRows(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trend.jsonl")
	rows := `{"ref":"refs/heads/main","warm":true,"cache_hit_ratio":0.5}
{"ref":"refs/heads/main","warm":true,"cache_hit_ratio":0.7}
{"ref":"refs/heads/feat","warm":true,"cache_hit_ratio":0.1}
{"ref":"refs/heads/main","warm":false,"cache_hit_ratio":0.1}
`
	if err := os.WriteFile(p, []byte(rows), 0o644); err != nil {
		t.Fatal(err)
	}
	m, n, ok, err := WarmMainMedian(p, 2)
	if err != nil || n != 2 || !ok || m != 0.6 {
		t.Fatalf("%v %v %v %v", m, n, ok, err)
	}
	if _, _, ok, _ := WarmMainMedian(p, 20); ok {
		t.Fatal("fewer than 20 rows must not be enough")
	}
}
