package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// run: `bazel-trend [flags] bep.json...` summarises one CI job; `bazel-trend hit-floor -trend f` checks 7.4c.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "hit-floor" {
		return runHitFloor(args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("bazel-trend", flag.ContinueOnError)
	fs.SetOutput(stderr)
	prom := fs.Bool("prom", false, "print `name value` lines and ::warning:: annotations instead of JSON")
	logPath := fs.String("log", "", "command.log to scan for cache auth failures")
	affected := fs.Int("affected-target-count", 0, "affected target count from affected-targets")
	appendTo := fs.String("append", "", "append the row to this trend JSONL file")
	runID := fs.String("run-id", "", "run id recorded in the trend row")
	ref := fs.String("ref", "", "git ref recorded in the trend row")
	warm := fs.Bool("warm", false, "mark the run as warm (for the hit-ratio floor)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "bazel-trend: need at least one BEP file")
		return 2
	}
	s := Summary{AffectedTargetCount: *affected}
	for _, p := range fs.Args() {
		f, err := os.Open(p)
		if err != nil {
			fmt.Fprintln(stderr, "bazel-trend:", err)
			return 2
		}
		err = s.ParseBEP(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(stderr, "bazel-trend: %s: %v\n", p, err)
			return 2
		}
	}
	if *logPath != "" {
		f, err := os.Open(*logPath)
		if err != nil {
			fmt.Fprintln(stderr, "bazel-trend:", err)
			return 2
		}
		err = s.AddLog(f)
		f.Close()
		if err != nil {
			fmt.Fprintln(stderr, "bazel-trend:", err)
			return 2
		}
	}
	s.Finish()
	row := TrendRow{RunID: *runID, Ref: *ref, Warm: *warm, Summary: s}
	if *appendTo != "" {
		if err := appendRow(*appendTo, row); err != nil {
			fmt.Fprintln(stderr, "bazel-trend:", err)
			return 2
		}
	}
	if *prom {
		fmt.Fprint(stdout, s.Prom())
		return 0
	}
	b, _ := json.Marshal(s)
	fmt.Fprintln(stdout, string(b))
	return 0
}

func appendRow(path string, row TrendRow) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(row)
	_, err = fmt.Fprintln(f, string(b))
	return err
}

func runHitFloor(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hit-floor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	trend := fs.String("trend", "", "trend JSONL file")
	last := fs.Int("last", 20, "number of warm main runs")
	floor := fs.Float64("floor", 0.60, "minimum median hit ratio")
	if err := fs.Parse(args); err != nil || *trend == "" {
		return 2
	}
	f, err := os.Open(*trend)
	if err != nil {
		fmt.Fprintln(stderr, "bazel-trend:", err)
		return 2
	}
	defer f.Close()
	var rows []TrendRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r TrendRow
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			rows = append(rows, r)
		}
	}
	m, ok, enough := HitRatioFloor(rows, *last, *floor)
	switch {
	case !enough:
		fmt.Fprintf(stdout, "hit_ratio_floor UNMEASURED (fewer than %d warm main runs)\n", *last)
		return 4
	case !ok:
		fmt.Fprintf(stdout, "::warning title=Cache hit ratio::median %.3f over last %d warm main runs is below floor %.2f\n", m, *last, *floor)
		return 1
	}
	fmt.Fprintf(stdout, "hit_ratio_floor PASS median=%.3f\n", m)
	return 0
}
