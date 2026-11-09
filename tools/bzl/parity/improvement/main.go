// Command improvement evaluates the T1-T8 improvement rows of the parity gate (Task 7.5g).
//
// Targets come from tools/bzl/baseline/targets.yaml; measurements come from a JSON object keyed by target row
// id (e.g. {"T1": 0.25, "T2-partial-ttg": 2100}; non-numeric keys such as "_meta" are ignored), produced by the collectors (affected-targets replay,
// bazel-trend, worktree-cost, ...). A row with no measurement or no target value is UNMEASURED, never PASS.
// Exit 1 when any row FAILs or a gating row (T1-T5, T7, T8) is UNMEASURED.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"gopkg.in/yaml.v3"
)

type target struct {
	ID    string   `yaml:"id"`
	Value *float64 `yaml:"target_value"`
}

type targetsFile struct {
	Targets []target `yaml:"targets"`
}

// minRows are "at least" rows; every other row is "at most".
var minRows = map[string]bool{"T2b": true, "T5": true}

// reportedOnly rows never fail the gate while UNMEASURED (T6 is reported until Story 10.1).
var reportedOnly = map[string]bool{"T6": true}

var rowGroup = regexp.MustCompile(`^(T[0-9]+)`)

const (
	pass       = "PASS"
	fail       = "FAIL"
	unmeasured = "UNMEASURED"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("improvement", flag.ContinueOnError)
	fs.SetOutput(stderr)
	targets := fs.String("targets", "tools/bzl/baseline/targets.yaml", "targets.yaml")
	meas := fs.String("measurements", "", "measurements JSON")
	fs.StringVar(meas, "fixture", "", "alias of --measurements")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rows, err := loadTargets(*targets)
	if err != nil {
		fmt.Fprintln(stderr, "improvement:", err)
		return 2
	}
	m := map[string]float64{}
	if *meas != "" {
		if m, err = loadMeasurements(*meas); err != nil {
			fmt.Fprintln(stderr, "improvement:", err)
			return 2
		}
	}
	verdicts := Evaluate(rows, m)
	code := 0
	for n := 1; n <= 8; n++ {
		id := fmt.Sprintf("T%d", n)
		fmt.Fprintf(stdout, "%s: %s\n", id, verdicts[id])
		if verdicts[id] == fail || (verdicts[id] == unmeasured && !reportedOnly[id]) {
			code = 1
		}
	}
	return code
}

func loadTargets(path string) ([]target, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f targetsFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return f.Targets, nil
}

func loadMeasurements(path string) (map[string]float64, error) { return collect.Numbers(path) }

// Evaluate returns PASS/FAIL/UNMEASURED per T<n>: FAIL beats UNMEASURED beats PASS across a group's rows.
func Evaluate(rows []target, m map[string]float64) map[string]string {
	out := map[string]string{}
	for n := 1; n <= 8; n++ {
		out[fmt.Sprintf("T%d", n)] = unmeasured
	}
	state := map[string][]string{}
	for _, r := range rows {
		g := rowGroup.FindString(r.ID)
		if g == "" {
			continue
		}
		state[g] = append(state[g], rowVerdict(r, m))
	}
	for g, vs := range state {
		if _, ok := out[g]; ok {
			out[g] = combine(vs)
		}
	}
	return out
}

func rowVerdict(r target, m map[string]float64) string {
	v, ok := m[r.ID]
	if !ok || r.Value == nil {
		return unmeasured
	}
	if minRows[r.ID] {
		if v >= *r.Value {
			return pass
		}
		return fail
	}
	if v <= *r.Value {
		return pass
	}
	return fail
}

func combine(vs []string) string {
	res := pass
	for _, v := range vs {
		switch v {
		case fail:
			return fail
		case unmeasured:
			res = unmeasured
		}
	}
	return res
}
