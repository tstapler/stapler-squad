// Command run-all is the parity gate P1-P10 orchestrator (Task 7.5a).
//
// It runs each row's checks, applies validated owner waivers to the two waivable P10 sub-checks only, and
// prints `PARITY: PASS`, `PARITY: PASS WITH WAIVERS (...)` or `PARITY: FAIL P<n>`. A row whose check cannot
// run (no tool yet, owner attestation absent) is UNMEASURED and counts as a failure: nothing passes by default.
//
// Tool labels (//pkg:name) resolve to built binaries under --bazel-bin so this command can itself be started
// with `bazel run` without a nested Bazel invocation; build them first with `bazel build //tools/bzl/parity/...`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/waivers"
	"gopkg.in/yaml.v3"
)

// Check is one command of a row. WaiverRow names the waiver that can excuse a failure; Attest names an owner
// attestation (a key of `parity_attestations:`) that replaces the command; an empty Cmd with no Attest is UNMEASURED.
type Check struct {
	Name      string
	Cmd       []string
	WaiverRow string
	Attest    string
	// Needs is a collector output file; when it is absent the check is UNMEASURED rather than run.
	Needs string
}

// Row is one parity row.
type Row struct {
	ID     string
	Desc   string
	Checks []Check
}

const (
	scripts = "tools/bzl/parity/"
	pass    = "PASS"
	fail    = "FAIL"
	waived  = "WAIVED"
	unmeas  = "UNMEASURED"
)

// Collector outputs: written by tools/bzl/parity/perf, collect-improvement and bazel-trend, read here. A missing
// file makes its check UNMEASURED.
const (
	PerfFile        = "bazel-perf.json"
	ImprovementFile = "bazel-improvement.json"
	TrendFile       = "bazel-trend.jsonl"
)

func perf(checks string) []string {
	return []string{"//tools/bzl/parity/perf", "check", "-perf", PerfFile, "-check", checks}
}

// exitUnmeasured is the exit code a collector-backed check uses for "no verdict number".
const exitUnmeasured = 4

func sh(script string, args ...string) []string {
	return append([]string{"bash", scripts + script}, args...)
}

// Rows is the P1-P10 table of plan Story 7.5.
func Rows() []Row {
	diff := func(cfg string) Check { return Check{Name: "test-set-diff " + cfg, Cmd: sh("test-set-diff.sh", cfg)} }
	return []Row{
		{"P1", "test-set parity (default tags)", []Check{diff("default")}},
		{"P2", "tag-matrix parity (race, integration, sinkguard, gitoracle)", []Check{diff("race"), diff("integration"), diff("sinkguard"), diff("gitoracle")}},
		{"P3", "gazelle idempotence twice and fresh clone", []Check{
			{Name: "gazelle diff 1", Cmd: []string{"bazel", "run", "//:gazelle", "--", "-mode=diff"}},
			{Name: "gazelle diff 2", Cmd: []string{"bazel", "run", "//:gazelle", "--", "-mode=diff"}},
			{Name: "fresh clone", Cmd: []string{"//tools/bzl/parity/fresh-clone"}}}},
		{"P4", "generated-code equivalence", []Check{
			{Name: "compare-proto", Cmd: sh("compare-proto.sh")}, {Name: "compare-ent", Cmd: sh("compare-ent.sh")}}},
		{"P5", "binary behaviour", []Check{{Name: "compare-binary", Cmd: sh("compare-binary.sh")}}},
		{"P6", "web parity", []Check{{Name: "compare-web", Cmd: sh("compare-web.sh")}}},
		{"P7", "e2e pass counts and /metrics names (Playwright run is a workflow step; e2e-compare judges its outputs)", []Check{
			{Name: "e2e-compare", Cmd: []string{"//tools/bzl/parity/e2e-compare", "--from-env"}}}},
		{"P8", "macOS signed binary keeps TCC grants (owner checklist)", []Check{{Name: "owner attestation P8", Attest: "P8"}}},
		{"P9", "deploy dry run on the isolated manual instance", []Check{
			{Name: "manual-instance smoke", Cmd: []string{"//tools/bzl/manual-instance", "--smoke"}}}},
		{"P10", "performance and improvement targets", []Check{
			{Name: "improvement T1-T8 (collect-improvement output)", Cmd: []string{"//tools/bzl/parity/improvement", "-measurements", ImprovementFile}, Needs: ImprovementFile},
			{Name: "cache hit-ratio floor", Cmd: []string{"//tools/ci/bazel-trend", "hit-floor", "-trend", TrendFile}, WaiverRow: "P10.hit_ratio_floor", Needs: TrendFile},
			{Name: "disk ceiling (perf collector)", Cmd: perf("disk"), WaiverRow: "P10.disk_ceiling", Needs: PerfFile},
			{Name: "warm no-op build, cold row, per-workflow p50 (perf collector)", Cmd: perf("warm,cold,ttg"), Needs: PerfFile}}},
	}
}

// Env is everything run-all needs from the outside, injectable for tests.
type Env struct {
	Run     func(cmd []string) int
	Attest  func(key string) bool
	Waivers []waivers.Waiver
	DryRun  bool
	Exists  func(path string) bool
}

// RowResult is the outcome of one row.
type RowResult struct {
	Row    Row
	Status string
	Notes  []string
}

// Execute evaluates every row.
func Execute(rows []Row, env Env) []RowResult {
	out := make([]RowResult, 0, len(rows))
	for _, r := range rows {
		res := RowResult{Row: r, Status: pass}
		for _, c := range r.Checks {
			st := checkStatus(c, env)
			if st == fail || st == unmeas {
				if w, ok := waivers.Active(env.Waivers, c.WaiverRow); ok && c.WaiverRow != "" {
					st = waived
					res.Notes = append(res.Notes, "waived "+w.Row)
				}
			}
			switch st {
			case fail, unmeas:
				res.Status = worse(res.Status, st)
				res.Notes = append(res.Notes, fmt.Sprintf("%s %s", st, c.Name))
			}
		}
		out = append(out, res)
	}
	return out
}

func worse(a, b string) string {
	if a == fail || b == fail {
		return fail
	}
	return unmeas
}

func checkStatus(c Check, env Env) string {
	if env.DryRun {
		return pass
	}
	switch {
	case c.Attest != "":
		if env.Attest(c.Attest) {
			return pass
		}
		return unmeas
	case len(c.Cmd) == 0:
		return unmeas
	case c.Needs != "" && !env.Exists(c.Needs):
		return unmeas
	}
	switch env.Run(c.Cmd) {
	case 0:
		return pass
	case exitUnmeasured:
		return unmeas
	}
	return fail
}

// Verdict returns the PARITY line and whether the gate passed.
func Verdict(results []RowResult, active []waivers.Waiver) (string, bool) {
	var failed []string
	for _, r := range results {
		if r.Status != pass {
			failed = append(failed, r.Row.ID)
		}
	}
	if len(failed) > 0 {
		return "PARITY: FAIL " + strings.Join(failed, ", "), false
	}
	if len(active) > 0 {
		return "PARITY: PASS WITH WAIVERS (" + waivers.Inline(active) + ")", true
	}
	return "PARITY: PASS", true
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run-all", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dry := fs.Bool("dry-run", false, "list the rows and treat every check as passing (commands are not executed)")
	attFile := fs.String("attestations", "project_plans/bazel-conversion/implementation/owner-attestations.yaml", "owner attestations file")
	wfile := fs.String("waivers", "", "file holding `waivers:` (default: the attestations file)")
	today := fs.String("today", "", "override today (YYYY-MM-DD), for tests")
	bin := fs.String("bazel-bin", "bazel-bin", "directory holding built tool binaries")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	now := time.Now().UTC()
	if *today != "" {
		t, err := time.Parse("2006-01-02", *today)
		if err != nil {
			fmt.Fprintln(stderr, "run-all: bad --today:", err)
			return 2
		}
		now = t
	}
	if *wfile == "" {
		*wfile = *attFile
	}
	ws, bad, err := loadWaivers(*wfile, now)
	if err != nil {
		fmt.Fprintln(stderr, "run-all:", err)
		return 2
	}
	fmt.Fprint(stdout, waivers.Block(ws))
	if len(bad) > 0 {
		for _, p := range bad {
			fmt.Fprintf(stdout, "PARITY: FAIL WAIVER %s (%s)\n", p.Row, p.Reason)
		}
		return 1
	}
	env := Env{Waivers: ws, DryRun: *dry, Attest: attestor(*attFile), Run: runner(*bin, stdout, stderr), Exists: fileExists}
	rows := Rows()
	results := Execute(rows, env)
	for _, r := range results {
		note := ""
		if len(r.Notes) > 0 {
			note = " [" + strings.Join(r.Notes, "; ") + "]"
		}
		fmt.Fprintf(stdout, "%s: %s %s%s\n", r.Row.ID, r.Status, r.Row.Desc, note)
	}
	if *dry {
		fmt.Fprintln(stdout, "DRY-RUN: no check command was executed; every row is assumed to pass")
	}
	line, ok := Verdict(results, ws)
	fmt.Fprintln(stdout, line)
	if !ok {
		return 1
	}
	return 0
}

func loadWaivers(path string, now time.Time) ([]waivers.Waiver, []waivers.Problem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	all, err := waivers.Parse(f)
	if err != nil {
		return nil, nil, err
	}
	ok, bad := waivers.Validate(all, now)
	return ok, bad, nil
}

func attestor(path string) func(string) bool {
	return func(key string) bool {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		var doc struct {
			Attestations map[string]struct {
				Status string `yaml:"status"`
			} `yaml:"parity_attestations"`
		}
		if yaml.Unmarshal(raw, &doc) != nil {
			return false
		}
		return doc.Attestations[key].Status == "attested"
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func runner(bin string, stdout, stderr io.Writer) func([]string) int {
	return func(cmd []string) int {
		argv := append([]string(nil), cmd...)
		if strings.HasPrefix(argv[0], "//") {
			pkg := strings.TrimPrefix(argv[0], "//")
			name := filepath.Base(pkg)
			argv[0] = filepath.Join(bin, pkg, name+"_", name)
		}
		c := exec.Command(argv[0], argv[1:]...)
		c.Stdout, c.Stderr = stdout, stderr
		if err := c.Run(); err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode()
			}
			return 1
		}
		return 0
	}
}
