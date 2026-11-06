// s6measure runs the Story 0.10 / spike S6 timing and memory measurements through the tools/bazel wrapper.
//
//	s6measure cold       first-worktree cold build, then N second-worktree builds against the warmed disk cache
//	s6measure concurrent K worktrees building at the same time (peak combined RSS, wall time each)
//
// Every build is gated on a quiet host (1-minute load <= 2.0) and records the load before and after.
// -allow-noisy-host (or BZL_ALLOW_NOISY_HOST=1) is the owner-authorised override: the build starts at once
// under load and every result carries not_quiet:true plus load and free memory before and after.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: s6measure cold|concurrent [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "cold":
		err = runCold(os.Args[2:])
	case "concurrent":
		err = runConcurrent(os.Args[2:])
	default:
		err = fmt.Errorf("unknown subcommand %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "s6measure:", err)
		os.Exit(1)
	}
}

type common struct {
	repo, ref, dir, diskCache, target, out string
	watchdogGB                             float64
	allowNoisy                             bool
	update                                 string
	quietWait                              time.Duration
}

func (c *common) flags(fs *flag.FlagSet) {
	fs.StringVar(&c.repo, "repo", ".", "any worktree of the repository (used for git worktree add)")
	fs.StringVar(&c.ref, "ref", "HEAD", "commit to check out in every measured worktree")
	fs.StringVar(&c.dir, "dir", "", "base directory for worktrees (required)")
	fs.StringVar(&c.diskCache, "disk-cache", "", "shared --disk_cache for every build (required)")
	fs.StringVar(&c.target, "target", "//:stapler-squad", "bazel target")
	fs.StringVar(&c.out, "out", "", "write result JSON here")
	fs.Float64Var(&c.watchdogGB, "watchdog-gb", 20, "kill the builds if combined RSS exceeds this (host protection, not the 12 GB verdict bound)")
	fs.BoolVar(&c.allowNoisy, "allow-noisy-host", false, "OWNER-AUTHORISED OVERRIDE: measure under load instead of waiting for a quiet host (also BZL_ALLOW_NOISY_HOST=1); results are marked not_quiet")
	fs.StringVar(&c.update, "update", "", "cold only: merge s6_/mid_ first and second cold keys (and noisy_inputs) into this inputs JSON")
	fs.DurationVar(&c.quietWait, "quiet-wait", 40*time.Minute, "max wait for a quiet host before a timed build")
}

func (c *common) check() error {
	c.allowNoisy = AllowNoisy(c.allowNoisy, os.Getenv(noisyEnv))
	if c.dir == "" || c.diskCache == "" {
		return fmt.Errorf("-dir and -disk-cache are required")
	}
	return nil
}

func loadNow() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return -1
	}
	return ParseLoad1(string(b))
}

func memAvailGB() float64 {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return -1
	}
	return ParseMemAvailGB(string(b))
}

func waitQuiet(max time.Duration) (float64, bool) {
	end := time.Now().Add(max)
	for {
		l := loadNow()
		if l >= 0 && l <= 2.0 {
			return l, true
		}
		if time.Now().After(end) {
			return l, false
		}
		time.Sleep(5 * time.Second)
	}
}

func git(repo string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...) //nolint:norawexec,norawgitcli // dev-time measurement tool, waited on
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

func bazelCmd(ctx context.Context, wt string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Join(wt, "tools", "bazel"), args...) //nolint:norawexec // long build, waited on by the caller
	cmd.Dir = wt
	bz, _ := exec.LookPath("bazelisk")
	cmd.Env = append(os.Environ(), "BAZEL_REAL="+bz, "BAZEL_AUTO_DETACH_SECS=0", "BAZEL_WRAPPER_SLOTS=3")
	return cmd
}

func outputBase(wt string) (string, error) {
	out, err := bazelCmd(context.Background(), wt, "info", "output_base").Output()
	return strings.TrimSpace(string(out)), err
}

// Build is one timed build.
type Build struct {
	Label        string  `json:"label"`
	WT           string  `json:"worktree"`
	ExitCode     int     `json:"exit"`
	WallS        float64 `json:"wall_s"`
	LoadPre      float64 `json:"load_pre"`
	LoadPost     float64 `json:"load_post"`
	Quiet        bool    `json:"quiet_at_start"`
	NotQuiet     bool    `json:"not_quiet"`
	MemAvailPre  float64 `json:"mem_avail_gb_pre"`
	MemAvailPost float64 `json:"mem_avail_gb_post"`
	PeakRSSKB    int64   `json:"peak_rss_kb"`
	OutputBaseKB int64   `json:"output_base_kb"`
	Log          string  `json:"log"`
	Processes    string  `json:"processes_line"`
}

func duKB(path string) int64 {
	out, err := exec.Command("du", "-sk", path).Output() //nolint:norawexec // short-lived, waited on
	if err != nil {
		return -1
	}
	var n int64
	fmt.Sscan(string(out), &n)
	return n
}

func (c *common) build(label, wt, ob string, quiet bool, sampler *Sampler) Build {
	b := Build{Label: label, WT: wt, Quiet: true}
	if quiet {
		if c.allowNoisy {
			b.LoadPre = loadNow()
			b.Quiet = !IsNoisy(b.LoadPre, b.LoadPre)
			fmt.Fprintf(os.Stderr, "s6measure: NOISY HOST OVERRIDE: starting %s at 1-min load %.2f\n", label, b.LoadPre)
		} else {
			b.LoadPre, b.Quiet = waitQuiet(c.quietWait)
		}
	} else {
		b.LoadPre = loadNow()
	}
	b.MemAvailPre = memAvailGB()
	b.Log = filepath.Join(c.dir, label+".log")
	lf, _ := os.Create(b.Log)
	defer lf.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if sampler != nil {
		sampler.Add(ob, cancel)
	}
	cmd := bazelCmd(ctx, wt, "build", c.target, "--disk_cache="+c.diskCache)
	cmd.Stdout, cmd.Stderr = lf, lf
	t0 := time.Now()
	err := cmd.Run()
	b.WallS = time.Since(t0).Seconds()
	b.LoadPost = loadNow()
	b.MemAvailPost = memAvailGB()
	b.NotQuiet = IsNoisy(b.LoadPre, b.LoadPost)
	if err != nil {
		b.ExitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			b.ExitCode = ee.ExitCode()
		}
	}
	b.OutputBaseKB = duKB(ob)
	if data, e := os.ReadFile(b.Log); e == nil {
		for _, ln := range strings.Split(string(data), "\n") {
			if strings.Contains(ln, " processes: ") {
				b.Processes = strings.TrimSpace(ln)
			}
		}
	}
	return b
}

func (c *common) addWT(name string, depth int) (string, error) {
	p := filepath.Join(c.dir, name)
	for i := 0; i < depth; i++ {
		p = filepath.Join(p, fmt.Sprintf("d%d", i))
	}
	p = filepath.Join(p, "wt")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, git(c.repo, "worktree", "add", "--quiet", "--detach", p, c.ref)
}

func (c *common) dropWT(wt, ob string) {
	cmd := bazelCmd(context.Background(), wt, "clean", "--expunge")
	_ = cmd.Run()
	_ = bazelCmd(context.Background(), wt, "shutdown").Run()
	_ = os.RemoveAll(ob)
	_ = git(c.repo, "worktree", "remove", "--force", wt)
}

func writeOut(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if path == "" {
		fmt.Println(string(b))
		return nil
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func runCold(args []string) error {
	var c common
	fs := flag.NewFlagSet("cold", flag.ExitOnError)
	c.flags(fs)
	runs := fs.Int("runs", 5, "second-worktree runs")
	_ = fs.Parse(args)
	if err := c.check(); err != nil {
		return err
	}
	var all []Build
	a, err := c.addWT("a", 0)
	if err != nil {
		return err
	}
	aob, err := outputBase(a)
	if err != nil {
		return err
	}
	var sa Sampler
	fa := c.build("first-cold", a, aob, true, &sa)
	fa.PeakRSSKB = sa.Stop()
	all = append(all, fa)
	c.dropWT(a, aob)
	if fa.ExitCode != 0 {
		return writeOut(c.out, map[string]any{"builds": all, "error": "first build failed", "not_quiet": AnyNoisy(all)})
	}
	for k := 1; k <= *runs; k++ {
		wt, err := c.addWT(fmt.Sprintf("b%d", k), k%4+1)
		if err != nil {
			return err
		}
		ob, err := outputBase(wt)
		if err != nil {
			return err
		}
		var s Sampler
		b := c.build(fmt.Sprintf("second-%d", k), wt, ob, true, &s)
		b.PeakRSSKB = s.Stop()
		all = append(all, b)
		c.dropWT(wt, ob)
	}
	return c.finishCold(all)
}

func (c *common) finishCold(all []Build) error {
	noisy := AnyNoisy(all)
	second := medianWall(all[1:])
	if c.update != "" && all[0].ExitCode == 0 && second >= 0 {
		first := all[0].WallS
		kv := map[string]any{"s6_first_cold_s": first, "s6_second_cold_s": second, "mid_first_cold_s": first, "mid_second_cold_s": second}
		if err := UpdateInputs(c.update, kv, noisy, NoisyEntry(all, "s6measure cold")); err != nil {
			return err
		}
	}
	return writeOut(c.out, map[string]any{"builds": all, "second_median_s": second, "not_quiet": noisy, "allow_noisy_host": c.allowNoisy})
}

func medianWall(bs []Build) float64 {
	var xs []float64
	for _, b := range bs {
		if b.ExitCode == 0 {
			xs = append(xs, b.WallS)
		}
	}
	return Median(xs)
}

func runConcurrent(args []string) error {
	var c common
	fs := flag.NewFlagSet("concurrent", flag.ExitOnError)
	c.flags(fs)
	k := fs.Int("k", 3, "concurrent builds (max 3)")
	_ = fs.Parse(args)
	if err := c.check(); err != nil {
		return err
	}
	if *k > 3 {
		return fmt.Errorf("-k above 3 is not allowed")
	}
	type unit struct{ wt, ob string }
	var us []unit
	for i := 0; i < *k; i++ {
		wt, err := c.addWT(fmt.Sprintf("c%d", i), i)
		if err != nil {
			return err
		}
		ob, err := outputBase(wt)
		if err != nil {
			return err
		}
		us = append(us, unit{wt, ob})
	}
	if !c.allowNoisy {
		waitQuiet(c.quietWait)
	}
	var sampler Sampler
	sampler.WatchdogKB = int64(c.watchdogGB * 1024 * 1024)
	res := make([]Build, *k)
	var wg sync.WaitGroup
	for i, u := range us {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = c.build(fmt.Sprintf("concurrent-%d", i), u.wt, u.ob, false, &sampler)
		}()
	}
	wg.Wait()
	peak := sampler.Stop()
	for _, u := range us {
		c.dropWT(u.wt, u.ob)
	}
	return writeOut(c.out, map[string]any{"builds": res, "peak_combined_rss_kb": peak, "peak_combined_rss_gb": float64(peak) / 1024 / 1024, "watchdog_fired": sampler.Fired, "not_quiet": AnyNoisy(res), "allow_noisy_host": c.allowNoisy})
}
