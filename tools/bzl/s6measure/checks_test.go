package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeProc struct{ killed bool }

func (p *fakeProc) Kill() error { p.killed = true; return nil }
func (p *fakeProc) Wait() error { return nil }

type fakeExec struct {
	mu    sync.Mutex
	calls []Cmd
	fn    func(Cmd) Out
	proc  *fakeProc
}

func (f *fakeExec) Run(_ context.Context, c Cmd) (Out, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
	if f.fn == nil {
		return Out{}, nil
	}
	return f.fn(c), nil
}

func (f *fakeExec) Start(c Cmd) (BgProc, error) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.proc = &fakeProc{}
	f.mu.Unlock()
	return f.proc, nil
}

func (f *fakeExec) saw(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.Contains(c.String(), sub) {
			return true
		}
	}
	return false
}

type fakeHost struct {
	mu        sync.Mutex
	load, mem float64
	now       time.Time
	sleeps    int
}

func (h *fakeHost) Load1() float64      { return h.load }
func (h *fakeHost) MemAvailGB() float64 { return h.mem }
func (h *fakeHost) Sleep(d time.Duration) {
	h.mu.Lock()
	h.now, h.sleeps = h.now.Add(d), h.sleeps+1
	h.mu.Unlock()
}
func (h *fakeHost) Now() time.Time { h.mu.Lock(); defer h.mu.Unlock(); return h.now }

func quietHost() *fakeHost { return &fakeHost{load: 0.5, mem: 40, now: time.Unix(1000, 0)} }

func newHarness(t *testing.T, x Exec, h Host) *Harness {
	t.Helper()
	d := t.TempDir()
	return &Harness{X: x, H: h, Repo: d, Ref: "HEAD", Dir: filepath.Join(d, "scratch"), DiskCache: filepath.Join(d, "cache"), QuietWait: time.Minute}
}

func arg(c Cmd, prefix string) string {
	for _, a := range c.Args {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			return v
		}
	}
	return ""
}

func writeLog(path string, recs ...Spawn) {
	var b strings.Builder
	for _, r := range recs {
		j, _ := json.Marshal(r)
		b.Write(j)
		b.WriteByte('\n')
	}
	_ = os.WriteFile(path, []byte(b.String()), 0o644)
}

func TestPercentile(t *testing.T) {
	t.Parallel()
	xs := make([]float64, 100)
	for i := range xs {
		xs[i] = float64(i + 1)
	}
	if Percentile(xs, 99) != 99 || Percentile(xs, 100) != 100 || Percentile(xs, 1) != 1 || Percentile(nil, 99) != -1 {
		t.Fatal("nearest-rank percentile")
	}
}

func TestAnalyzeSpawnsE1E2(t *testing.T) {
	t.Parallel()
	hit := Spawn{Mnemonic: "GoCompile", Runner: "disk cache hit", Cacheable: true, TargetLabel: "//a"}
	sp := []Spawn{hit, hit, hit,
		{Mnemonic: "Genrule", Runner: "linux-sandbox", Cacheable: true, TargetLabel: "//tmux:build"},
		{Mnemonic: "GoCompile", Runner: "worker", Cacheable: true, TargetLabel: "//b"},
		{Mnemonic: "SymlinkTree", Runner: "internal", Cacheable: true, TargetLabel: "//c"}}
	st := AnalyzeSpawns(sp, []string{"tmux"})
	if st.Cacheable != 5 || st.Hits != 3 || st.HitRatio != 0.6 {
		t.Fatalf("%+v", st)
	}
	if len(st.NonAllowMisses) != 1 || st.NonAllowMisses[0].TargetLabel != "//b" {
		t.Fatalf("non-allow misses: %+v", st.NonAllowMisses)
	}
	if st.NonAllowExecuted != 1 {
		t.Fatalf("E2 counts non-allow executed spawns: %d", st.NonAllowExecuted)
	}
	if AnalyzeSpawns(nil, nil).HitRatio != -1 {
		t.Fatal("no cacheable spawns must read as unevaluable")
	}
}

func TestParseSpawnsSkipsGarbage(t *testing.T) {
	t.Parallel()
	in := "{\"mnemonic\":\"A\",\"runner\":\"disk cache hit\",\"cacheable\":true}\nnot json\n{}\n"
	if got := ParseSpawns(strings.NewReader(in)); len(got) != 1 || !isHit(got[0]) {
		t.Fatalf("%+v", got)
	}
}

func TestCPUMultiple(t *testing.T) {
	t.Parallel()
	a := Spawn{Mnemonic: "GoCompile", Runner: "linux-sandbox", TargetLabel: "//x"}
	b := Spawn{Mnemonic: "GoCompile", Runner: "linux-sandbox", TargetLabel: "//y"}
	if m := CPUMultiple([][]Spawn{{a, b}, {a, b}, {a, b}}); m != 3 {
		t.Fatalf("three identical cold starts must read 3.0, got %v", m)
	}
	if CPUMultiple(nil) != -1 {
		t.Fatal("no executions")
	}
}

func TestCompareAquery(t *testing.T) {
	t.Parallel()
	if ok, _ := CompareAquery("compile -o bazel-out/x", "compile -o bazel-out/x", "/wt/a", "/wt/b"); !ok {
		t.Fatal("identical relative lines must pass")
	}
	if ok, why := CompareAquery("a", "b"); ok || why == "" {
		t.Fatal("differing lines")
	}
	if ok, _ := CompareAquery("cc -I/wt/a/inc", "cc -I/wt/a/inc", "/wt/a"); ok {
		t.Fatal("worktree path leak")
	}
	if ok, _ := CompareAquery("x /home/u/y", "x /home/u/y"); ok {
		t.Fatal("generic absolute path leak")
	}
}

func TestHashTreeAndDiff(t *testing.T) {
	t.Parallel()
	a, b := t.TempDir(), t.TempDir()
	for _, d := range []string{a, b} {
		_ = os.MkdirAll(filepath.Join(d, "p"), 0o755)
		_ = os.WriteFile(filepath.Join(d, "p", "same"), []byte("1"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(a, "p", "ctl"), []byte(a), 0o644)
	_ = os.WriteFile(filepath.Join(b, "p", "ctl"), []byte(b), 0o644)
	_ = os.WriteFile(filepath.Join(b, "only-b"), []byte("x"), 0o644)
	ha, err := HashTree(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := HashTree(b)
	got := DiffDigests(ha, hb)
	if strings.Join(got, ",") != "only-b,p/ctl" {
		t.Fatalf("%v", got)
	}
}

func TestSmallParsers(t *testing.T) {
	t.Parallel()
	fm := " ext logical physical length flags\n   0:  0..  9:  100..  109: 10: last,shared,eof\n"
	if !ParseFilefragShared(fm) || ParseFilefragShared("   0: 0..9: 1..10: 10: last,eof\n") {
		t.Fatal("filefrag shared flag")
	}
	if CountPattern("ok\nDigest Mismatch here\nCORRUPT blob\n", "corrupt", "digest mismatch") != 2 {
		t.Fatal("corruption grep")
	}
	if CountErrorLines("INFO: x\nERROR: bad\n  ERROR: indented\nERROR: again") != 2 {
		t.Fatal("ERROR: lines")
	}
	if PctChange(100, 85) != -15 || PctChange(0, 5) != 0 {
		t.Fatal("pct")
	}
}

func TestProfileCPUSeconds(t *testing.T) {
	t.Parallel()
	doc := `{"traceEvents":[{"ph":"C","name":"CPU usage (Bazel)","ts":0,"args":{"cpu":0}},
	 {"ph":"C","name":"CPU usage (Bazel)","ts":1000000,"args":{"cpu":2}},
	 {"ph":"C","name":"CPU usage (Bazel)","ts":3000000,"args":{"cpu":1}},{"ph":"X","name":"other","ts":5}]}`
	if got := ProfileCPUSeconds([]byte(doc)); got != 4 {
		t.Fatalf("plain: %v", got)
	}
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	_, _ = zw.Write([]byte(doc))
	_ = zw.Close()
	if ProfileCPUSeconds(zb.Bytes()) != 4 {
		t.Fatal("gzip")
	}
	if ProfileCPUSeconds([]byte(`{"traceEvents":[]}`)) != -1 || ProfileCPUSeconds([]byte("x")) != -1 {
		t.Fatal("missing counter must be -1, not 0")
	}
}

func TestWorkerMnemonicsAndVerdict(t *testing.T) {
	t.Parallel()
	doc := `{"actions":[{"mnemonic":"GoCompile","executionInfo":{"supports-workers":"1"}},
	 {"mnemonic":"TsProject","executionRequirements":[{"key":"supports-multiplex-workers","value":"1"}]},
	 {"mnemonic":"Genrule"}]}`
	w, m, err := WorkerMnemonics([]byte(doc))
	if err != nil || strings.Join(w, ",") != "GoCompile,TsProject" || strings.Join(m, ",") != "TsProject" {
		t.Fatalf("%v %v %v", w, m, err)
	}
	if _, _, err := WorkerMnemonics([]byte("nope")); err == nil {
		t.Fatal("bad json")
	}
	_, _, v := MultiplexVerdict(workerMeasure{1000, 100}, workerMeasure{880, 105})
	if v != "Pass" {
		t.Fatal(v)
	}
	if _, _, v = MultiplexVerdict(workerMeasure{1000, 100}, workerMeasure{950, 100}); v != "NoAction" {
		t.Fatal("5% reduction must not adopt")
	}
	if _, _, v = MultiplexVerdict(workerMeasure{1000, 100}, workerMeasure{800, 115}); v != "NoAction" {
		t.Fatal("wall regression above 10% must not adopt")
	}
}

func TestExternalRepos(t *testing.T) {
	t.Parallel()
	d, contents := t.TempDir(), t.TempDir()
	_ = os.MkdirAll(filepath.Join(contents, "abc", "contents"), 0o755)
	target := filepath.Join(contents, "abc", "contents", "x")
	_ = os.MkdirAll(target, 0o755)
	_ = os.Symlink(target, filepath.Join(d, "rules_go+"))
	_ = os.MkdirAll(filepath.Join(d, "gazelle+"), 0o755) // plain, not linked
	_ = os.MkdirAll(filepath.Join(d, "gazelle++go_deps+com_x"), 0o755)
	_ = os.MkdirAll(filepath.Join(d, "bazel_tools"), 0o755)
	_ = os.WriteFile(filepath.Join(d, "@rules_go+.marker"), nil, 0o644)
	total, linked, ext, err := ExternalRepos(d)
	if err != nil || total != 2 || linked != 1 || len(ext) != 1 || ext[0] != "gazelle++go_deps+com_x" {
		t.Fatalf("%d %d %v %v", total, linked, ext, err)
	}
}

func TestValidateIsolation(t *testing.T) {
	t.Parallel()
	if ValidateIsolation("ssq-manual-1", "http://127.0.0.1:62871/") != nil {
		t.Fatal("manual instance must be accepted")
	}
	for _, bad := range [][2]string{{"default", "http://127.0.0.1:62871/"}, {"ssq-manual-1", "http://localhost:8543/"}, {"ssq-manual-1", "https://h:8444"}, {"", "x"}, {"ssq-manual-1; rm", "x"}} {
		if ValidateIsolation(bad[0], bad[1]) == nil {
			t.Fatalf("%v must be refused", bad)
		}
	}
}

func TestGate(t *testing.T) {
	t.Parallel()
	h := quietHost()
	if r, err := Gate(h, "x", false, time.Minute, func(string) {}); err != nil || r.OverrideAccepted {
		t.Fatalf("quiet host: %v %+v", err, r)
	}
	h = &fakeHost{load: 9, mem: 40, now: time.Unix(0, 0)}
	if _, err := Gate(h, "x", false, 30*time.Second, func(string) {}); err == nil || h.sleeps == 0 {
		t.Fatal("noisy host without override must wait then refuse")
	}
	var warned string
	r, err := Gate(h, "x", true, time.Second, func(s string) { warned = s })
	if err != nil || !r.OverrideAccepted || !strings.Contains(warned, "NOISY HOST OVERRIDE") {
		t.Fatalf("override must start at once, loudly: %v %+v %q", err, r, warned)
	}
	r.Finish(h)
	if !r.NotQuiet {
		t.Fatal("override reading must be tagged not_quiet")
	}
	if _, err := Gate(&fakeHost{load: 0.1, mem: 8}, "x", true, time.Second, func(string) {}); err == nil {
		t.Fatal("below the 15 GB floor must refuse even with the override")
	}
}

func TestReportUpdateTagsNoisyAndSkipsErrors(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "m.json")
	rep := NewReport()
	rep.Keys["e1_hit_ratio"], rep.Keys["e3_gocompile_identical"], rep.Keys["e4_differing_files"] = 0.97, true, []string{"a"}
	rep.Check("e1", true, "rule")
	rs := []Reading{{Label: "x", LoadPre: 12, LoadPost: 14, MemPre: 30, MemPost: 30, NotQuiet: true}}
	if err := rep.Finish(filepath.Join(filepath.Dir(path), "out.json"), path, "s6measure cross", rs, true); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := readJSON(path, &m); err != nil {
		t.Fatal(err)
	}
	if m["s6_e1_hit_ratio"] != 0.97 || m["s6_e3_gocompile_identical"] != true {
		t.Fatalf("%v", m)
	}
	if _, ok := m["s6_e4_differing_files"]; ok {
		t.Fatal("lists must stay out of the measurements file")
	}
	side := m["noisy_inputs"].(map[string]any)
	if e, ok := side["s6_e1_hit_ratio"].(map[string]any); !ok || e["not_quiet"] != true {
		t.Fatalf("noisy tag: %v", side)
	}
	bad := NewReport()
	bad.Keys["x"] = 1.0
	bad.Errorf("boom")
	p2 := filepath.Join(t.TempDir(), "m2.json")
	_ = bad.Finish(filepath.Join(t.TempDir(), "o.json"), p2, "s", nil, false)
	if _, err := os.Stat(p2); err == nil {
		t.Fatal("an errored run must write no measurement keys")
	}
	if NewReport().ChecksPass() {
		t.Fatal("nothing checked is not a pass")
	}
}

// ---- orchestrators against fakes ----

// bazelFake answers the wrapper and bazelisk calls the orchestrators make.
func bazelFake(t *testing.T, bins map[string]string, logFor func(label string, c Cmd) []Spawn) *fakeExec {
	return &fakeExec{fn: func(c Cmd) Out {
		switch {
		case c.Name == "git" || c.Name == "chmod":
			return Out{}
		case len(c.Args) > 0 && c.Args[0] == "info" || len(c.Args) > 2 && c.Args[2] == "info":
			key := c.Args[len(c.Args)-1]
			dir := filepath.Join(bins["root"], strings.ReplaceAll(c.Dir, "/", "_"), key)
			_ = os.MkdirAll(dir, 0o755)
			if key == "bazel-bin" {
				if err := os.WriteFile(filepath.Join(dir, "f"), []byte(bins["f"]), 0o644); err != nil {
					t.Error(err)
				}
				_ = os.MkdirAll(filepath.Join(dir, controlPkg), 0o755)
				_ = os.WriteFile(filepath.Join(dir, pwdControl), []byte(c.Dir), 0o644)
				_ = os.WriteFile(filepath.Join(dir, stampControl), []byte(c.Dir), 0o644)
			}
			return Out{Stdout: dir + "\n"}
		case len(c.Args) > 0 && c.Args[0] == "aquery":
			return Out{Stdout: "GoCompile bazel-out/k8/bin/x\n"}
		}
		if lp := arg(c, "--execution_log_json_file="); lp != "" {
			writeLog(lp, logFor(strings.TrimSuffix(filepath.Base(lp), ".execlog.json"), c)...)
		}
		return Out{}
	}}
}

func TestCrossAllChecksPass(t *testing.T) {
	t.Parallel()
	hit := Spawn{Mnemonic: "GoCompile", Runner: "disk cache hit", Cacheable: true, TargetLabel: "//a"}
	exe := Spawn{Mnemonic: "GoCompile", Runner: "linux-sandbox", Cacheable: true, TargetLabel: "//a"}
	fx := bazelFake(t, map[string]string{"root": t.TempDir(), "f": "same"}, func(l string, c Cmd) []Spawn {
		switch l {
		case "e1-b", "e2-c":
			return []Spawn{hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit, hit}
		case "stamp-b":
			return []Spawn{{Mnemonic: "Genrule", Runner: "linux-sandbox", TargetLabel: "//tools/bzl/spike/s6_control:stamped_control"}}
		}
		return []Spawn{exe}
	})
	h := newHarness(t, fx, quietHost())
	rep := Cross(h, CrossOpts{Target: "//:stapler-squad", GoTargets: []string{"//config", "//log", "//session/git"}, E4Targets: []string{"//..."}, Allow: []string{"tmux"}})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	// the fake advances no wall time, so E2's 30% bound is 0 <= 0
	for _, k := range []string{"e1", "e2", "e3", "e4", "e4_stamp"} {
		if !rep.Checks[k] {
			t.Errorf("check %s did not pass: keys=%v", k, rep.Keys)
		}
	}
	if rep.Keys["e1_hit_ratio"] != 1.0 || rep.Keys["e4_control_flagged"] != true || rep.Keys["e4_digest_lists_equal"] != true {
		t.Fatalf("%v", rep.Keys)
	}
	if !fx.saw("--disk_cache= --stamp") {
		t.Fatal("E4 must build with the disk cache disabled")
	}
	if !fx.saw("worktree remove") {
		t.Fatal("scratch worktrees must be removed")
	}
}

func TestCrossFailsOnLowHitRatioAndRequiresEmptyCache(t *testing.T) {
	t.Parallel()
	hit := Spawn{Mnemonic: "GoCompile", Runner: "disk cache hit", Cacheable: true, TargetLabel: "//a"}
	miss := Spawn{Mnemonic: "GoCompile", Runner: "linux-sandbox", Cacheable: true, TargetLabel: "//a"}
	fx := bazelFake(t, map[string]string{"root": t.TempDir(), "f": "x"}, func(l string, c Cmd) []Spawn {
		if l == "e1-b" {
			return []Spawn{hit, miss}
		}
		return nil
	})
	h := newHarness(t, fx, quietHost())
	rep := Cross(h, CrossOpts{Target: "//:t", Which: map[string]bool{"e1": true}})
	if rep.Checks["e1"] || rep.Keys["e1_hit_ratio"] != 0.5 {
		t.Fatalf("50%% hits must fail E1: %v %v", rep.Checks, rep.Keys)
	}
	h2 := newHarness(t, bazelFake(t, map[string]string{"root": t.TempDir()}, nil), quietHost())
	_ = os.MkdirAll(h2.DiskCache, 0o755)
	_ = os.WriteFile(filepath.Join(h2.DiskCache, "dirty"), nil, 0o644)
	rep = Cross(h2, CrossOpts{Target: "//:t", Which: map[string]bool{"e1": true}})
	if len(rep.Errors) == 0 || len(rep.Checks) != 0 {
		t.Fatal("a non-empty shared cache must be an error, not a check result")
	}
}

func TestCrossNoisyHostRefusedWithoutOverride(t *testing.T) {
	t.Parallel()
	noisy := &fakeHost{load: 9, mem: 40, now: time.Unix(0, 0)}
	h := newHarness(t, bazelFake(t, map[string]string{"root": t.TempDir()}, func(string, Cmd) []Spawn { return nil }), noisy)
	h.QuietWait = time.Minute
	rep := Cross(h, CrossOpts{Target: "//:t", Which: map[string]bool{"e1": true}})
	if len(rep.Errors) == 0 || len(rep.Checks) != 0 {
		t.Fatal("noisy host without the override must produce an error and no check")
	}
	h = newHarness(t, bazelFake(t, map[string]string{"root": t.TempDir()}, func(string, Cmd) []Spawn { return nil }), noisy)
	h.AllowNoisy = true
	rep = Cross(h, CrossOpts{Target: "//:t", Which: map[string]bool{"e1": true}})
	if !rep.Output(h.Readings, true)["not_quiet"].(bool) {
		t.Fatal("override run must be tagged not_quiet")
	}
}

func TestDiskConc(t *testing.T) {
	t.Parallel()
	exe := Spawn{Mnemonic: "GoCompile", Runner: "linux-sandbox", Cacheable: true, TargetLabel: "//a"}
	fx := bazelFake(t, map[string]string{"root": t.TempDir(), "f": "same"}, func(string, Cmd) []Spawn { return []Spawn{exe} })
	h := newHarness(t, fx, quietHost())
	rep := DiskConc(h, DiskConcOpts{Target: "//...", Rounds: 2, GCMaxSize: "1G"})
	if len(rep.Errors) != 0 {
		t.Fatal(rep.Errors)
	}
	if rep.Keys["concurrency_cpu_multiple"] != 3.0 || rep.Keys["hash_lists_equal"] != true || rep.Keys["corruption_lines"] != 0 || rep.Keys["gc_race_errors"] != 0 {
		t.Fatalf("%v", rep.Keys)
	}
	if !rep.Checks["disk_cache_integrity"] || !rep.Checks["gc_race"] {
		t.Fatalf("%v", rep.Checks)
	}
	if !fx.saw("--max_idle_secs=300") || !fx.saw("--experimental_disk_cache_gc_max_size=1G") {
		t.Fatal("bypass run and GC run flags missing")
	}
}

func TestDiskConcDetectsCorruptionLine(t *testing.T) {
	t.Parallel()
	fx := bazelFake(t, map[string]string{"root": t.TempDir(), "f": "same"}, func(string, Cmd) []Spawn { return nil })
	inner := fx.fn
	fx.fn = func(c Cmd) Out {
		o := inner(c)
		if len(c.Args) > 0 && c.Args[0] == "build" {
			o.Stderr = "ERROR: digest mismatch for blob"
		}
		return o
	}
	rep := DiskConc(newHarness(t, fx, quietHost()), DiskConcOpts{Target: "//...", Rounds: 1, GCMaxSize: "1G"})
	if rep.Checks["disk_cache_integrity"] || rep.Keys["corruption_lines"] == 0 {
		t.Fatalf("%v %v", rep.Checks, rep.Keys)
	}
}

type fakeProber struct {
	mu   sync.Mutex
	n    int
	idle time.Duration
	load time.Duration
	busy func() bool
	// enough is closed once five probes ran while a build was in flight
	enough chan struct{}
	busyN  int
}

func (p *fakeProber) Probe() (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	if p.busy != nil && p.busy() {
		if p.busyN++; p.busyN == 5 && p.enough != nil {
			close(p.enough)
		}
		return p.load, nil
	}
	return p.idle, nil
}

func queuedRun(t *testing.T, idle, load time.Duration) *Report {
	t.Helper()
	var running int32
	var mu sync.Mutex
	enough := make(chan struct{})
	fx := bazelFake(t, map[string]string{"root": t.TempDir()}, nil)
	inner := fx.fn
	fx.fn = func(c Cmd) Out {
		if len(c.Args) > 0 && c.Args[0] == "build" {
			mu.Lock()
			running++
			mu.Unlock()
			<-enough
		}
		return inner(c)
	}
	host := quietHost()
	p := &fakeProber{enough: enough, idle: idle, load: load, busy: func() bool { mu.Lock(); defer mu.Unlock(); return running > 0 }}
	h := newHarness(t, fx, host)
	return Queued(h, QueuedOpts{Target: "//:t", Invocations: 8, Worktrees: 4, IdleSamples: 100, Gap: time.Millisecond, Prober: p})
}

func TestQueuedStreamRatioAndUnevaluatedRSS(t *testing.T) {
	t.Parallel()
	rep := queuedRun(t, 10*time.Millisecond, 11*time.Millisecond)
	if !rep.Checks["stream_p99"] {
		t.Fatalf("1.1x idle must pass: %v %v", rep.Checks, rep.Keys)
	}
	if rep.Keys["queued_invocations"] != 8 {
		t.Fatal(rep.Keys)
	}
	if len(rep.Errors) == 0 || rep.ChecksPass() {
		t.Fatal("with RSS sampling off the peak-RSS clause is unevaluated, so the report cannot pass")
	}
	rep = queuedRun(t, 10*time.Millisecond, 13*time.Millisecond)
	if rep.Checks["stream_p99"] {
		t.Fatal("1.3x idle must fail")
	}
}

func TestQueuedFailedInvocationIsNotAResult(t *testing.T) {
	t.Parallel()
	fx := bazelFake(t, map[string]string{"root": t.TempDir()}, nil)
	inner := fx.fn
	fx.fn = func(c Cmd) Out {
		if len(c.Args) > 0 && c.Args[0] == "build" {
			return Out{Code: 75}
		}
		return inner(c)
	}
	p := &fakeProber{idle: time.Millisecond, load: time.Millisecond}
	rep := Queued(newHarness(t, fx, quietHost()), QueuedOpts{Target: "//:t", Invocations: 8, Worktrees: 2, IdleSamples: 5, Gap: time.Millisecond, Prober: p})
	if len(rep.Checks) != 0 || len(rep.Errors) == 0 {
		t.Fatalf("wrapper slot timeout (exit 75) must not yield a verdict: %v", rep)
	}
}

func TestTmuxProberUsesPrivateSocketAndMarker(t *testing.T) {
	t.Parallel()
	var sent string
	fx := &fakeExec{fn: func(c Cmd) Out {
		if len(c.Args) > 3 && c.Args[2] == "send-keys" {
			sent = c.Args[5]
		}
		if len(c.Args) > 2 && c.Args[2] == "capture-pane" {
			return Out{Stdout: "line\n" + sent + "\n"}
		}
		return Out{}
	}}
	p := &tmuxProber{X: fx, H: quietHost(), Sock: "ssq-manual-1"}
	if err := p.Setup(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Probe(); err != nil {
		t.Fatal(err)
	}
	p.Teardown()
	for _, c := range fx.calls {
		if c.Name != "tmux" || c.Args[0] != "-L" || c.Args[1] != "ssq-manual-1" {
			t.Fatalf("every tmux call must name the private socket: %v", c)
		}
	}
	if !fx.saw("kill-session -t s6stream") || fx.saw("kill-server") {
		t.Fatal("teardown must kill only its own session")
	}
}

func TestLayoutE5AndE6(t *testing.T) {
	t.Parallel()
	obRoot := t.TempDir()
	fx := &fakeExec{}
	fx.fn = func(c Cmd) Out {
		if c.Name == "bazelisk" {
			// emulate GC: with the idle delay flag the cache is trimmed to 50 MB, otherwise left alone
			cache := arg(c, "--disk_cache=")
			if cache != "" && strings.Contains(c.String(), "gc_idle_delay=10s") {
				_ = os.RemoveAll(cache)
				_ = populateCache(cache, 50)
			}
			return Out{}
		}
		if len(c.Args) > 0 && c.Args[0] == "info" {
			ob := filepath.Join(obRoot, filepath.Base(c.Dir))
			_ = os.MkdirAll(filepath.Join(ob, "external"), 0o755)
			return Out{Stdout: ob}
		}
		return Out{}
	}
	h := newHarness(t, fx, quietHost())
	rep := NewReport()
	layoutE5(h, rep, LayoutOpts{TinyTarget: "//:t"}, "/wt/a")
	if !rep.Checks["e5"] || rep.Keys["e5_gc_trimmed_with_idle_delay"] != true || rep.Keys["e5_default_delay_untrimmed"] != true {
		t.Fatalf("%v %v %v", rep.Checks, rep.Keys, rep.Errors)
	}
	if !fx.saw("--max_idle_secs=30") {
		t.Fatal("E5 must run with max_idle_secs=30")
	}
	rep = NewReport()
	layoutE6(h, rep, LayoutOpts{}, "/wt/a", "/wt/b")
	if rep.Keys["e6_offline_build_ok"] != true || !fx.saw("--repository_disable_download") {
		t.Fatalf("%v %v", rep.Keys, rep.Errors)
	}
	if _, ok := rep.Keys["e6_extension_repos_listed"].([]string); !ok {
		t.Fatalf("extension repos must be listed even when empty: %v", rep.Keys)
	}
}

func TestPopulateCacheBackdates(t *testing.T) {
	t.Parallel()
	d := t.TempDir()
	if err := populateCache(d, 3); err != nil {
		t.Fatal(err)
	}
	if mb := duMB(d); mb < 2.5 {
		t.Fatalf("%v MB", mb)
	}
}

func TestContainmentE7(t *testing.T) {
	t.Parallel()
	fx := &fakeExec{fn: func(c Cmd) Out {
		switch c.Name {
		case "systemd-run":
			return Out{Stdout: "268435456\n", Code: 137}
		case "systemctl":
			return Out{Stdout: "MainPID=42\n"}
		case "bazelisk":
			return Out{Stdout: "1\n"}
		case "ionice":
			return Out{Stdout: "best-effort: prio 7\n"}
		case "chrt":
			return Out{Stdout: "current scheduling policy: SCHED_BATCH\n"}
		}
		return Out{}
	}}
	rep := Containment(newHarness(t, fx, quietHost()), ContainmentOpts{LiveUnit: "stapler-squad.service", ScopeLimit: "256M", AllocMB: 600, SkipCold: true})
	if !rep.Checks["e7"] || rep.Keys["e7_io_nice_applied"] != true || rep.Keys["e7_batch_scheduling_applied"] != true {
		t.Fatalf("%v %v %v", rep.Checks, rep.Keys, rep.Errors)
	}
	for _, c := range fx.calls {
		if c.Name == "systemctl" && (c.Args[1] != "show") {
			t.Fatalf("the live unit may only be read: %v", c)
		}
	}
}

func TestContainmentE11Ratio(t *testing.T) {
	t.Parallel()
	prof := func(cores float64) string {
		return fmt.Sprintf(`{"traceEvents":[{"ph":"C","name":"CPU usage (Bazel)","ts":0,"args":{"cpu":0}},{"ph":"C","name":"CPU usage (Bazel)","ts":10000000,"args":{"cpu":%v}}]}`, cores)
	}
	run := func(serialCores float64) *Report {
		fx := &fakeExec{fn: func(c Cmd) Out {
			if p := arg(c, "--profile="); p != "" {
				cores := 4.0
				if strings.Contains(p, "serial") {
					cores = serialCores
				}
				_ = os.WriteFile(p, []byte(prof(cores)), 0o644)
			}
			return Out{}
		}}
		h := newHarness(t, fx, quietHost())
		rep := NewReport()
		containE11(h, rep, ContainmentOpts{Target: "//:t"})
		return rep
	}
	if rep := run(2); !rep.Checks["e11"] || rep.Keys["e11_serial_over_concurrent_cpu"] != 0.5 {
		t.Fatalf("serial at half the CPU must pass: %v", rep.Keys)
	}
	if rep := run(3); rep.Checks["e11"] {
		t.Fatal("75% of concurrent must fail the 60% bound")
	}
}

func TestWorkersNoneFound(t *testing.T) {
	t.Parallel()
	fx := &fakeExec{fn: func(c Cmd) Out {
		if len(c.Args) > 0 && c.Args[0] == "aquery" {
			return Out{Stdout: `{"actions":[{"mnemonic":"Genrule"}]}`}
		}
		return Out{}
	}}
	rep := Workers(newHarness(t, fx, quietHost()), WorkersOpts{Target: "//..."})
	if rep.Keys["outcome"] != "none-found" || rep.Keys["verdict"] != "NoAction" || len(rep.Errors) != 0 {
		t.Fatalf("%v %v", rep.Keys, rep.Errors)
	}
}

func TestLayeringE8(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil, quietHost())
	fx := &fakeExec{fn: func(c Cmd) Out {
		if lp := arg(c, "--execution_log_json_file="); lp != "" && strings.Contains(lp, "e8-b") {
			writeLog(lp, Spawn{Mnemonic: "GoCompile", Runner: "remote cache hit", Cacheable: true})
			if d := arg(c, "--disk_cache="); d != "" {
				_ = populateCache(d, 2) // a remote hit written back to the disk layer
			}
		}
		return Out{}
	}}
	h.X = fx
	rep := Layering(h, LayeringOpts{Target: "//:t", RemoteBin: "bazel-remote", Port: "62876", KillAfter: time.Second})
	if !rep.Checks["e8"] || rep.Keys["b_remote_hits"] != 1 {
		t.Fatalf("%v %v %v", rep.Checks, rep.Keys, rep.Errors)
	}
	if !fx.proc.killed {
		t.Fatal("the loopback remote must be killed")
	}
	if !fx.saw("127.0.0.1:62876") || fx.saw(":8543") {
		t.Fatal("remote must bind loopback on the manual-block port only")
	}
}

func TestCostAndMakeBaseline(t *testing.T) {
	t.Parallel()
	base := filepath.Join(t.TempDir(), "b.json")
	_ = os.WriteFile(base, []byte(`{"runs":[{"wall_s":56,"disk_kb_worktree":500000},{"wall_s":57,"disk_kb_worktree":520000},{"wall_s":55,"disk_kb_worktree":510000}]}`), 0o644)
	w, d, err := MakeBaseline(base)
	if err != nil || w != 56 || d != 510000 {
		t.Fatalf("%v %v %v", w, d, err)
	}
	if _, _, err := MakeBaseline(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing baseline must error")
	}
	root := t.TempDir()
	fx := &fakeExec{fn: func(c Cmd) Out {
		if len(c.Args) > 0 && c.Args[0] == "info" {
			ob := filepath.Join(root, strings.ReplaceAll(c.Dir, "/", "_"))
			_ = os.MkdirAll(filepath.Join(ob, "external"), 0o755)
			return Out{Stdout: ob}
		}
		if c.Name == "stat" {
			return Out{Stdout: "btrfs\n"}
		}
		return Out{}
	}}
	h := newHarness(t, fx, quietHost())
	rep := Cost(h, CostOpts{Target: "//:t", Baseline: base, Worktrees: 5})
	for _, k := range []string{"cold_ratio_median", "output_base_disk_kb", "external_new_kb_per_worktree", "cache_fs", "output_base_fs", "inwt_wall_s", "inwt_removed_with_worktree"} {
		if _, ok := rep.Keys[k]; !ok {
			t.Errorf("missing key %s (errors %v)", k, rep.Errors)
		}
	}
	if rep.ChecksPass() {
		t.Fatal("peak RSS unsampled in a unit test, so the cost report must not pass")
	}
	if !fx.saw("BAZEL_WORKTREE_OUTPUT_BASE") && len(fx.calls) > 0 {
		var seen bool
		for _, c := range fx.calls {
			for _, e := range c.Env {
				seen = seen || e == "BAZEL_WORKTREE_OUTPUT_BASE=1"
			}
		}
		if !seen {
			t.Fatal("in-worktree variant must set BAZEL_WORKTREE_OUTPUT_BASE=1")
		}
	}
}

func TestRejectsTmpScratchDir(t *testing.T) {
	t.Parallel()
	c := checkFlags{dir: "/tmp/x"}
	if _, err := c.harness(); err == nil {
		t.Fatal("/tmp scratch must be refused")
	}
	if _, err := (&checkFlags{}).harness(); err == nil {
		t.Fatal("-dir is required")
	}
}
