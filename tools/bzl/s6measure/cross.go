package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// controlBUILD is written into each scratch worktree (never committed): the E4 positive control, whose output
// embeds the execroot path, and the stamp=1 genrule that must re-execute in a second worktree.
const controlBUILD = `genrule(
    name = "pwd_control",
    outs = ["pwd_control.txt"],
    cmd = "pwd > $@",
)

genrule(
    name = "stamped_control",
    outs = ["stamped_control.txt"],
    cmd = "cat bazel-out/volatile-status.txt > $@",
    stamp = 1,
)
`

const (
	controlPkg   = "tools/bzl/spike/s6_control"
	pwdControl   = controlPkg + "/pwd_control.txt"
	stampControl = controlPkg + "/stamped_control.txt"
)

// CrossOpts configures the E1 to E4 run (Task 0.10e).
type CrossOpts struct {
	Target    string
	GoTargets []string // three Go library labels of different sizes (E3)
	E4Targets []string // "build everything" per the plan; narrowed only by the operator
	Allow     []string // pre-declared allow-list substrings (tmux, Bun consumers)
	Which     map[string]bool
}

func (o CrossOpts) on(k string) bool { return len(o.Which) == 0 || o.Which[k] }

func writeControl(wt string) error {
	d := filepath.Join(wt, controlPkg)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "BUILD.bazel"), []byte(controlBUILD), 0o644)
}

func (h *Harness) spawnsOf(path string) []Spawn {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return ParseSpawns(f)
}

// timedBuild is a gated BuildLogged that returns the wall seconds.
func (h *Harness) timedBuild(rep *Report, wt, label string, extra ...string) (wall float64, spawns []Spawn, ok bool) {
	var logPath string
	err := h.Gated(label, func() error {
		t0 := h.H.Now()
		o, lp, err := h.BuildLogged(context.Background(), wt, label, extra...)
		wall, logPath = h.H.Now().Sub(t0).Seconds(), lp
		if err == nil && o.Code != 0 {
			err = fmt.Errorf("bazel build exit %d", o.Code)
		}
		return err
	})
	if err != nil {
		rep.Errorf("%s: %v", label, err)
		return 0, nil, false
	}
	return wall, h.spawnsOf(logPath), true
}

// Cross runs E1 to E4. A check whose inputs could not be produced is left unrecorded, so it cannot read as Pass.
func Cross(h *Harness, o CrossOpts) *Report {
	rep := NewReport()
	allow := append(append([]string(nil), o.Allow...), "stamped_control")
	var wts []string
	defer func() {
		for _, w := range wts {
			h.DropWT(w)
		}
	}()
	mk := func(name string, depth int) string {
		w, err := h.AddWT(name, depth)
		if err != nil {
			rep.Errorf("worktree %s: %v", name, err)
			return ""
		}
		wts = append(wts, w)
		return w
	}
	if o.on("e1") || o.on("e2") || o.on("e3") {
		crossShared(h, rep, o, allow, mk)
	}
	if o.on("e4") {
		crossDigests(h, rep, o, mk)
	}
	return rep
}

func crossShared(h *Harness, rep *Report, o CrossOpts, allow []string, mk func(string, int) string) {
	a, b, c := mk("e-a", 0), mk("e-b", 3), mk("e-c", 1)
	if a == "" || b == "" || c == "" {
		return
	}
	if err := os.MkdirAll(h.DiskCache, 0o755); err != nil {
		rep.Errorf("disk cache: %v", err)
		return
	}
	if ents, _ := os.ReadDir(h.DiskCache); len(ents) > 0 {
		rep.Errorf("shared disk cache %s must start empty", h.DiskCache)
		return
	}
	wallA, _, okA := h.timedBuild(rep, a, "e1-a", h.DiskFlag(), o.Target)
	if !okA {
		return
	}
	if _, spB, ok := h.timedBuild(rep, b, "e1-b", h.DiskFlag(), o.Target); ok {
		st := AnalyzeSpawns(spB, allow)
		rep.Keys["e1_cacheable_spawns"], rep.Keys["e1_hit_ratio"], rep.Keys["e1_nonallow_misses"] = st.Cacheable, st.HitRatio, len(st.NonAllowMisses)
		rep.Keys["e1_first_misses"] = firstMisses(st.NonAllowMisses, 5)
		if st.HitRatio >= 0 {
			rep.Check("e1", st.HitRatio >= 0.95 && len(st.NonAllowMisses) == 0,
				"at least 95% of cacheable spawns in B are hits and every other spawn is on the allow-list")
		} else {
			rep.Errorf("e1: no cacheable spawns in the execution log")
		}
	}
	if wallC, spC, ok := h.timedBuild(rep, c, "e2-c", h.DiskFlag(), o.Target); ok {
		st := AnalyzeSpawns(spC, allow)
		rep.Keys["e2_nonallow_spawns"], rep.Keys["e2_wall_ratio"] = st.NonAllowExecuted, wallC/wallA
		rep.Check("e2", st.NonAllowExecuted == 0 && wallC <= 0.30*wallA,
			"zero non-allow-listed linux-sandbox/worker spawns in at most 30% of the first worktree's wall time")
	}
	if o.on("e3") {
		crossGoCompile(h, rep, o, a, b)
	}
}

func firstMisses(ms []Spawn, n int) []string {
	var out []string
	for _, m := range ms[:min(n, len(ms))] {
		out = append(out, m.Mnemonic+" "+m.TargetLabel)
	}
	return out
}

func crossGoCompile(h *Harness, rep *Report, o CrossOpts, a, b string) {
	ctx := context.Background()
	var fails []string
	for _, t := range o.GoTargets {
		q := fmt.Sprintf("mnemonic(\"GoCompile\", %s)", t)
		oa, e1 := h.Bazel(ctx, a, "aquery", q, "--output=text")
		ob, e2 := h.Bazel(ctx, b, "aquery", q, "--output=text")
		if e1 != nil || e2 != nil || oa.Code != 0 || ob.Code != 0 || strings.TrimSpace(oa.Stdout) == "" {
			rep.Errorf("e3: aquery for %s failed or empty", t)
			return
		}
		if same, why := CompareAquery(oa.Stdout, ob.Stdout, a, b); !same {
			fails = append(fails, t+": "+why)
		}
	}
	rep.Keys["e3_gocompile_identical"], rep.Keys["e3_failures"] = len(fails) == 0, fails
	rep.Check("e3", len(fails) == 0, "GoCompile aquery command lines of three Go targets byte-identical across two worktree paths with no absolute path")
}

func crossDigests(h *Harness, rep *Report, o CrossOpts, mk func(string, int) string) {
	d, e := mk("e4-d", 0), mk("e4-e", 3)
	if d == "" || e == "" {
		return
	}
	for _, w := range []string{d, e} {
		if err := writeControl(w); err != nil {
			rep.Errorf("e4 control: %v", err)
			return
		}
	}
	targets := append([]string{NoCache(), "--stamp"}, o.E4Targets...)
	targets = append(targets, "//"+controlPkg+":pwd_control")
	var sums [2]map[string]string
	for i, w := range []string{d, e} {
		if _, _, ok := h.timedBuild(rep, w, fmt.Sprintf("e4-%d", i), targets...); !ok {
			return
		}
		bin, err := h.info(context.Background(), w, "bazel-bin")
		if err != nil {
			rep.Errorf("e4 bazel-bin: %v", err)
			return
		}
		if sums[i], err = HashTree(bin); err != nil {
			rep.Errorf("e4 hash: %v", err)
			return
		}
	}
	diff := DiffDigests(sums[0], sums[1])
	var real []string
	flagged := false
	for _, p := range diff {
		switch p {
		case pwdControl:
			flagged = true
		case stampControl:
		default:
			real = append(real, p)
		}
	}
	rep.Keys["e4_digest_lists_equal"], rep.Keys["e4_control_flagged"], rep.Keys["e4_differing_files"] = len(real) == 0, flagged, real
	rep.Check("e4", len(real) == 0 && flagged, "sorted sha256 lists of bazel-bin identical across two cache-disabled worktree builds and the positive-control pwd genrule is flagged")
	crossStamp(h, rep, d, e)
}

// crossStamp asserts a stamp=1 genrule misses the cache in the second worktree (pitfalls R11).
func crossStamp(h *Harness, rep *Report, a, b string) {
	t := "//" + controlPkg + ":stamped_control"
	if _, _, ok := h.timedBuild(rep, a, "stamp-a", h.DiskFlag(), "--stamp", t); !ok {
		return
	}
	_, sp, ok := h.timedBuild(rep, b, "stamp-b", h.DiskFlag(), "--stamp", t)
	if !ok {
		return
	}
	re := false
	for _, s := range sp {
		if strings.Contains(s.TargetLabel, "stamped_control") && executed(s) {
			re = true
		}
	}
	rep.Keys["e4_stamp_reexecuted"] = re
	rep.Check("e4_stamp", re, "a stamp = 1 genrule re-executes in the second worktree")
}
