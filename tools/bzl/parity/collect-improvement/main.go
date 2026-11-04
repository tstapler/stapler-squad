// Command collect-improvement produces the measurements JSON that tools/bzl/parity/improvement evaluates
// (Task 7.5g): one subcommand per collector, all merging into --out (default bazel-improvement.json).
//
//	t1  affected-targets replay over the sampled partial-change PRs
//	t2  time-to-green and runner-minutes p50 from `gh api` (partial class and whole population)
//	t2b warm remote-cache hit ratio median from the bazel-trend JSONL
//	t3  second-worktree cold and per-worktree disk from an `s6measure cold` result
//	t4  edit-build-test latency of a config/ file
//	t6  manual regen steps per agent session from transcripts (reported only before the flip)
//	t7  hermeticity from `hermeticity.sh` outputs (Bazel vs make baseline)
//	t8  e2e work avoided, via tools/bzl/e2eavoided
//
// A timing row (T3 second-cold, T4, T7 build) is never emitted from a host whose 1-minute load exceeds 2.0
// (--require-quiet, default true): the row is recorded UNMEASURED with the reason NOT QUIET in "_meta".
// T1, T2, T6 and T8 replay recorded history and need no quiet host.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"github.com/tstapler/stapler-squad/tools/bzl/parity/ghci"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, collect.ProcHost{}, ghci.ExecGH{})) }

func run(args []string, stdout, stderr io.Writer, host collect.Host, gh ghci.GH) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: collect-improvement t1|t2|t3|t4|t6|t7|t8 [flags]")
		return 2
	}
	sub := args[0]
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "bazel-improvement.json", "measurements JSON (merged)")
	require := fs.Bool("require-quiet", true, "refuse timing samples when 1-minute load > 2.0")
	baseWT := fs.String("baseline-worktree", "tools/bzl/baseline/baseline-worktree.json", "make worktree baseline")
	measured := fs.String("measured", "tools/bzl/baseline/measured.json", "make CI baseline")
	sample := fs.String("pr-sample", "tools/bzl/baseline/pr-sample.json", "pr-sample.json")
	root := fs.String("root", ".", "repository root")
	repo := fs.String("repo", "tstapler/stapler-squad", "owner/repo")
	fixture := fs.String("gh-fixture", "", "serve gh responses from this fixture instead of calling gh")
	g := collect.NewGate(host, *require)
	d := collect.NewDoc()
	var err error
	setup := func() bool { // flags are parsed per subcommand below; this applies shared ones afterwards
		g.Require = *require
		if *fixture != "" {
			var fk *ghci.FakeGH
			if fk, err = ghci.LoadFake(*fixture); err != nil {
				return false
			}
			gh = fk
		}
		return true
	}
	switch sub {
	case "t1":
		affected := fs.String("affected", "", "built tools/ci/affected-targets binary (required)")
		bazel := fs.String("bazel", "tools/bazel", "bazel launcher")
		tmp := fs.String("tmpdir", os.Getenv("TMPDIR"), "scratch dir for name-status files")
		if fs.Parse(args[1:]) != nil || *affected == "" {
			fmt.Fprintln(stderr, "t1: -affected is required")
			return 2
		}
		var s PRSample
		if s, err = loadSample(*sample); err == nil {
			rp := ExecReplayer{Root: *root, Bazel: abs(*bazel, *root), Affected: *affected, TmpDir: *tmp}
			err = MeasureT1(d, rp, s, func(p string) bool { _, e := os.Stat(filepath.Join(*root, p)); return e == nil })
		}
	case "t2":
		wfs := fs.String("workflows", "bazel.yml,lint.yml,mcp-integration.yml", "workflow files forming time-to-green (Bazel set)")
		n := fs.Int("n", 30, "newest PR head SHAs")
		minRuns := fs.Int("min-runs", 20, "minimum qualifying PR runs per class")
		if fs.Parse(args[1:]) != nil || !setup() {
			return finish(err, 2, stderr)
		}
		var s PRSample
		if s, err = loadSample(*sample); err == nil {
			err = MeasureT2(d, gh, *repo, strings.Split(*wfs, ","), *n, *minRuns, s.GlobalRegex)
		}
	case "t2b":
		trend := fs.String("trend", "", "bazel-trend JSONL (required)")
		last := fs.Int("last", 20, "warm main runs")
		if fs.Parse(args[1:]) != nil || *trend == "" {
			fmt.Fprintln(stderr, "t2b: -trend is required")
			return 2
		}
		var m float64
		var n int
		var ok bool
		if m, n, ok, err = collect.WarmMainMedian(*trend, *last); err == nil {
			if ok {
				d.Set("T2b", m, collect.Meta{Source: *trend})
			} else {
				d.Unmeasured("T2b", fmt.Sprintf("only %d warm main runs, need %d", n, *last), collect.Meta{Source: *trend})
			}
		}
	case "t3":
		s6 := fs.String("s6", "", "result JSON of `s6measure cold` (required)")
		if fs.Parse(args[1:]) != nil || *s6 == "" {
			fmt.Fprintln(stderr, "t3: -s6 is required")
			return 2
		}
		g.Require = *require
		var res collect.S6Result
		var base collect.WorktreeBaseline
		if res, err = collect.LoadS6(*s6); err == nil {
			if base, err = collect.LoadWorktreeBaseline(*baseWT); err == nil {
				MeasureT3(d, res, base, g, "s6measure "+*s6)
			}
		}
	case "t4":
		file := fs.String("file", "", "non-test Go file in config/ to edit (required)")
		runs := fs.Int("runs", 5, "timed runs")
		target := fs.String("target", "//config:all", "bazel test target")
		jobs := fs.Int("jobs", 4, "--jobs")
		mem := fs.Int("memory-mb", 3000, "--local_resources=memory")
		if fs.Parse(args[1:]) != nil || *file == "" || strings.HasSuffix(*file, "_test.go") {
			fmt.Fprintln(stderr, "t4: -file must name a non-test file")
			return 2
		}
		g.Require = *require
		bz, _ := exec.LookPath("bazelisk")
		r := &FileEditRunner{Path: filepath.Join(*root, *file), Dir: *root, Env: []string{"BAZEL_REAL=" + bz},
			Argv: []string{filepath.Join(*root, "tools", "bazel"), "test", *target, fmt.Sprintf("--jobs=%d", *jobs), fmt.Sprintf("--local_resources=memory=%d", *mem)}}
		MeasureT4(d, r, g, *runs, fmt.Sprintf("median of %d edit+`bazel test %s` runs editing %s", *runs, *target, *file))
	case "t6":
		glob := fs.String("glob", os.ExpandEnv("$HOME/.claude/projects/-home-tstapler-Programming-stapler-squad*/*.jsonl"), "transcript glob")
		since := fs.String("since", "", "flip date YYYY-MM-DD; absent means pre-flip, so UNMEASURED")
		n := fs.Int("n", 30, "newest sessions")
		if fs.Parse(args[1:]) != nil {
			return 2
		}
		var t time.Time
		if *since != "" {
			if t, err = time.Parse("2006-01-02", *since); err != nil {
				return finish(err, 2, stderr)
			}
		}
		var m collect.MeasuredBaseline
		if m, err = collect.LoadMeasured(*measured); err == nil {
			err = MeasureT6(d, *glob, t, *n, m.RegenMedian)
		}
	case "t7":
		bz := fs.String("bazel-result", "", "hermeticity.sh --system bazel output (required)")
		mk := fs.String("make-result", "tools/bzl/baseline/hermeticity-make.json", "make baseline")
		allow := fs.String("allow-list", "", "file of allow-listed host executables (Bazel prerequisites), one per line")
		att := fs.String("attestations", "project_plans/bazel-conversion/implementation/owner-attestations.yaml", "owner attestations (read only)")
		if fs.Parse(args[1:]) != nil || *bz == "" {
			fmt.Fprintln(stderr, "t7: -bazel-result is required")
			return 2
		}
		g.Require = *require
		var hb, hm Hermeticity
		var al map[string]bool
		var acc int
		if hb, err = loadHermeticity(*bz); err == nil {
			if hm, err = loadHermeticity(*mk); err == nil {
				if al, err = readAllow(*allow); err == nil {
					if acc, err = AcceptedDowngrades(*att); err == nil {
						MeasureT7(d, hb, hm, al, acc, g, "hermeticity "+*bz)
					}
				}
			}
		}
	case "t8":
		bin := fs.String("e2eavoided", "", "built tools/bzl/e2eavoided binary (required)")
		samples := fs.String("samples", "", "e2eavoided -samples file (gh api)")
		fx := fs.String("e2e-fixture", "", "e2eavoided -fixture file")
		if fs.Parse(args[1:]) != nil || *bin == "" || (*samples == "") == (*fx == "") {
			fmt.Fprintln(stderr, "t8: -e2eavoided and exactly one of -samples or -e2e-fixture are required")
			return 2
		}
		argv := []string{"-fixture", *fx}
		if *samples != "" {
			argv = []string{"-samples", *samples, "-repo", *repo}
		}
		o, _ := exec.Command(*bin, argv...).Output() //nolint:norawexec // exit 1 is a FAIL verdict, output is parsed
		MeasureT8(d, string(o))
	default:
		fmt.Fprintf(stderr, "collect-improvement: unknown subcommand %q\n", sub)
		return 2
	}
	if err != nil {
		return finish(err, 1, stderr)
	}
	if err := collect.Merge(*out, d); err != nil {
		return finish(err, 1, stderr)
	}
	for _, k := range d.Keys() {
		if v, ok := d.Value(k); ok {
			fmt.Fprintf(stdout, "%s = %g\n", k, v)
		} else {
			fmt.Fprintf(stdout, "%s UNMEASURED (%s)\n", k, d.Meta(k).Reason)
		}
	}
	return 0
}

func abs(p, root string) string {
	if filepath.IsAbs(p) || !strings.Contains(p, "/") {
		return p
	}
	return filepath.Join(root, p)
}

func finish(err error, code int, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, "collect-improvement:", err)
	}
	return code
}
