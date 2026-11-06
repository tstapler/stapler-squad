// economic-midpoint prints the Story 2.7 mid-point criteria T1, T3, T4 and gathers their inputs.
//
//	economic-midpoint t1     replay the sampled PR diffs through tools/ci/affected-targets
//	economic-midpoint t4     warm edit-build-test latency of `bazel test //config:all`
//	economic-midpoint report print T1/T3/T4 PASS|FAIL from spike-measurements.json + measured.json
//
// t4 takes -allow-noisy-host (or BZL_ALLOW_NOISY_HOST=1), the owner-authorised override: it runs under load
// instead of waiting for a quiet host, and its output and inputs entry are marked not_quiet / [NOISY].
// A criterion that cannot be evaluated prints FAIL (plan rule), with the reason on stderr.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: economic-midpoint t1|t4|report [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "t1":
		err = runT1(os.Args[2:])
	case "t4":
		err = runT4(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "economic-midpoint:", err)
		os.Exit(1)
	}
}

const defaultInputs = "project_plans/bazel-conversion/implementation/spike-measurements.json"

func bazelOut(root, bazel string, args ...string) ([]byte, error) {
	cmd := exec.Command(bazel, args...) //nolint:norawexec // dev-time measurement tool, waited on
	cmd.Dir = root
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w: %s", bazel, strings.Join(args, " "), err, tail(errb.String()))
	}
	return out, nil
}

func tail(s string) string {
	if len(s) > 600 {
		return s[len(s)-600:]
	}
	return s
}

func runT1(args []string) error {
	fs := flag.NewFlagSet("t1", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	sample := fs.String("sample", "tools/bzl/baseline/pr-sample.json", "pr-sample.json")
	affected := fs.String("affected", "", "path of the built affected-targets binary (required)")
	bazel := fs.String("bazel", "bazel", "bazel launcher")
	out := fs.String("out", "", "write per-PR detail JSON here")
	update := fs.String("update", "", "merge mid_t1_ratio into this inputs JSON")
	_ = fs.Parse(args)
	if *affected == "" {
		return fmt.Errorf("-affected is required")
	}
	raw, err := os.ReadFile(filepath.Join(*root, *sample))
	if err != nil {
		return err
	}
	var s struct {
		Partial []struct {
			Number int      `json:"number"`
			Paths  []string `json:"paths"`
		} `json:"partial"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	tq, err := bazelOut(*root, *bazel, "query", "--output=label", `kind(".*_test", //...)`)
	if err != nil {
		return err
	}
	total := len(strings.Fields(string(tq)))
	type per struct {
		Number   int     `json:"number"`
		Files    int     `json:"files"`
		Selected int     `json:"selected_tests"`
		Ratio    float64 `json:"ratio"`
	}
	var pers []per
	var ratios []float64
	for _, pr := range s.Partial {
		ns := NameStatus(pr.Paths, func(p string) bool { _, e := os.Stat(filepath.Join(*root, p)); return e == nil })
		f, err := os.CreateTemp(os.Getenv("TMPDIR"), "ns")
		if err != nil {
			return err
		}
		_, _ = f.WriteString(ns)
		_ = f.Close()
		cmd := exec.Command(*affected, "-root", *root, "-name-status", f.Name(), "-bazel", *bazel) //nolint:norawexec // dev-time measurement tool, waited on
		var eb bytes.Buffer
		cmd.Stderr = &eb
		o, err := cmd.Output()
		_ = os.Remove(f.Name())
		if err != nil {
			return fmt.Errorf("PR %d: %w: %s", pr.Number, err, tail(eb.String()))
		}
		sel := strings.Fields(string(o))
		r := Ratio(sel, total)
		n := 0
		for _, t := range sel {
			if !strings.HasPrefix(t, "job:") {
				n++
			}
		}
		pers = append(pers, per{pr.Number, len(pr.Paths), n, r})
		ratios = append(ratios, r)
	}
	med := median(ratios)
	detail := map[string]any{"total_test_targets": total, "n": len(pers), "median_ratio": med, "per_pr": pers}
	if *out != "" {
		b, _ := json.MarshalIndent(detail, "", "  ")
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	if *update != "" {
		if err := MergeInputs(*update, map[string]any{"mid_t1_ratio": med, "mid_t1_n": len(pers), "mid_t1_total_test_targets": total}); err != nil {
			return err
		}
	}
	fmt.Printf("T1 median affected-test ratio %.4f over %d PRs (%d test targets in //...)\n", med, len(pers), total)
	return nil
}

func loadNow() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return math.NaN()
	}
	l, err := LoadAvg1(string(b))
	if err != nil {
		return math.NaN()
	}
	return l
}

func waitQuiet(max time.Duration) (float64, bool) {
	end := time.Now().Add(max)
	for {
		l := loadNow()
		if IsQuiet(l) {
			return l, true
		}
		if time.Now().After(end) {
			return l, false
		}
		time.Sleep(5 * time.Second)
	}
}

func runT4(args []string) error {
	fs := flag.NewFlagSet("t4", flag.ExitOnError)
	root := fs.String("root", ".", "repository root")
	file := fs.String("file", "", "non-test Go file in config/ to edit (required)")
	pattern := fs.String("target", "//config:all", "bazel test target")
	runs := fs.Int("runs", 5, "timed runs")
	bazel := fs.String("bazel", "bazel", "bazel launcher")
	out := fs.String("out", "", "write samples JSON here")
	update := fs.String("update", "", "merge mid_edit_test_s into this inputs JSON")
	wait := fs.Duration("wait", 40*time.Minute, "max wait for a quiet host before each run")
	noisyFlag := fs.Bool("allow-noisy-host", false, "OWNER-AUTHORISED OVERRIDE: measure under load instead of waiting for a quiet host (also BZL_ALLOW_NOISY_HOST=1); results are marked not_quiet")
	_ = fs.Parse(args)
	allowNoisy := AllowNoisy(*noisyFlag, os.Getenv(noisyEnv))
	if *file == "" || strings.HasSuffix(*file, "_test.go") {
		return fmt.Errorf("-file must be a non-test file")
	}
	path := filepath.Join(*root, *file)
	orig, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	defer func() { _ = os.WriteFile(path, orig, 0o644) }()
	test := func() error {
		_, err := bazelOut(*root, *bazel, "test", *pattern)
		return err
	}
	if err := test(); err != nil { // warm the action cache on the unedited tree
		return fmt.Errorf("warm-up: %w", err)
	}
	type sample struct {
		Run      int     `json:"run"`
		WallS    float64 `json:"wall_s"`
		LoadPre  float64 `json:"load_pre"`
		LoadPost float64 `json:"load_post"`
		Quiet    bool    `json:"quiet"`
		NotQuiet bool    `json:"not_quiet"`
		MemPre   float64 `json:"mem_avail_gb_pre"`
		MemPost  float64 `json:"mem_avail_gb_post"`
	}
	var ss []sample
	var ts []float64
	for i := 1; i <= *runs; i++ {
		var pre float64
		var ok bool
		if allowNoisy {
			pre = loadNow()
			ok = !IsNoisy(pre, pre)
			fmt.Fprintf(os.Stderr, "economic-midpoint: NOISY HOST OVERRIDE: run %d at 1-min load %.2f\n", i, pre)
		} else {
			pre, ok = waitQuiet(*wait)
		}
		memPre := memAvailGB()
		edited := append(append([]byte(nil), orig...), []byte(fmt.Sprintf("\n// midpoint edit %d\n", i))...)
		if err := os.WriteFile(path, edited, 0o644); err != nil {
			return err
		}
		t0 := time.Now()
		if err := test(); err != nil {
			return fmt.Errorf("run %d: %w", i, err)
		}
		d := time.Since(t0).Seconds()
		post := loadNow()
		ss = append(ss, sample{i, d, pre, post, ok, IsNoisy(pre, post), memPre, memAvailGB()})
		ts = append(ts, d)
	}
	med := median(ts)
	entry := map[string]any{"not_quiet": false, "source": "economic-midpoint t4", "authorisation": "owner, measured under load; never a quiet-host number",
		"load1_pre_max": math.Inf(-1), "load1_post_max": math.Inf(-1), "mem_avail_gb_min": math.Inf(1)}
	for _, s := range ss {
		if s.NotQuiet {
			entry["not_quiet"] = true
		}
		entry["load1_pre_max"] = math.Max(entry["load1_pre_max"].(float64), s.LoadPre)
		entry["load1_post_max"] = math.Max(entry["load1_post_max"].(float64), s.LoadPost)
		entry["mem_avail_gb_min"] = math.Min(entry["mem_avail_gb_min"].(float64), math.Min(s.MemPre, s.MemPost))
	}
	noisy := entry["not_quiet"] == true
	if *out != "" {
		b, _ := json.MarshalIndent(map[string]any{"samples": ss, "median_s": med, "not_quiet": noisy, "allow_noisy_host": allowNoisy, "summary": entry}, "", "  ")
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	if *update != "" {
		if err := MergeNoisy(*update, map[string]any{"mid_edit_test_s": med}, noisy, entry); err != nil {
			return err
		}
	}
	tag := ""
	if noisy {
		tag = " [NOISY: measured under load, not a quiet-host number]"
	}
	fmt.Printf("T4 median edit-build-test %.2f s over %d runs%s\n", med, len(ts), tag)
	return nil
}

func memAvailGB() float64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return math.NaN()
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(ln, "MemAvailable:"); ok {
			var kb float64
			if _, err := fmt.Sscan(v, &kb); err == nil {
				return kb / 1024 / 1024
			}
		}
	}
	return math.NaN()
}

func readJSON(path string) map[string]any {
	m := map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	inputs := fs.String("inputs", defaultInputs, "spike-measurements.json")
	measured := fs.String("measured", "tools/bzl/baseline/measured.json", "measured.json")
	_ = fs.Parse(args)
	i, m := readJSON(*inputs), readJSON(*measured)
	base := Num(i, "baseline_edit_test_s")
	if math.IsNaN(base) {
		base = Num(m, "warm_one_pkg_test_s")
	}
	vs := []Verdict{
		EvalT1(Num(i, "mid_t1_ratio"), Num(m, "ra1_median_fanout")),
		EvalT3(Num(i, "mid_second_cold_s"), Num(i, "mid_first_cold_s")),
		EvalT4(Num(i, "mid_edit_test_s"), base),
	}
	for _, v := range vs {
		fmt.Println(v.Line() + noisyTagFor(v.ID, i))
		fmt.Fprintf(os.Stderr, "  %s %s\n", v.ID, v.Detail)
	}
	return nil
}

// noisyTagFor tags a verdict derived from any input listed under noisy_inputs.
func noisyTagFor(id string, inputs map[string]any) string {
	keys := map[string][]string{"T1": {"mid_t1_ratio"}, "T3": {"mid_second_cold_s", "mid_first_cold_s"}, "T4": {"mid_edit_test_s"}}
	for _, k := range keys[id] {
		if t := NoisyTag(inputs, k); t != "" {
			return t
		}
	}
	return ""
}
