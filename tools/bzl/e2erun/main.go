// Command e2erun is the runner behind every e2e_test target (Task 12.2b): it runs one spec group through
// Playwright with the JSON reporter and applies the baseline comparator, so the Bazel verdict is "no failure
// outside the baseline and expected-diff lists" (ADR-009 decision 5), not the raw Playwright exit code.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type pwResult struct {
	Retry  int    `json:"retry"`
	Status string `json:"status"`
}
type pwTest struct {
	ProjectName string     `json:"projectName"`
	Results     []pwResult `json:"results"`
}
type pwSpec struct {
	File  string   `json:"file"`
	Title string   `json:"title"`
	Tests []pwTest `json:"tests"`
}
type pwSuite struct {
	Title  string    `json:"title"`
	Specs  []pwSpec  `json:"specs"`
	Suites []pwSuite `json:"suites"`
}
type pwReport struct {
	Suites []pwSuite `json:"suites"`
}

func key(spec, project, title string) string { return spec + " :: " + project + " :: " + title }

// failures lists tests whose final attempt failed (a pass on retry is flaky, handled by the baseline, not here).
func failures(r pwReport) []string {
	var out []string
	var walk func(s pwSuite, titles []string)
	walk = func(s pwSuite, titles []string) {
		for _, sp := range s.Specs {
			for _, t := range sp.Tests {
				if len(t.Results) == 0 {
					continue
				}
				switch t.Results[len(t.Results)-1].Status {
				case "failed", "timedOut", "interrupted":
					out = append(out, key(sp.File, t.ProjectName, strings.Join(append(append([]string{}, titles...), sp.Title), " > ")))
				}
			}
		}
		for _, c := range s.Suites {
			next := titles
			if c.Title != "" && !strings.HasSuffix(c.Title, ".spec.ts") {
				next = append(append([]string{}, titles...), c.Title)
			}
			walk(c, next)
		}
	}
	for _, s := range r.Suites {
		walk(s, nil)
	}
	return out
}

type listed struct {
	Spec    string `json:"spec"`
	Project string `json:"project"`
	Title   string `json:"title"`
	Class   string `json:"class"`
}

// allowed builds the set of keys the comparator tolerates: baseline `fail`/`flaky` rows and the expected-diff entries.
func allowed(baseline, expected []byte) (map[string]bool, error) {
	set := map[string]bool{}
	var b struct {
		Specs []listed `json:"specs"`
	}
	if err := json.Unmarshal(baseline, &b); err != nil {
		return nil, err
	}
	for _, s := range b.Specs {
		if s.Class == "fail" || s.Class == "flaky" {
			set[key(s.Spec, s.Project, s.Title)] = true
		}
	}
	if len(expected) > 0 {
		var e struct {
			Entries []listed `json:"entries"`
		}
		if err := json.Unmarshal(expected, &e); err != nil {
			return nil, err
		}
		for _, s := range e.Entries {
			set[key(s.Spec, s.Project, s.Title)] = true
		}
	}
	return set, nil
}

// verdict returns the failures that are not tolerated; empty means the target passes.
func verdict(report pwReport, tolerated map[string]bool) []string {
	var bad []string
	for _, f := range failures(report) {
		if !tolerated[f] {
			bad = append(bad, f)
		}
	}
	return bad
}

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("e2erun", flag.ContinueOnError)
	fs.SetOutput(errw)
	var specs multi
	fs.Var(&specs, "spec", "spec file under tests/e2e (repeatable)")
	target := fs.String("target", "e2e", "target name, for messages")
	root := fs.String("root", firstNonEmpty(os.Getenv("E2E_REPO_ROOT"), "."), "repository root with tests/e2e/node_modules")
	report := fs.String("report", "", "judge this existing Playwright JSON report instead of running Playwright")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *report == "" {
		if os.Getenv("TEST_SERVER_BINARY") == "" {
			fmt.Fprintln(errw, "e2erun: TEST_SERVER_BINARY is unset\nRemedy: rebuild the e2e binary (bazel build //:stapler-squad) and run through --config=e2e")
			return 1
		}
		*report = filepath.Join(firstNonEmpty(os.Getenv("TEST_TMPDIR"), os.TempDir()), *target+".json")
		cmd := exec.Command("bunx", append([]string{"playwright", "test", "--reporter=json"}, []string(specs)...)...)
		cmd.Dir = filepath.Join(*root, "tests/e2e")
		cmd.Env = append(dropEnv(os.Environ(), "npm_config_user_agent"), "PLAYWRIGHT_JSON_OUTPUT_NAME="+*report)
		cmd.Stdout, cmd.Stderr = io.Discard, errw
		_ = cmd.Run() // the verdict comes from the report, not the exit code
	}
	rb, err := os.ReadFile(*report)
	if err != nil {
		fmt.Fprintln(errw, "e2erun: no Playwright report (the run crashed before writing one):", err)
		return 1
	}
	var rep pwReport
	if err := json.Unmarshal(rb, &rep); err != nil || len(rep.Suites) == 0 {
		fmt.Fprintln(errw, "e2erun: empty or invalid Playwright report", err)
		return 1
	}
	bl, err := os.ReadFile(filepath.Join(*root, "tests/e2e/baseline/next-e2e-baseline.json"))
	if err != nil {
		fmt.Fprintln(errw, "e2erun:", err)
		return 1
	}
	ex, _ := os.ReadFile(filepath.Join(*root, "tests/e2e/baseline/bun-expected-diff.json"))
	tolerated, err := allowed(bl, ex)
	if err != nil {
		fmt.Fprintln(errw, "e2erun:", err)
		return 1
	}
	bad := verdict(rep, tolerated)
	for _, b := range bad {
		fmt.Fprintln(errw, "new failure:", b)
	}
	if len(bad) > 0 {
		fmt.Fprintf(errw, "e2erun: %s: %d failure(s) outside the baseline\n", *target, len(bad))
		return 1
	}
	fmt.Fprintf(out, "e2erun: %s: no failure outside the baseline\n", *target)
	return 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func dropEnv(env []string, name string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, name+"=") {
			out = append(out, e)
		}
	}
	return out
}
