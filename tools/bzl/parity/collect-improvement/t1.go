package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
)

// Replayer answers the two questions the T1 replay needs; the exec implementation drives Bazel and
// tools/ci/affected-targets, fakes drive the tests.
type Replayer interface {
	TotalTests() (int, error)
	Select(nameStatus string) ([]string, error)
}

// ExecReplayer shells out to the Bazel wrapper and the built affected-targets binary.
type ExecReplayer struct{ Root, Bazel, Affected, TmpDir string }

// TotalTests counts test targets in //...
func (e ExecReplayer) TotalTests() (int, error) {
	cmd := exec.Command(e.Bazel, "query", "--output=label", `kind(".*_test", //...)`) //nolint:norawexec // dev-time collector, waited on
	cmd.Dir = e.Root
	out, err := cmd.Output()
	return len(strings.Fields(string(out))), err
}

// Select runs affected-targets over a name-status listing.
func (e ExecReplayer) Select(ns string) ([]string, error) {
	f, err := os.CreateTemp(e.TmpDir, "ns")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString(ns)
	_ = f.Close()
	var eb bytes.Buffer
	cmd := exec.Command(e.Affected, "-root", e.Root, "-name-status", f.Name(), "-bazel", e.Bazel) //nolint:norawexec // dev-time collector, waited on
	cmd.Stderr = &eb
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, eb.String())
	}
	return strings.Fields(string(out)), nil
}

// PRSample is the part of pr-sample.json the collectors read.
type PRSample struct {
	GlobalRegex string `json:"seed_global_regex"`
	Partial     []struct {
		Number int      `json:"number"`
		Paths  []string `json:"paths"`
	} `json:"partial"`
}

func loadSample(path string) (PRSample, error) {
	var s PRSample
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// nameStatus renders `git diff --name-status` lines: M for paths present in the tree, D otherwise.
func nameStatus(paths []string, exists func(string) bool) string {
	var b strings.Builder
	for _, p := range paths {
		st := "D"
		if exists(p) {
			st = "M"
		}
		b.WriteString(st + "\t" + p + "\n")
	}
	return b.String()
}

// selectedRatio is selected tests over all tests; a //... selection means the whole graph (ratio 1).
func selectedRatio(sel []string, total int) float64 {
	n := 0
	for _, t := range sel {
		switch {
		case t == "//...":
			return 1
		case t != "" && !strings.HasPrefix(t, "job:"):
			n++
		}
	}
	if total <= 0 {
		return -1
	}
	if r := float64(n) / float64(total); r < 1 {
		return r
	}
	return 1
}

// MeasureT1 replays the sampled partial-change PR diffs and records the median affected-test ratio.
// It is a replay over history, so it needs no quiet host.
func MeasureT1(d *collect.Doc, r Replayer, s PRSample, exists func(string) bool) error {
	src := fmt.Sprintf("affected-targets replay over %d sampled partial-change PRs (test targets only)", len(s.Partial))
	total, err := r.TotalTests()
	if err != nil {
		return err
	}
	var ratios []float64
	for _, pr := range s.Partial {
		sel, err := r.Select(nameStatus(pr.Paths, exists))
		if err != nil {
			return fmt.Errorf("PR %d: %w", pr.Number, err)
		}
		ratios = append(ratios, selectedRatio(sel, total))
	}
	if len(ratios) == 0 || total <= 0 {
		d.Unmeasured("T1", "no sampled PRs or no test targets", collect.Meta{Source: src})
		return nil
	}
	d.Set("T1", collect.Median(ratios), collect.Meta{Source: src})
	d.Set("T1_total_test_targets", float64(total), collect.Meta{})
	d.Set("T1_n", float64(len(ratios)), collect.Meta{})
	return nil
}
