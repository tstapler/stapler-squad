// Command perf is the P10 performance collector (Task 7.5d). It writes measurement rows to a JSON file
// (default bazel-perf.json) and `perf check` judges them against the P10 non-regression bounds.
//
//	perf warm      warm no-op `bazel build //:stapler-squad` x5, median
//	perf disk      per-worktree output_base disk vs baseline-worktree.json
//	perf worktree  second-worktree cold build, from an `s6measure cold` result
//	perf ttg       PR time-to-green and per-workflow p50 from `gh api` (read-only)
//	perf cache     cache hit ratio from BEP files (via bazel-trend), an execution log or a trend file
//	perf check     exit 0 PASS, 1 FAIL, 4 UNMEASURED
//
// Timing subcommands refuse to run on a host whose 1-minute load exceeds 2.0 (--require-quiet, default true)
// and record the row as UNMEASURED with the reason NOT QUIET; they never emit a number from a noisy host.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"github.com/tstapler/stapler-squad/tools/bzl/parity/ghci"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, collect.ProcHost{}, ghci.ExecGH{})) }

type opts struct {
	out, baseWT, measured string
	require               bool
}

func run(args []string, stdout, stderr io.Writer, host collect.Host, gh ghci.GH) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: perf warm|disk|worktree|ttg|cache|check [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("perf "+sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o opts
	fs.StringVar(&o.out, "out", "bazel-perf.json", "measurements JSON (merged)")
	fs.StringVar(&o.baseWT, "baseline-worktree", "tools/bzl/baseline/baseline-worktree.json", "make worktree baseline")
	fs.StringVar(&o.measured, "measured", "tools/bzl/baseline/measured.json", "make CI baseline")
	fs.BoolVar(&o.require, "require-quiet", true, "refuse timing samples when 1-minute load > 2.0")
	d := collect.NewDoc()
	g := collect.NewGate(host, true)
	var err error
	switch sub {
	case "warm":
		dir := fs.String("dir", ".", "worktree to build in")
		target := fs.String("target", "//:stapler-squad", "target")
		runs := fs.Int("runs", 5, "timed runs")
		jobs := fs.Int("jobs", 4, "--jobs")
		mem := fs.Int("memory-mb", 3000, "--local_resources=memory")
		if fs.Parse(rest) != nil {
			return 2
		}
		g.Require = o.require
		bz, _ := exec.LookPath("bazelisk")
		MeasureWarm(d, BazelBuilder{Dir: *dir, Bazel: bz, Jobs: *jobs, MemMB: *mem}, g, *target, *runs)
	case "disk":
		ob := fs.String("output-base", "", "bazel output_base (see `bazel info output_base`)")
		if fs.Parse(rest) != nil || *ob == "" {
			fmt.Fprintln(stderr, "perf disk: -output-base is required")
			return 2
		}
		var base collect.WorktreeBaseline
		if base, err = collect.LoadWorktreeBaseline(o.baseWT); err == nil {
			MeasureDisk(d, DuKB, *ob, base)
		}
	case "worktree":
		s6 := fs.String("s6", "", "result JSON of `s6measure cold`")
		if fs.Parse(rest) != nil || *s6 == "" {
			fmt.Fprintln(stderr, "perf worktree: -s6 is required")
			return 2
		}
		g.Require = o.require
		var res collect.S6Result
		var base collect.WorktreeBaseline
		if res, err = collect.LoadS6(*s6); err == nil {
			if base, err = collect.LoadWorktreeBaseline(o.baseWT); err == nil {
				MeasureWorktree(d, res, base, g, "s6measure "+*s6)
			}
		}
	case "ttg":
		repo := fs.String("repo", "tstapler/stapler-squad", "owner/repo")
		wfs := fs.String("workflows", "build.yml,lint.yml,mcp-integration.yml", "workflow files forming time-to-green")
		n := fs.Int("n", 30, "newest PR head SHAs")
		fixture := fs.String("gh-fixture", "", "serve gh responses from this fixture instead of calling gh")
		if fs.Parse(rest) != nil {
			return 2
		}
		if *fixture != "" {
			var fk *ghci.FakeGH
			if fk, err = ghci.LoadFake(*fixture); err == nil {
				gh = fk
			}
		}
		if err == nil {
			var base collect.MeasuredBaseline
			if base, err = collect.LoadMeasured(o.measured); err == nil {
				err = MeasureTTG(d, gh, *repo, strings.Split(*wfs, ","), *n, base)
			}
		}
	case "cache":
		err = runCache(rest, fs, d)
	case "check":
		which := fs.String("check", "all", "warm|cold|ttg|disk|all (comma separated)")
		file := fs.String("perf", "bazel-perf.json", "measurements JSON")
		if fs.Parse(rest) != nil {
			return 2
		}
		pf, lerr := loadPerf(*file)
		if lerr != nil {
			fmt.Fprintf(stdout, "UNMEASURED: %v\n", lerr)
			return exitUnmeasured
		}
		return Check(pf, strings.Split(*which, ","), stdout)
	default:
		fmt.Fprintf(stderr, "perf: unknown subcommand %q\n", sub)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "perf:", err)
		return 1
	}
	if err := collect.Merge(o.out, d); err != nil {
		fmt.Fprintln(stderr, "perf:", err)
		return 1
	}
	for _, k := range d.Keys() {
		m := d.Meta(k)
		if v, ok := d.Value(k); ok {
			fmt.Fprintf(stdout, "%s = %g\n", k, v)
		} else {
			fmt.Fprintf(stdout, "%s UNMEASURED (%s)\n", k, m.Reason)
		}
	}
	return 0
}

func runCache(rest []string, fs *flag.FlagSet, d *collect.Doc) error {
	execLog := fs.String("exec-log", "", "execution_log_json_file of one build")
	trend := fs.String("trend", "", "bazel-trend JSONL for the warm-main median")
	last := fs.Int("last", 20, "warm main runs for the median")
	bt := fs.String("bazel-trend", "", "path of the built bazel-trend binary (for BEP files)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	did := false
	if *execLog != "" {
		did = true
		f, err := os.Open(*execLog)
		if err != nil {
			return err
		}
		c, err := collect.ParseExecLog(f)
		f.Close()
		if err != nil {
			return err
		}
		setRatio(d, "cache_hit_ratio", c, "execution log "+*execLog)
	}
	if fs.NArg() > 0 {
		did = true
		if *bt == "" {
			return fmt.Errorf("BEP files need -bazel-trend <binary>")
		}
		s, err := collect.RunBazelTrend(*bt, nil, fs.Args()...)
		if err != nil {
			return err
		}
		setRatio(d, "cache_hit_ratio", collect.CacheCount{Hits: s.CacheHits, Spawns: s.Spawns}, "BEP via bazel-trend")
		d.Set("bep_attempts", float64(s.Attempts), collect.Meta{})
	}
	if *trend != "" {
		did = true
		m, n, ok, err := collect.WarmMainMedian(*trend, *last)
		switch {
		case err != nil:
			return err
		case !ok:
			d.Unmeasured("cache_hit_ratio_warm_main_median", fmt.Sprintf("only %d warm main rows, need %d", n, *last), collect.Meta{Source: *trend})
		default:
			d.Set("cache_hit_ratio_warm_main_median", m, collect.Meta{Source: *trend})
		}
	}
	if !did {
		return fmt.Errorf("give -exec-log, BEP files or -trend")
	}
	return nil
}

func setRatio(d *collect.Doc, key string, c collect.CacheCount, src string) {
	if r, ok := c.Ratio(); ok {
		d.Set(key, r, collect.Meta{Source: src})
		return
	}
	d.Unmeasured(key, "no spawns in "+src, collect.Meta{Source: src})
}
