// Command e2esemantic evaluates the Bazel semantics ADR-009 relies on (spike S14, Task 0.15d) on the throwaway
// targets in tools/bzl/spike/e2e_slice and prints one `S-x: PASS|FAIL` line per check.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const pkg = "//tools/bzl/spike/e2e_slice"

type bazel struct {
	root  string
	flags []string
}

func (b bazel) run(args ...string) (string, error) {
	//nolint:norawexec short-lived run-to-completion spike driver
	c := exec.Command("nice", append([]string{"-n", "19", "tools/bazel"}, args...)...)
	c.Dir = b.root
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	return string(out), err
}

func (b bazel) test(target string, extra ...string) (string, error) {
	args := append(append([]string{"test", target, "--test_output=errors", "--test_summary=short"}, b.flags...), extra...)
	return b.run(args...)
}

var cachedRe = regexp.MustCompile(`\(cached\) (PASSED|FAILED)`)

func cached(out string) bool { return cachedRe.MatchString(out) }

func check(name string, ok bool, detail string) bool {
	state := "FAIL"
	if ok {
		state = "PASS"
	}
	fmt.Printf("%s: %s %s\n", name, state, detail)
	return ok
}

func edit(path string, add string) (restore func()) {
	orig, _ := os.ReadFile(path)
	_ = os.WriteFile(path, append(append([]byte{}, orig...), []byte(add)...), 0o644)
	return func() { _ = os.WriteFile(path, orig, 0o644) }
}

func semanticA(b bazel, dir string) bool {
	slice := pkg + ":slice_test"
	_, _ = b.test(slice)
	second, _ := b.test(slice)
	restore := edit(filepath.Join(dir, "inside.txt"), "edited\n")
	afterInside, _ := b.test(slice)
	restore()
	_, _ = b.test(slice)
	restore = edit(filepath.Join(dir, "outside.txt"), "edited\n")
	afterOutside, _ := b.test(slice)
	restore()
	ok := cached(second) && !cached(afterInside) && cached(afterOutside)
	return check("S-a", ok, fmt.Sprintf("rerun-cached=%v inside-edit-cached=%v (want false) outside-edit-cached=%v", cached(second), cached(afterInside), cached(afterOutside)))
}

func semanticB(b bazel, scratch string) bool {
	bin := filepath.Join(scratch, "const-server")
	_ = os.WriteFile(bin, []byte("binary v1\n"), 0o755)
	slice := pkg + ":slice_test"
	env := "--test_env=E2E_BIN=" + bin
	_, _ = b.test(slice, env)
	_ = os.WriteFile(bin, []byte("binary v2, different bytes\n"), 0o755)
	out, _ := b.test(slice, env)
	return check("S-b", cached(out), fmt.Sprintf("after swapping the file behind the constant path, cached=%v (want true: key unchanged)", cached(out)))
}

func semanticC(b bazel, scratch string) bool {
	fail := pkg + ":failing_test"
	dc := "--disk_cache=" + filepath.Join(scratch, "disk-cache")
	first, e1 := b.test(fail, dc)
	second, e2 := b.test(fail, dc)
	ok := e1 != nil && e2 != nil && !cached(first) && !cached(second) && strings.Contains(second, "FAILED")
	return check("S-c", ok, fmt.Sprintf("second run cached=%v failed-again=%v", cached(second), e2 != nil))
}

func semanticD(b bazel) bool {
	out, err := b.run("query", fmt.Sprintf("rdeps(%s:slice_test, %s:lib.go)", pkg, pkg))
	return check("S-d", err == nil && strings.Contains(out, pkg+":slice_test"), "rdeps over a go_library source label finds the test")
}

type progress struct {
	Specs map[string]struct {
		State    string
		Attempts []struct {
			WallSeconds float64 `json:"wall_seconds"`
		}
	} `json:"specs"`
}

func play(root, bin, scratch string, specs []string) (time.Duration, error) {
	dir, _ := os.MkdirTemp(scratch, "sem-srv-")
	defer os.RemoveAll(dir)
	args := []string{"-n", "19", "ionice", "-c3", "bunx", "playwright", "test"}
	for _, s := range specs {
		args = append(args, filepath.Base(s))
	}
	args = append(args, "--workers=1", "--project=chromium", "--reporter=line")
	//nolint:norawexec short-lived run-to-completion spike driver
	c := exec.Command("nice", args...)
	c.Dir = filepath.Join(root, "tests/e2e")
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "npm_config_user_agent=") {
			env = append(env, e)
		}
	}
	abs, _ := filepath.Abs(bin)
	c.Env = append(env, "TEST_SERVER_BINARY="+abs, "TEST_SERVER_DIR="+dir, "E2E_WORKERS=1", "TMPDIR="+scratch)
	start := time.Now()
	err := c.Run()
	return time.Since(start), err
}

// semanticE: start-up overhead of one Playwright invocation (global-setup, server boot) as a share of a 5-spec
// group's wall time: (sum of 5 separate invocations - one group invocation) / 4 against 15% of the group.
func semanticE(root, bin, scratch, progressPath string) bool {
	b, err := os.ReadFile(progressPath)
	if err != nil {
		return check("S-e", false, "no progress.json: "+err.Error())
	}
	var p progress
	if err := json.Unmarshal(b, &p); err != nil {
		return check("S-e", false, err.Error())
	}
	type sp struct {
		name string
		wall float64
	}
	var all []sp
	for n, r := range p.Specs {
		if r.State == "done" && len(r.Attempts) == 1 {
			all = append(all, sp{n, r.Attempts[0].WallSeconds})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].wall < all[j].wall })
	if len(all) < 20 {
		return check("S-e", false, "fewer than 20 captured passing specs")
	}
	var group []string
	for _, s := range all[5:10] { // fast but not the very fastest: representative short specs
		group = append(group, s.name)
	}
	gw, err := play(root, bin, scratch, group)
	if err != nil {
		return check("S-e", false, "group run failed: "+err.Error())
	}
	var sep time.Duration
	for _, s := range group {
		d, err := play(root, bin, scratch, []string{s})
		if err != nil {
			return check("S-e", false, "single run failed: "+err.Error())
		}
		sep += d
	}
	overhead := (sep - gw).Seconds() / float64(len(group)-1)
	ok := overhead <= 0.15*gw.Seconds()
	return check("S-e", ok, fmt.Sprintf("group=%.1fs separate-sum=%.1fs per-target-startup=%.1fs limit=%.1fs specs=%v", gw.Seconds(), sep.Seconds(), overhead, 0.15*gw.Seconds(), group))
}

func main() {
	root := flag.String("root", ".", "repository root")
	scratch := flag.String("scratch", "", "scratch dir (never /tmp)")
	prog := flag.String("progress", "", "progress.json from e2ebatch (S-e)")
	bin := flag.String("binary", "", "non-instrumented server binary (S-e)")
	only := flag.String("only", "a,b,c,d,e", "checks to run")
	jobs := flag.String("bazel-flags", "--jobs=4 --local_resources=memory=3000", "flags passed to every bazel invocation")
	flag.Parse()
	if *scratch == "" || strings.HasPrefix(*scratch, "/tmp") || strings.HasPrefix(*scratch, "/dev/shm") {
		fmt.Fprintln(os.Stderr, "e2esemantic: -scratch must be a disk path, not /tmp")
		os.Exit(2)
	}
	b := bazel{root: *root, flags: strings.Fields(*jobs)}
	dir := filepath.Join(*root, "tools/bzl/spike/e2e_slice")
	pass := true
	for _, c := range strings.Split(*only, ",") {
		switch c {
		case "a":
			pass = semanticA(b, dir) && pass
		case "b":
			pass = semanticB(b, *scratch) && pass
		case "c":
			pass = semanticC(b, *scratch) && pass
		case "d":
			pass = semanticD(b) && pass
		case "e":
			pass = semanticE(*root, *bin, *scratch, *prog) && pass
		}
	}
	if !pass {
		os.Exit(1)
	}
}
