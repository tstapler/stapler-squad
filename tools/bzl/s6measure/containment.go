package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ContainmentOpts configures Task 0.10g (E7, E11).
type ContainmentOpts struct {
	Target      string
	LiveUnit    string // the live service unit; only ever read (systemctl --user show), never touched
	ScopeLimit  string // MemoryMax of the E7 probe scope
	AllocMB     int    // bounded allocation of the probe, larger than the limit
	TinyTarget  string
	SkipCold    bool
	ScopeSuffix string
}

// ProfileCPUSeconds integrates the "CPU usage (Bazel)" counter (cores, one sample per second) of a Bazel JSON
// profile, plain or gzipped. -1 when the profile has no such counter.
func ProfileCPUSeconds(raw []byte) float64 {
	if zr, err := gzip.NewReader(bytes.NewReader(raw)); err == nil {
		if b, err := io.ReadAll(zr); err == nil {
			raw = b
		}
	}
	var doc struct {
		TraceEvents []struct {
			Ph   string             `json:"ph"`
			Name string             `json:"name"`
			Ts   float64            `json:"ts"`
			Args map[string]float64 `json:"args"`
		} `json:"traceEvents"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return -1
	}
	var sum, prev float64
	n := 0
	for _, e := range doc.TraceEvents {
		if e.Ph != "C" || e.Name != "CPU usage (Bazel)" {
			continue
		}
		if n > 0 {
			sum += e.Args["cpu"] * (e.Ts - prev) / 1e6
		}
		prev = e.Ts
		n++
	}
	if n < 2 {
		return -1
	}
	return sum
}

func (h *Harness) unitState(unit string) string {
	o, err := h.X.Run(context.Background(), Cmd{Name: "systemctl", Args: []string{"--user", "show", unit, "-p", "MainPID", "-p", "ActiveEnterTimestampMonotonic", "-p", "NRestarts"}})
	if err != nil {
		return "unreadable: " + err.Error()
	}
	return o.Stdout
}

// Containment runs E7 (limit alternatives) and E11 (cost of not queuing).
func Containment(h *Harness, o ContainmentOpts) *Report {
	rep := NewReport()
	containE7(h, rep, o)
	if !o.SkipCold {
		containE11(h, rep, o)
	}
	return rep
}

func containE7(h *Harness, rep *Report, o ContainmentOpts) {
	ctx := context.Background()
	before := h.unitState(o.LiveUnit)
	unit := "s6-e7-probe" + o.ScopeSuffix
	script := fmt.Sprintf("systemctl --user show %s.scope -p MemoryMax --value; head -c %dM /dev/zero | tail -n 1 >/dev/null", unit, o.AllocMB)
	out, err := h.X.Run(ctx, Cmd{Name: "systemd-run", Args: []string{"--user", "--scope", "--quiet", "--unit=" + unit, "-p", "MemoryMax=" + o.ScopeLimit, "bash", "-c", script}})
	if err != nil {
		rep.Errorf("e7 systemd-run: %v", err)
		return
	}
	first := strings.TrimSpace(strings.SplitN(out.Stdout, "\n", 2)[0])
	rep.Keys["e7_limit_reported"] = first != "" && first != "infinity" && !strings.HasPrefix(first, "[")
	rep.Keys["e7_limit_value"] = first
	rep.Keys["e7_probe_exit"] = out.Code
	rep.Keys["e7_live_unit_untouched"] = h.unitState(o.LiveUnit) == before
	rep.Check("e7", rep.Keys["e7_limit_reported"] == true && out.Code != 0 && rep.Keys["e7_live_unit_untouched"] == true,
		"a deliberately memory-hungry action is limited or killed without the live service's unit being touched and systemctl --user show reports the limit")
	containE7Server(h, rep, o)
}

// containE7Server records whether the committed .bazelrc's --io_nice_level=7 / --batch_cpu_scheduling take effect
// on a server started with --experimental_run_in_user_cgroup. Inputs, not gates.
func containE7Server(h *Harness, rep *Report, o ContainmentOpts) {
	ctx := context.Background()
	wt, err := h.AddWT("e7-wt", 0)
	if err != nil {
		rep.Errorf("e7 worktree: %v", err)
		return
	}
	defer h.DropWT(wt)
	out, err := h.Raw(ctx, wt, []string{"--experimental_run_in_user_cgroup", "--io_nice_level=7", "--batch_cpu_scheduling"}, "info", "server_pid")
	pid := strings.TrimSpace(out.Stdout)
	if err != nil || out.Code != 0 || pid == "" {
		rep.Errorf("e7 server_pid unavailable")
		return
	}
	io, _ := h.X.Run(ctx, Cmd{Name: "ionice", Args: []string{"-p", pid}})
	ch, _ := h.X.Run(ctx, Cmd{Name: "chrt", Args: []string{"-p", pid}})
	cg, _ := os.ReadFile(filepath.Join("/proc", pid, "cgroup"))
	rep.Keys["e7_io_nice_applied"] = strings.Contains(io.Stdout, "best-effort") && strings.Contains(io.Stdout, "prio 7")
	rep.Keys["e7_batch_scheduling_applied"] = strings.Contains(ch.Stdout, "SCHED_BATCH")
	rep.Keys["e7_server_cgroup"] = strings.TrimSpace(string(cg))
}

func containE11(h *Harness, rep *Report, o ContainmentOpts) {
	conc, ok1 := coldCPU(h, rep, "e11-conc", true, o)
	prev := h.Slots
	h.Slots = "1"
	ser, ok2 := coldCPU(h, rep, "e11-serial", false, o)
	h.Slots = prev
	if !ok1 || !ok2 || conc <= 0 {
		return
	}
	rep.Keys["e11_concurrent_cpu_s"], rep.Keys["e11_serial_cpu_s"], rep.Keys["e11_serial_over_concurrent_cpu"] = conc, ser, ser/conc
	rep.Check("e11", ser/conc <= 0.60, "serialised total CPU-seconds (profile CPU usage) is at most 60% of concurrent")
}

// coldCPU starts 3 cold builds at once (direct, or queued through the wrapper) on an empty cache and sums profile CPU.
func coldCPU(h *Harness, rep *Report, label string, direct bool, o ContainmentOpts) (float64, bool) {
	var wts []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	for i := range 3 {
		w, err := h.AddWT(fmt.Sprintf("%s%d", label, i), i)
		if err != nil {
			rep.Errorf("%s worktree: %v", label, err)
			return 0, false
		}
		wts = append(wts, w)
	}
	cache := filepath.Join(h.Dir, label+"-cache")
	_ = os.RemoveAll(cache)
	var rs []concResult
	err := h.Gated(label, func() error {
		rs = h.concurrentBuilds(wts, label, direct, func(i int) []string {
			return []string{"--disk_cache=" + cache, "--profile=" + filepath.Join(h.Dir, fmt.Sprintf("%s-%d.json", label, i)), o.Target}
		}, nil)
		return nil
	})
	if err != nil {
		rep.Errorf("%s: %v", label, err)
		return 0, false
	}
	var total float64
	for i, r := range rs {
		if r.err != nil {
			rep.Errorf("%s build %d: %v", label, i, r.err)
			return 0, false
		}
		raw, _ := os.ReadFile(filepath.Join(h.Dir, fmt.Sprintf("%s-%d.json", label, i)))
		c := ProfileCPUSeconds(raw)
		if c < 0 {
			rep.Errorf("%s build %d: profile has no CPU usage counter", label, i)
			return 0, false
		}
		total += c
	}
	return total, true
}
