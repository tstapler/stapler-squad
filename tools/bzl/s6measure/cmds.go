package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// checkFlags are the flags every S6 check subcommand shares.
type checkFlags struct {
	repo, ref, dir, diskCache, out, update string
	allowNoisy                             bool
	quietWait                              time.Duration
	watchdogGB                             float64
}

func (c *checkFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&c.repo, "repo", ".", "any worktree of the repository (used for git worktree add)")
	fs.StringVar(&c.ref, "ref", "HEAD", "commit checked out in every scratch worktree")
	fs.StringVar(&c.dir, "dir", "", "scratch base under ~/.cache/tmp/<task>/ (required; never /tmp)")
	fs.StringVar(&c.diskCache, "disk-cache", "", "shared --disk_cache directory (default <dir>/disk-cache)")
	fs.StringVar(&c.out, "out", "", "write result JSON here instead of stdout")
	fs.StringVar(&c.update, "update", "", "merge the scalar result keys (prefixed s6_) into this spike-measurements.json")
	fs.BoolVar(&c.allowNoisy, "allow-noisy-host", false, "OWNER-AUTHORISED OVERRIDE: run under load; results are marked not_quiet (also BZL_ALLOW_NOISY_HOST=1)")
	fs.DurationVar(&c.quietWait, "quiet-wait", 40*time.Minute, "max wait for a quiet host (1-min load <= 2.0) before each timed step")
	fs.Float64Var(&c.watchdogGB, "watchdog-gb", 20, "kill the builds if combined RSS exceeds this (host protection, not a verdict bound)")
}

func (c *checkFlags) harness() (*Harness, error) {
	if c.dir == "" {
		return nil, fmt.Errorf("-dir is required")
	}
	if strings.HasPrefix(c.dir, "/tmp") {
		return nil, fmt.Errorf("-dir %s is under /tmp (a RAM tmpfs); use ~/.cache/tmp/<task>/", c.dir)
	}
	if c.diskCache == "" {
		c.diskCache = c.dir + "/disk-cache"
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return nil, err
	}
	return &Harness{X: osExec{}, H: osHost{}, Repo: c.repo, Ref: c.ref, Dir: c.dir, DiskCache: c.diskCache,
		AllowNoisy: AllowNoisy(c.allowNoisy, os.Getenv(noisyEnv)), QuietWait: c.quietWait,
		Warn: func(s string) { fmt.Fprintln(os.Stderr, "s6measure:", s) }}, nil
}

func (c *checkFlags) finish(h *Harness, rep *Report, source string) error {
	return rep.Finish(c.out, c.update, source, h.Readings, h.AllowNoisy)
}

func csv(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// subcommands maps a name to its runner; each parses its own flags.
var subcommands = map[string]func([]string) error{
	"cold": runCold, "concurrent": runConcurrent,
	"cost": runCost, "queued": runQueued, "diskconc": runDiskConc, "cross": runCross,
	"layout": runLayout, "containment": runContainment, "workers": runWorkers, "layering": runLayering,
}

func newSet(name string) (*flag.FlagSet, *checkFlags) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	var c checkFlags
	c.bind(fs)
	return fs, &c
}

func runCost(args []string) error {
	fs, c := newSet("cost")
	var o CostOpts
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "bazel target")
	fs.StringVar(&o.Baseline, "baseline", "tools/bzl/baseline/baseline-worktree.json", "make baseline JSON")
	fs.IntVar(&o.Worktrees, "worktrees", 5, "worktrees to build")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.Sample, o.WatchdogGB = true, c.watchdogGB
	return c.finish(h, Cost(h, o), "s6measure cost")
}

func runQueued(args []string) error {
	fs, c := newSet("queued")
	o := QueuedOpts{Sample: true}
	var sock, url string
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "bazel target")
	fs.IntVar(&o.Invocations, "invocations", 8, "queued invocations through the wrapper")
	fs.IntVar(&o.Worktrees, "worktrees", 4, "worktrees the invocations are spread over")
	fs.IntVar(&o.IdleSamples, "idle-samples", 200, "idle stream probes")
	fs.DurationVar(&o.Gap, "gap", 50*time.Millisecond, "gap between stream probes")
	fs.StringVar(&sock, "tmux-socket", "ssq-manual-1", "the manual instance's private tmux socket")
	fs.StringVar(&url, "manual-url", "http://127.0.0.1:62871/", "manual instance URL (health check only; never :8543)")
	_ = fs.Parse(args)
	if err := ValidateIsolation(sock, url); err != nil {
		return err
	}
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.WatchdogGB = c.watchdogGB
	p := &tmuxProber{X: h.X, H: h.H, Sock: sock}
	if err := p.Setup(); err != nil {
		return err
	}
	defer p.Teardown()
	o.Prober = p
	o.Health = func() error {
		r, err := http.Get(url) //nolint:gosec,noctx // loopback health probe of the manual instance
		if err != nil {
			return err
		}
		return r.Body.Close()
	}
	return c.finish(h, Queued(h, o), "s6measure queued")
}

func runDiskConc(args []string) error {
	fs, c := newSet("diskconc")
	o := DiskConcOpts{Sample: true}
	fs.StringVar(&o.Target, "target", "//...", "bazel target (the plan builds //...)")
	fs.IntVar(&o.Rounds, "rounds", 2, "concurrent rounds through the wrapper")
	fs.StringVar(&o.GCMaxSize, "gc-max-size", "1G", "disk cache GC cap for the GC-under-reader run")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.WatchdogGB = c.watchdogGB
	return c.finish(h, DiskConc(h, o), "s6measure diskconc")
}

func runCross(args []string) error {
	fs, c := newSet("cross")
	var o CrossOpts
	var goT, e4T, allow, which string
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "E1/E2 target")
	fs.StringVar(&goT, "go-targets", "//config:config,//log:log,//session/git:git", "three Go library labels of different sizes (E3)")
	fs.StringVar(&e4T, "e4-targets", "//...", "E4 build set (the plan builds everything)")
	fs.StringVar(&allow, "allow-list", "tmux,bun", "pre-declared allow-list substrings (tmux, Bun consumers)")
	fs.StringVar(&which, "checks", "", "comma list of e1,e2,e3,e4 (default all)")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.GoTargets, o.E4Targets, o.Allow, o.Which = csv(goT), csv(e4T), csv(allow), map[string]bool{}
	for _, k := range csv(which) {
		o.Which[k] = true
	}
	return c.finish(h, Cross(h, o), "s6measure cross")
}

func runLayout(args []string) error {
	fs, c := newSet("layout")
	var o LayoutOpts
	var fetch string
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "E10 target")
	fs.StringVar(&o.TinyTarget, "tiny-target", "//tools/bzl/s6measure:s6measure", "E5's tiny build")
	fs.StringVar(&fetch, "e6-extra", "", "extra E6 build targets (go_deps / Bun fetches), comma list")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.FetchArgs = csv(fetch)
	return c.finish(h, Layout(h, o), "s6measure layout")
}

func runContainment(args []string) error {
	fs, c := newSet("containment")
	o := ContainmentOpts{}
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "E11 cold build")
	fs.StringVar(&o.LiveUnit, "live-unit", "stapler-squad.service", "live service unit (read-only checks)")
	fs.StringVar(&o.ScopeLimit, "scope-limit", "256M", "E7 probe scope MemoryMax")
	fs.IntVar(&o.AllocMB, "alloc-mb", 600, "E7 probe's bounded allocation (larger than the limit)")
	fs.BoolVar(&o.SkipCold, "skip-e11", false, "skip the 3-cold-server E11 run")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	return c.finish(h, Containment(h, o), "s6measure containment")
}

func runWorkers(args []string) error {
	fs, c := newSet("workers")
	o := WorkersOpts{Sample: true}
	fs.StringVar(&o.Target, "target", "//...", "inventory and comparison target")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.WatchdogGB = c.watchdogGB
	return c.finish(h, Workers(h, o), "s6measure workers")
}

func runLayering(args []string) error {
	fs, c := newSet("layering")
	var o LayeringOpts
	var extra string
	fs.StringVar(&o.Target, "target", "//:stapler-squad", "E8 target")
	fs.StringVar(&o.RemoteBin, "remote-bin", "bazel-remote", "bazel-remote binary (loopback only)")
	fs.StringVar(&o.Port, "port", "62876", "loopback HTTP port from the manual dev block")
	fs.StringVar(&extra, "remote-args", "", "comma list replacing the default (UNVERIFIED) bazel-remote argument template")
	fs.DurationVar(&o.KillAfter, "kill-after", 20*time.Second, "kill the remote this long into C's build")
	_ = fs.Parse(args)
	h, err := c.harness()
	if err != nil {
		return err
	}
	o.RemoteArgs = csv(extra)
	return c.finish(h, Layering(h, o), "s6measure layering")
}
