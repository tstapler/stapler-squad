package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/collect"
	"github.com/tstapler/stapler-squad/tools/bzl/parity/ghci"
)

// Builder times one Bazel build; fakes make the collector testable without starting Bazel.
type Builder interface {
	Build(target string) (time.Duration, error)
}

// BazelBuilder runs the repository's tools/bazel wrapper (one invocation at a time, resource-capped).
type BazelBuilder struct {
	Dir    string
	Bazel  string // path of the real launcher exported as BAZEL_REAL
	Jobs   int
	MemMB  int
	Config []string
}

// Build implements Builder.
func (b BazelBuilder) Build(target string) (time.Duration, error) {
	args := append([]string{"build", target, fmt.Sprintf("--jobs=%d", b.Jobs), fmt.Sprintf("--local_resources=memory=%d", b.MemMB)}, b.Config...)
	cmd := exec.Command(filepath.Join(b.Dir, "tools", "bazel"), args...) //nolint:norawexec // timed build, waited on
	cmd.Dir = b.Dir
	cmd.Env = append(os.Environ(), "BAZEL_REAL="+b.Bazel)
	t0 := time.Now()
	out, err := cmd.CombinedOutput()
	d := time.Since(t0)
	if err != nil {
		return d, fmt.Errorf("bazel build %s: %w: %s", target, err, tailOf(string(out)))
	}
	return d, nil
}

func tailOf(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

// MeasureWarm records warm_noop_s: one untimed priming build, then runs timed no-op builds (median).
func MeasureWarm(d *collect.Doc, b Builder, g collect.Gate, target string, runs int) {
	src := fmt.Sprintf("median of %d no-op `bazel build %s` after a priming build", runs, target)
	if g.Require && !g.PreOK() {
		d.SetTimed("warm_noop_s", 0, []collect.Reading{g.End(g.Begin())}, g, src)
		return
	}
	if _, err := b.Build(target); err != nil {
		d.Unmeasured("warm_noop_s", "priming build failed: "+err.Error(), collect.Meta{Source: src})
		return
	}
	vals, rs, err := collect.Sample(g, runs, func() (float64, error) {
		dur, err := b.Build(target)
		return dur.Seconds(), err
	})
	switch {
	case err != nil && err != collect.ErrNotQuiet:
		d.Unmeasured("warm_noop_s", err.Error(), collect.Meta{Source: src})
	default:
		d.SetTimed("warm_noop_s", collect.Median(vals), rs, g, src)
	}
}

// DiskUsage returns the size of a directory in KB.
type DiskUsage func(path string) (int64, error)

// DuKB runs `du -sk`.
func DuKB(path string) (int64, error) {
	out, err := exec.Command("du", "-sk", path).Output() //nolint:norawexec // short-lived, waited on
	if err != nil {
		return 0, err
	}
	var n int64
	_, err = fmt.Sscan(string(out), &n)
	return n, err
}

// MeasureDisk records output_base_disk_kb and its ratio to the make baseline (disk needs no quiet host).
func MeasureDisk(d *collect.Doc, du DiskUsage, outputBase string, base collect.WorktreeBaseline) {
	src := "du -sk " + outputBase
	kb, err := du(outputBase)
	if err != nil {
		d.Unmeasured("output_base_disk_kb", err.Error(), collect.Meta{Source: src})
		d.Unmeasured("disk_ratio", "output_base_disk_kb unmeasured", collect.Meta{})
		return
	}
	d.Set("output_base_disk_kb", float64(kb), collect.Meta{Source: src})
	if base.DiskKB.Median <= 0 {
		d.Unmeasured("disk_ratio", "baseline disk_kb_worktree missing", collect.Meta{})
		return
	}
	d.Set("disk_ratio", float64(kb)/base.DiskKB.Median, collect.Meta{Source: "output_base_disk_kb / baseline-worktree.json disk_kb_worktree"})
}

// MeasureWorktree derives the second-worktree cold rows from an `s6measure cold` result (reused, not rerun).
func MeasureWorktree(d *collect.Doc, s6 collect.S6Result, base collect.WorktreeBaseline, g collect.Gate, src string) {
	walls := s6.SecondWalls()
	first, ok := s6.First()
	if len(walls) == 0 || !ok || base.WallS.Median <= 0 {
		for _, k := range []string{"second_cold_s", "second_cold_ratio_make", "second_cold_ratio_first"} {
			d.Unmeasured(k, "s6measure result lacks a successful first and second build, or the make baseline is missing", collect.Meta{Source: src})
		}
		return
	}
	rs := s6.Readings(g.Max)
	med := collect.Median(walls)
	d.SetTimed("second_cold_s", med, rs, g, src)
	d.SetTimed("second_cold_ratio_make", med/base.WallS.Median, rs, g, src+" / baseline-worktree.json wall_s.median")
	d.SetTimed("second_cold_ratio_first", med/first.WallS, rs, g, src+" / first-worktree cold build")
}

// MeasureTTG records PR time-to-green against measured.json (needs no quiet host; it reads history).
func MeasureTTG(d *collect.Doc, gh ghci.GH, repo string, workflows []string, n int, base collect.MeasuredBaseline) error {
	runs, err := ghci.FetchRuns(gh, repo, "pull_request", 40)
	if err != nil {
		return err
	}
	shas := ghci.TimeToGreen(runs, workflows, n)
	src := fmt.Sprintf("gh api actions/runs, workflows %s", strings.Join(workflows, ","))
	d.Set("ttg_n", float64(len(shas)), collect.Meta{Source: src})
	if len(shas) == 0 || base.CIP50S <= 0 {
		d.Unmeasured("ttg_p50_s", "no qualifying PR head SHA or no baseline", collect.Meta{Source: src})
		d.Unmeasured("ttg_ratio", "no qualifying PR head SHA or no baseline", collect.Meta{})
		d.Unmeasured("ttg_wf_ratio_max", "no qualifying PR head SHA or no baseline", collect.Meta{})
		return nil
	}
	var all []float64
	wf := map[string][]float64{}
	for _, s := range shas {
		all = append(all, s.TTGS)
		for k, v := range s.PerWF {
			wf[k] = append(wf[k], v)
		}
	}
	p50 := collect.Median(all)
	d.Set("ttg_p50_s", p50, collect.Meta{Source: src})
	d.Set("ttg_ratio", p50/base.CIP50S, collect.Meta{Source: "ttg_p50_s / measured.json ci_p50_s"})
	worst, matched := 0.0, 0
	for _, b := range base.PerWorkflow {
		if xs := wf[b.Workflow]; len(xs) > 0 && b.P50S > 0 {
			r := collect.Median(xs) / b.P50S
			d.Set("ttg_wf_p50_ratio:"+b.Workflow, r, collect.Meta{Source: "per-workflow p50 / measured.json ci_per_workflow"})
			if r > worst {
				worst = r
			}
			matched++
		}
	}
	if matched == 0 {
		d.Unmeasured("ttg_wf_ratio_max", "no collected workflow has a per-workflow baseline", collect.Meta{Source: src})
	} else {
		d.Set("ttg_wf_ratio_max", worst, collect.Meta{Source: "max per-workflow p50 ratio"})
	}
	return nil
}
