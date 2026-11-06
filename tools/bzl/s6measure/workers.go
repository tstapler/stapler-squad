package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WorkersOpts configures Task 0.10h.
type WorkersOpts struct {
	Target     string // //... per the plan
	WatchdogGB float64
	Sample     bool
}

// Workers inventories worker-capable mnemonics and, if any, compares multiplex on versus --noworker_multiplex
// over three concurrent builds. Non-gating: the verdict is Pass (adopt) or NoAction.
func Workers(h *Harness, o WorkersOpts) *Report {
	rep := NewReport()
	ctx := context.Background()
	wt, err := h.AddWT("w-inv", 0)
	if err != nil {
		rep.Errorf("worktree: %v", err)
		return rep
	}
	out, err := h.Bazel(ctx, wt, "aquery", o.Target, "--output=jsonproto")
	h.DropWT(wt)
	if err != nil || out.Code != 0 {
		rep.Errorf("aquery failed")
		return rep
	}
	workers, multiplex, err := WorkerMnemonics([]byte(out.Stdout))
	if err != nil {
		rep.Errorf("aquery output: %v", err)
		return rep
	}
	rep.Keys["mnemonics"], rep.Keys["multiplex_mnemonics"] = workers, multiplex
	if len(workers) == 0 {
		rep.Keys["outcome"], rep.Keys["verdict"] = "none-found", "NoAction"
		return rep
	}
	on, ok1 := workerRun(h, rep, "w-on", nil, o)
	off, ok2 := workerRun(h, rep, "w-off", []string{"--noworker_multiplex"}, o)
	if !ok1 || !ok2 {
		return rep
	}
	red, reg, v := MultiplexVerdict(off, on)
	rep.Keys["outcome"], rep.Keys["rss_reduction_pct"], rep.Keys["wall_regression_pct"], rep.Keys["verdict"] = "measured", red, reg, v
	return rep
}

// MultiplexVerdict: "PASS iff there is a peak-RSS reduction of at least 10% with no wall-time regression above
// 10%, in which case the setting is adopted; otherwise --noworker_multiplex or the default stays."
func MultiplexVerdict(off, on workerMeasure) (rssReductionPct, wallRegressionPct float64, verdict string) {
	rssReductionPct, wallRegressionPct = -PctChange(off.rssKB, on.rssKB), PctChange(off.wall, on.wall)
	verdict = "NoAction"
	if rssReductionPct >= 10 && wallRegressionPct <= 10 {
		verdict = "Pass"
	}
	return
}

type workerMeasure struct{ rssKB, wall float64 }

func workerRun(h *Harness, rep *Report, label string, extra []string, o WorkersOpts) (workerMeasure, bool) {
	var wts []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	for i := range 3 {
		w, err := h.AddWT(fmt.Sprintf("%s%d", label, i), i)
		if err != nil {
			rep.Errorf("%s: %v", label, err)
			return workerMeasure{}, false
		}
		wts = append(wts, w)
	}
	var sm *Sampler
	if o.Sample {
		sm = &Sampler{WatchdogKB: int64(o.WatchdogGB * 1024 * 1024)}
	}
	cache := filepath.Join(h.Dir, label+"-cache")
	_ = os.RemoveAll(cache)
	var rs []concResult
	var wall float64
	err := h.Gated(label, func() error {
		t0 := h.H.Now()
		rs = h.concurrentBuilds(wts, label, true, func(int) []string {
			return append(append([]string{}, extra...), "--disk_cache="+cache, o.Target)
		}, sm)
		wall = h.H.Now().Sub(t0).Seconds()
		return nil
	})
	if err != nil {
		rep.Errorf("%s: %v", label, err)
		return workerMeasure{}, false
	}
	for _, r := range rs {
		if r.err != nil {
			rep.Errorf("%s: %v", label, r.err)
			return workerMeasure{}, false
		}
	}
	m := workerMeasure{wall: wall}
	if sm != nil {
		m.rssKB = float64(sm.Stop())
	}
	return m, true
}
