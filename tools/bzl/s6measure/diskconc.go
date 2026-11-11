package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DiskConcOpts configures Task 0.10c.
type DiskConcOpts struct {
	Target     string // //... per the plan
	Rounds     int    // concurrent wrapper rounds, 2 per the plan
	WatchdogGB float64
	GCMaxSize  string // --experimental_disk_cache_gc_max_size for the GC-under-reader run
	Sample     bool   // sample RSS with the host-protection watchdog (off in unit tests)
}

type concResult struct {
	out   Out
	spawn []Spawn
	err   error
}

// concurrentBuilds starts one build per worktree at the same time and waits for all of them.
func (h *Harness) concurrentBuilds(wts []string, label string, direct bool, extra func(i int) []string, sm *Sampler) []concResult {
	res := make([]concResult, len(wts))
	var wg sync.WaitGroup
	for i, w := range wts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if sm != nil {
				if ob, err := h.OutputBase(w); err == nil {
					sm.Add(ob, cancel)
				}
			}
			lbl := fmt.Sprintf("%s-%d", label, i)
			execLog := filepath.Join(h.Dir, lbl+".execlog.json")
			args := append([]string{"build", "--execution_log_json_file=" + execLog}, extra(i)...)
			run := h.Bazel
			if direct {
				run = h.BazeliskDirect
			}
			o, err := run(ctx, w, args...)
			if err == nil && o.Code != 0 {
				err = fmt.Errorf("exit %d", o.Code)
			}
			_ = os.WriteFile(filepath.Join(h.Dir, lbl+".log"), []byte(o.Stdout+o.Stderr), 0o644)
			res[i] = concResult{o, h.spawnsOf(execLog), err}
		}()
	}
	wg.Wait()
	return res
}

func same(a ...string) func(int) []string { return func(int) []string { return a } }

// DiskConc runs Task 0.10c: concurrent builds into one shared disk cache, then integrity checks.
func DiskConc(h *Harness, o DiskConcOpts) *Report {
	rep := NewReport()
	var wts []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	for i := range 4 {
		w, err := h.AddWT(fmt.Sprintf("dc%d", i), i)
		if err != nil {
			rep.Errorf("worktree %d: %v", i, err)
			return rep
		}
		wts = append(wts, w)
	}
	three, fourth := wts[:3], wts[3]
	logs := [][]Spawn{}
	var texts []string
	collect := func(rs []concResult) bool {
		ok := true
		for _, r := range rs {
			texts = append(texts, r.out.Stdout+r.out.Stderr)
			if r.err != nil {
				rep.Errorf("concurrent build: %v", r.err)
				ok = false
			}
		}
		return ok
	}
	var sm *Sampler
	if o.Sample {
		sm = &Sampler{WatchdogKB: int64(o.WatchdogGB * 1024 * 1024)}
	}
	disk := h.DiskFlag()
	step := func(label string, direct bool) ([]concResult, bool) {
		var rs []concResult
		err := h.Gated(label, func() error { rs = h.concurrentBuilds(three, label, direct, same(disk, o.Target), sm); return nil })
		return rs, err == nil && collect(rs)
	}
	cold, ok := step("dc-bypass-cold", true) // empty cache, wrapper bypassed: true concurrency, the CPU multiple
	if !ok {
		return rep
	}
	for _, r := range cold {
		logs = append(logs, r.spawn)
	}
	rep.Keys["concurrency_cpu_multiple"] = CPUMultiple(logs)
	for r := 1; r <= o.Rounds; r++ {
		if _, ok := step(fmt.Sprintf("dc-wrapper-%d", r), false); !ok {
			return rep
		}
	}
	if sm != nil {
		rep.Keys["peak_combined_rss_gb"], rep.Keys["watchdog_fired"] = float64(sm.Stop())/1024/1024, sm.Fired
	}
	cached, nocache, ok := diskconcReference(h, rep, fourth, disk, o)
	if !ok {
		return rep
	}
	rep.Keys["hash_lists_equal"] = len(DiffDigests(cached, nocache)) == 0
	for _, w := range wts {
		if ob, err := h.OutputBase(w); err == nil {
			if b, e := os.ReadFile(filepath.Join(ob, "command.log")); e == nil {
				texts = append(texts, string(b))
			}
		}
	}
	rep.Keys["corruption_lines"] = CountPattern(strings.Join(texts, "\n"), "corrupt", "digest mismatch")
	diskconcGC(h, rep, wts[0], fourth, nocache, o)
	rep.Check("disk_cache_integrity", rep.Keys["hash_lists_equal"] == true && rep.Keys["corruption_lines"] == 0,
		"the 4th worktree's bazel-bin sha256 list built from the shared cache equals a no-cache build and no command.log contains corrupt or digest mismatch")
	if v, ok := rep.Keys["gc_race_errors"]; ok {
		rep.Check("gc_race", v == 0 && rep.Keys["gc_hash_lists_equal"] == true, "GC deleting a blob under a reader gives zero ERROR: lines and outputs identical to a no-cache build")
	}
	return rep
}

// diskconcReference builds the 4th worktree from the cache, hashes bazel-bin, then repeats with the cache off.
func diskconcReference(h *Harness, rep *Report, w, disk string, o DiskConcOpts) (cached, nocache map[string]string, ok bool) {
	ctx := context.Background()
	hash := func(label string, flags ...string) map[string]string {
		var m map[string]string
		err := h.Gated(label, func() error {
			out, _, e := h.BuildLogged(ctx, w, label, append(flags, o.Target)...)
			if e == nil && out.Code != 0 {
				e = fmt.Errorf("exit %d", out.Code)
			}
			if e != nil {
				return e
			}
			bin, e := h.info(ctx, w, "bazel-bin")
			if e != nil {
				return e
			}
			m, e = HashTree(bin)
			return e
		})
		if err != nil {
			rep.Errorf("%s: %v", label, err)
		}
		return m
	}
	cached = hash("dc-fourth-cached", disk)
	if cached == nil {
		return nil, nil, false
	}
	if out, err := h.Bazel(ctx, w, "clean"); err != nil || out.Code != 0 {
		rep.Errorf("dc clean: %v", err)
		return nil, nil, false
	}
	nocache = hash("dc-fourth-nocache", NoCache())
	return cached, nocache, nocache != nil
}

// diskconcGC runs a size-capped GC build while another worktree reads the same cache.
func diskconcGC(h *Harness, rep *Report, gcWT, readWT string, ref map[string]string, o DiskConcOpts) {
	ctx := context.Background()
	for _, w := range []string{gcWT, readWT} {
		if out, err := h.Bazel(ctx, w, "clean"); err != nil || out.Code != 0 {
			rep.Errorf("gc clean: %v", err)
			return
		}
	}
	rep.Keys["gc_cache_kb_before"] = duKB(h.DiskCache)
	gcFlags := []string{h.DiskFlag(), "--experimental_disk_cache_gc_max_size=" + o.GCMaxSize, "--experimental_disk_cache_gc_idle_delay=1s", o.Target}
	both := h.concurrentBuilds([]string{gcWT, readWT}, "dc-gc", false, func(i int) []string {
		if i == 0 {
			return gcFlags
		}
		return []string{h.DiskFlag(), o.Target}
	}, nil)
	rs, rd := both[:1], both[1:]
	rep.Keys["gc_cache_kb_after"] = duKB(h.DiskCache)
	errs := CountErrorLines(rs[0].out.Stderr + rd[0].out.Stderr)
	rep.Keys["gc_race_errors"] = errs
	bin, err := h.info(ctx, readWT, "bazel-bin")
	if err != nil {
		rep.Errorf("gc bazel-bin: %v", err)
		return
	}
	m, _ := HashTree(bin)
	rep.Keys["gc_hash_lists_equal"] = len(DiffDigests(m, ref)) == 0
}
