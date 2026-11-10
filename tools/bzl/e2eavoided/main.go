// Command e2eavoided is the T8 collector (Task 12.4d): e2e runner-minutes per partial-change PR, Bazel-selective
// versus the full-suite baseline, plus e2e_escapes from the escape ledger. It prints `T8: PASS|FAIL|UNMEASURED`.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// Sample is one sampled partial-change PR. A nil SelectiveMin means no shadow run exists for it yet.
type Sample struct {
	PR              int      `json:"pr"`
	BaselineFullMin *float64 `json:"baseline_full_min,omitempty"`
	SelectiveMin    *float64 `json:"selective_min,omitempty"`
	BaselineRunID   int64    `json:"baseline_run_id,omitempty"`
	SelectiveRunID  int64    `json:"selective_run_id,omitempty"`
}

// Input is the fixture / collected data set.
type Input struct {
	Samples    []Sample `json:"prs"`
	Escapes    int      `json:"e2e_escapes"`
	SoakRuns   int      `json:"soak_runs"`
	EscapeRows string   `json:"-"`
}

// Verdict is the T8 evaluation.
type Verdict struct {
	Status          string
	BaselineMedian  float64
	SelectiveMedian float64
	Ratio           float64
	Escapes         int
	Measured        int
	Reason          string
}

func median(xs []float64) float64 {
	sort.Float64s(xs)
	n := len(xs)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return xs[n/2]
	}
	return (xs[n/2-1] + xs[n/2]) / 2
}

// evaluate: PASS needs selective median <= maxRatio x baseline median over the PRs that have both values, zero
// escapes, and at least one measured PR. No measured PR (or no soak run) is UNMEASURED, which the gate treats as FAIL.
func evaluate(in Input, maxRatio float64) Verdict {
	var base, sel []float64
	for _, s := range in.Samples {
		if s.BaselineFullMin != nil && s.SelectiveMin != nil {
			base, sel = append(base, *s.BaselineFullMin), append(sel, *s.SelectiveMin)
		}
	}
	v := Verdict{Escapes: in.Escapes, Measured: len(base)}
	if len(base) == 0 {
		v.Status, v.Reason = "UNMEASURED", "no sampled PR has both a baseline full run and a selective shadow run"
		return v
	}
	v.BaselineMedian, v.SelectiveMedian = median(base), median(sel)
	if v.BaselineMedian > 0 {
		v.Ratio = v.SelectiveMedian / v.BaselineMedian
	}
	switch {
	case in.Escapes > 0:
		v.Status, v.Reason = "FAIL", fmt.Sprintf("e2e_escapes = %d, must be 0", in.Escapes)
	case v.Ratio > maxRatio:
		v.Status, v.Reason = "FAIL", fmt.Sprintf("selective/baseline median %.2f > %.2f", v.Ratio, maxRatio)
	default:
		v.Status = "PASS"
	}
	return v
}

type job struct {
	Name        string `json:"name"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

// shardMinutes sums the durations of jobs whose name starts with prefix, from `gh api .../actions/runs/<id>/jobs`.
func shardMinutes(jobsJSON []byte, prefix string) (float64, error) {
	var doc struct {
		Jobs []job `json:"jobs"`
	}
	if err := json.Unmarshal(jobsJSON, &doc); err != nil {
		return 0, err
	}
	var total float64
	for _, j := range doc.Jobs {
		if !strings.HasPrefix(j.Name, prefix) || j.CompletedAt == "" {
			continue
		}
		d, err := duration(j.StartedAt, j.CompletedAt)
		if err != nil {
			return 0, err
		}
		total += d
	}
	return total / 60, nil
}

func countEscapes(ledger []byte) (int, error) {
	var rows []struct {
		Selected *bool `json:"selected"`
	}
	if err := json.Unmarshal(ledger, &rows); err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if r.Selected != nil && !*r.Selected {
			n++
		}
	}
	return n, nil
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("e2eavoided", flag.ContinueOnError)
	fs.SetOutput(errw)
	fixture := fs.String("fixture", "", "evaluate this JSON file instead of calling gh")
	samples := fs.String("samples", "", "JSON {prs:[{pr,baseline_run_id,selective_run_id}]} to collect through gh api")
	repo := fs.String("repo", "tstapler/stapler-squad", "owner/repo for gh api")
	ledger := fs.String("escapes", "tests/e2e/escapes.json", "escape ledger")
	maxRatio := fs.Float64("max-ratio", 0.50, "T8 target: selective median <= this x baseline median")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var in Input
	switch {
	case *fixture != "":
		b, err := os.ReadFile(*fixture)
		if err == nil {
			err = json.Unmarshal(b, &in)
		}
		if err != nil {
			fmt.Fprintln(errw, "e2eavoided:", err)
			return 1
		}
	case *samples != "":
		var err error
		if in, err = collect(*samples, *repo, *ledger); err != nil {
			fmt.Fprintln(errw, "e2eavoided:", err)
			return 1
		}
	default:
		fmt.Fprintln(errw, "usage: e2eavoided -fixture FILE | -samples FILE [-repo owner/repo]")
		return 2
	}
	v := evaluate(in, *maxRatio)
	fmt.Fprintf(out, "measured_prs=%d baseline_median_min=%.1f selective_median_min=%.1f ratio=%.2f e2e_escapes=%d %s\n",
		v.Measured, v.BaselineMedian, v.SelectiveMedian, v.Ratio, v.Escapes, v.Reason)
	fmt.Fprintf(out, "T8: %s\n", v.Status)
	if v.Status == "PASS" {
		return 0
	}
	return 1
}

func collect(samplesFile, repo, ledger string) (Input, error) {
	var in Input
	b, err := os.ReadFile(samplesFile)
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(b, &in); err != nil {
		return in, err
	}
	for i := range in.Samples {
		s := &in.Samples[i]
		if s.BaselineRunID != 0 {
			if m, err := ghShardMinutes(repo, s.BaselineRunID, "e2e (Bun build) shard"); err == nil {
				s.BaselineFullMin = &m
			}
		}
		if s.SelectiveRunID != 0 {
			if m, err := ghShardMinutes(repo, s.SelectiveRunID, "shard"); err == nil {
				s.SelectiveMin = &m
			}
		}
	}
	if lb, err := os.ReadFile(ledger); err == nil {
		if in.Escapes, err = countEscapes(lb); err != nil {
			return in, err
		}
	}
	return in, nil
}

func ghShardMinutes(repo string, run int64, prefix string) (float64, error) {
	b, err := exec.Command("gh", "api", "--paginate", fmt.Sprintf("repos/%s/actions/runs/%d/jobs", repo, run)).Output()
	if err != nil {
		return 0, err
	}
	return shardMinutes(b, prefix)
}
