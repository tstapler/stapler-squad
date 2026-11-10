package collect

import (
	"encoding/json"
	"os"
)

// WorktreeBaseline is the make-path worktree cost (tools/bzl/baseline/baseline-worktree.json).
type WorktreeBaseline struct {
	WallS struct {
		Median float64 `json:"median"`
	} `json:"wall_s"`
	DiskKB struct {
		Median float64 `json:"median"`
	} `json:"disk_kb_worktree"`
}

// ClassBaseline is one PR class of measured.json.
type ClassBaseline struct {
	N            int     `json:"n"`
	RunnerMinP50 float64 `json:"runner_min_p50"`
	TTGP50S      float64 `json:"ttg_p50_s"`
}

// WorkflowBaseline is one per-workflow p50 of measured.json.
type WorkflowBaseline struct {
	Workflow string  `json:"workflow"`
	P50S     float64 `json:"p50_s"`
}

// MeasuredBaseline is the subset of tools/bzl/baseline/measured.json the collectors join against.
type MeasuredBaseline struct {
	CIP50S          float64            `json:"ci_p50_s"`
	PerWorkflow     []WorkflowBaseline `json:"ci_per_workflow"`
	WarmOnePkgTestS float64            `json:"warm_one_pkg_test_s"`
	RegenMedian     float64            `json:"regen_steps_per_session_median"`
	Partial         ClassBaseline      `json:"partial_class"`
	All             ClassBaseline      `json:"all_class"`
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// LoadWorktreeBaseline reads baseline-worktree.json.
func LoadWorktreeBaseline(path string) (WorktreeBaseline, error) {
	var w WorktreeBaseline
	return w, loadJSON(path, &w)
}

// LoadMeasured reads measured.json.
func LoadMeasured(path string) (MeasuredBaseline, error) {
	var m MeasuredBaseline
	return m, loadJSON(path, &m)
}

// S6Build is one build of an s6measure result.
type S6Build struct {
	Label        string  `json:"label"`
	Exit         int     `json:"exit"`
	WallS        float64 `json:"wall_s"`
	LoadPre      float64 `json:"load_pre"`
	LoadPost     float64 `json:"load_post"`
	Quiet        bool    `json:"quiet_at_start"`
	OutputBaseKB int64   `json:"output_base_kb"`
}

// S6Result is the output of `s6measure cold`.
type S6Result struct {
	Builds []S6Build `json:"builds"`
	Second float64   `json:"second_median_s"`
}

// LoadS6 reads an s6measure cold result.
func LoadS6(path string) (S6Result, error) {
	var r S6Result
	return r, loadJSON(path, &r)
}

// Readings converts the builds to quiet readings (load recorded before and after each).
func (r S6Result) Readings(max float64) []Reading {
	out := make([]Reading, 0, len(r.Builds))
	for _, b := range r.Builds {
		out = append(out, Reading{Pre: b.LoadPre, Post: b.LoadPost, Quiet: b.Quiet && b.LoadPre <= max && b.LoadPost <= max})
	}
	return out
}

// First returns the first-worktree cold build and the second-worktree builds that succeeded.
func (r S6Result) First() (S6Build, bool) {
	if len(r.Builds) == 0 || r.Builds[0].Exit != 0 {
		return S6Build{}, false
	}
	return r.Builds[0], true
}

// SecondWalls lists wall times of the successful second-worktree builds.
func (r S6Result) SecondWalls() []float64 {
	var xs []float64
	for _, b := range r.Builds[min(1, len(r.Builds)):] {
		if b.Exit == 0 {
			xs = append(xs, b.WallS)
		}
	}
	return xs
}
