package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// CostOpts configures Task 0.10a (five-worktree cost and RSS).
type CostOpts struct {
	Target     string
	Baseline   string // baseline-worktree.json
	Worktrees  int    // 5
	WatchdogGB float64
	Sample     bool
}

// MakeBaseline returns the make-build wall-time median and disk_kb_worktree median of baseline-worktree.json.
func MakeBaseline(path string) (wallS, diskKB float64, err error) {
	var doc struct {
		Runs []struct {
			Wall float64 `json:"wall_s"`
			Disk float64 `json:"disk_kb_worktree"`
		} `json:"runs"`
	}
	if err = readJSON(path, &doc); err != nil {
		return 0, 0, err
	}
	var w, d []float64
	for _, r := range doc.Runs {
		w, d = append(w, r.Wall), append(d, r.Disk)
	}
	if len(w) == 0 {
		return 0, 0, fmt.Errorf("%s has no runs", path)
	}
	return Median(w), Median(d), nil
}

func (h *Harness) duDeref(dir string) int64 {
	o, err := h.X.Run(context.Background(), Cmd{Name: "du", Args: []string{"-skL", dir}})
	if err != nil {
		return -1
	}
	var n int64
	fmt.Sscan(o.Stdout, &n)
	return n
}

// Cost builds one target in n worktrees against a shared disk cache and measures time, RSS and disk.
func Cost(h *Harness, o CostOpts) *Report {
	rep := NewReport()
	makeWall, makeDisk, err := MakeBaseline(o.Baseline)
	if err != nil {
		rep.Errorf("baseline: %v", err)
		return rep
	}
	var sm *Sampler
	if o.Sample {
		sm = &Sampler{WatchdogKB: int64(o.WatchdogGB * 1024 * 1024)}
	}
	var wts, obs []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	var walls []float64
	for i := range o.Worktrees {
		w, err := h.AddWT(fmt.Sprintf("k%d", i), i%4)
		if err != nil {
			rep.Errorf("worktree: %v", err)
			return rep
		}
		wts = append(wts, w)
		ob, err := h.OutputBase(w)
		if err != nil {
			rep.Errorf("output_base: %v", err)
			return rep
		}
		obs = append(obs, ob)
		wall, _, ok := h.costBuild(rep, w, ob, fmt.Sprintf("cost-%d", i), o, sm)
		if !ok {
			return rep
		}
		walls = append(walls, wall)
	}
	if sm != nil {
		peak := float64(sm.Stop()) / 1024 / 1024
		rep.Keys["peak_rss_gb"], rep.Keys["watchdog_fired"] = peak, sm.Fired
	} else {
		rep.Errorf("peak_rss_gb: RSS sampling off")
	}
	h.costDisk(rep, obs, walls, makeWall, makeDisk)
	h.costInWorktree(rep, o)
	return rep
}

func (h *Harness) costBuild(rep *Report, wt, ob, label string, o CostOpts, sm *Sampler) (float64, []Spawn, bool) {
	if sm != nil {
		sm.Add(ob, func() {})
	}
	return h.timedBuild(rep, wt, label, h.DiskFlag(), o.Target)
}

func (h *Harness) costDisk(rep *Report, obs []string, walls []float64, makeWall, makeDisk float64) {
	var disk, ext, deref []float64
	for _, ob := range obs {
		disk = append(disk, float64(duKB(ob)))
		ext = append(ext, float64(duKB(filepath.Join(ob, "external"))))
		deref = append(deref, float64(h.duDeref(filepath.Join(ob, "external"))))
	}
	rep.Keys["cold_first_s"], rep.Keys["cold_second_median_s"] = walls[0], Median(walls[1:])
	rep.Keys["cold_ratio_median"] = Median(walls[1:]) / makeWall
	rep.Keys["output_base_disk_kb"] = Median(disk)
	rep.Keys["disk_ratio_vs_make"] = Median(disk) / makeDisk
	rep.Keys["external_new_kb_per_worktree"], rep.Keys["external_deref_kb_per_worktree"] = Median(ext), Median(deref)
	rep.Keys["cache_fs"], rep.Keys["output_base_fs"] = h.fsType(h.DiskCache), h.fsType(obs[0])
	peak, _ := rep.Keys["peak_rss_gb"].(float64)
	rep.Check("s6_cost", peak > 0 && peak <= 12 && Median(walls[1:])/makeWall <= 0.30 && rep.Keys["cache_fs"] == rep.Keys["output_base_fs"] && rep.Keys["cache_fs"] != "",
		"median second-worktree cold build at most 30% of the make median, peak combined RSS at most 12 GB, disk cache and output base on one filesystem")
	rep.Check("disk_bound_p10", Median(disk) <= 1.25*makeDisk, "per-worktree disk (output_base_disk_kb) not more than 1.25 x baseline disk_kb_worktree unless the owner waives")
}

// costInWorktree records the BAZEL_WORKTREE_OUTPUT_BASE=1 variant: its wall time, path length and whether
// removing the worktree removes the output base (the orphan-proofing claim). Inputs, not gates.
func (h *Harness) costInWorktree(rep *Report, o CostOpts) {
	w, err := h.AddWT("k-inwt", 1)
	if err != nil {
		rep.Errorf("in-worktree variant: %v", err)
		return
	}
	var wall float64
	err = h.Gated("cost-inwt", func() error {
		t0 := h.H.Now()
		out, e := h.X.Run(context.Background(), Cmd{Dir: w, Env: h.env("BAZEL_WORKTREE_OUTPUT_BASE=1"), Name: filepath.Join(w, "tools", "bazel"), Args: []string{"build", h.DiskFlag(), o.Target}})
		wall = h.H.Now().Sub(t0).Seconds()
		if e == nil && out.Code != 0 {
			e = fmt.Errorf("exit %d", out.Code)
		}
		return e
	})
	if err != nil {
		rep.Errorf("in-worktree variant: %v", err)
		h.DropWT(w)
		return
	}
	base := filepath.Join(w, ".bazel-out")
	rep.Keys["inwt_wall_s"], rep.Keys["inwt_output_base_kb"], rep.Keys["inwt_path_len"] = wall, duKB(base), len(base)
	ctx := context.Background()
	_, _ = h.X.Run(ctx, Cmd{Dir: w, Env: h.env("BAZEL_WORKTREE_OUTPUT_BASE=1"), Name: filepath.Join(w, "tools", "bazel"), Args: []string{"shutdown"}})
	_, _ = h.X.Run(ctx, Cmd{Name: "chmod", Args: []string{"-R", "u+w", base}}) // Bazel leaves outputs read-only
	h.DropWT(w)
	_, statErr := os.Stat(base)
	rep.Keys["inwt_removed_with_worktree"] = os.IsNotExist(statErr)
}
