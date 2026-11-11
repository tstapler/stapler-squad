package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// LayoutOpts configures Task 0.10f (E5, E6, E10).
type LayoutOpts struct {
	Target     string // the build whose output E10 inspects
	TinyTarget string // E5's "one tiny build"
	FetchArgs  []string
}

// populateCache fills dir/cas with mb 1 MB blobs, backdated so size-based GC may evict them.
func populateCache(dir string, mb int) error {
	old := time.Now().Add(-2 * time.Hour)
	for i := range mb {
		var buf [8]byte
		for j := range buf {
			buf[j] = byte(rand.IntN(256))
		}
		sum := sha256.Sum256(append(buf[:], byte(i)))
		name := hex.EncodeToString(sum[:])
		d := filepath.Join(dir, "cas", name[:2])
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
		p := filepath.Join(d, name)
		if err := os.WriteFile(p, make([]byte, 1<<20), 0o644); err != nil {
			return err
		}
		if err := os.Chtimes(p, old, old); err != nil {
			return err
		}
	}
	return nil
}

func duMB(dir string) float64 { return float64(duKB(dir)) / 1024 }

// ExternalRepos classifies an output base's external/ directory. Plain repos have a canonical name with a single
// trailing '+'; module-extension repos contain "++" or more than one '+'; bazel_tools, local_config_* and
// bookkeeping entries are skipped.
func ExternalRepos(dir string) (plainTotal, plainLinked int, extNotLinked []string, err error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, nil, err
	}
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, "@") || strings.HasPrefix(n, "_") || strings.HasPrefix(n, "local_config_") || n == "bazel_tools" || strings.HasSuffix(n, ".marker") || !strings.Contains(n, "+") {
			continue
		}
		linked := false
		if t, e := os.Readlink(filepath.Join(dir, n)); e == nil && strings.Contains(t, "/contents/") {
			linked = true
		}
		if strings.Count(n, "+") == 1 && strings.HasSuffix(n, "+") {
			plainTotal++
			if linked {
				plainLinked++
			}
		} else if !linked {
			extNotLinked = append(extNotLinked, n)
		}
	}
	return plainTotal, plainLinked, extNotLinked, nil
}

// Layout runs E10, E5 and E6.
func Layout(h *Harness, o LayoutOpts) *Report {
	rep := NewReport()
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
	a, b := mk("l-a", 0), mk("l-b", 2)
	if a == "" || b == "" {
		return rep
	}
	layoutE10(h, rep, o, a, b)
	layoutE5(h, rep, o, a)
	layoutE6(h, rep, o, a, b)
	return rep
}

func (h *Harness) fsType(path string) string {
	out, err := h.X.Run(context.Background(), Cmd{Name: "stat", Args: []string{"-f", "-c", "%T", path}})
	if err != nil || out.Code != 0 {
		return ""
	}
	return strings.TrimSpace(out.Stdout)
}

func nlink(p string) uint64 {
	var st syscall.Stat_t
	if syscall.Stat(p, &st) != nil {
		return 0
	}
	return uint64(st.Nlink)
}

func layoutE10(h *Harness, rep *Report, o LayoutOpts, a, b string) {
	ctx := context.Background()
	if err := os.MkdirAll(h.DiskCache, 0o755); err != nil {
		rep.Errorf("e10: %v", err)
		return
	}
	for i, w := range []string{a, b} {
		if _, _, ok := h.timedBuild(rep, w, fmt.Sprintf("e10-%d", i), h.DiskFlag(), o.Target); !ok {
			return
		}
	}
	out, err := h.Bazel(ctx, b, "cquery", o.Target, "--output=files", h.DiskFlag())
	files := strings.Fields(out.Stdout)
	if err != nil || out.Code != 0 || len(files) == 0 {
		rep.Errorf("e10: cquery --output=files gave no output file")
		return
	}
	restored := filepath.Join(b, files[0])
	data, err := os.ReadFile(restored)
	if err != nil {
		rep.Errorf("e10: %v", err)
		return
	}
	sum := sha256.Sum256(data)
	hexs := hex.EncodeToString(sum[:])
	blob := filepath.Join(h.DiskCache, "cas", hexs[:2], hexs)
	rep.Keys["cache_fs"], rep.Keys["output_base_fs"] = h.fsType(h.DiskCache), ""
	if ob, e := h.OutputBase(b); e == nil {
		rep.Keys["output_base_fs"] = h.fsType(ob)
	}
	var shared [2]bool
	for i, p := range []string{restored, blob} {
		fo, e := h.X.Run(ctx, Cmd{Name: "filefrag", Args: []string{"-v", p}})
		if e != nil || fo.Code != 0 {
			rep.Errorf("e10: filefrag %s failed", p)
			return
		}
		shared[i] = ParseFilefragShared(fo.Stdout)
	}
	rep.Keys["e10_shared_extent"] = shared[0] && shared[1]
	rep.Keys["e10_nlink_one"] = nlink(restored) == 1 && nlink(blob) == 1
	rep.Keys["e10_recorded_only"] = "reflink is an input, not a gate: without it the disk bound is re-measured as copy-based"
}

func layoutE5(h *Harness, rep *Report, o LayoutOpts, wt string) {
	ctx := context.Background()
	gc := func(withDelay bool, wait time.Duration) (sizeAfterBuild, sizeAfterWait float64, ok bool) {
		dir := filepath.Join(h.Dir, "e5-cache")
		_ = os.RemoveAll(dir)
		if err := populateCache(dir, 300); err != nil {
			rep.Errorf("e5 populate: %v", err)
			return 0, 0, false
		}
		args := []string{"build", "--disk_cache=" + dir, "--experimental_disk_cache_gc_max_size=100M", o.TinyTarget}
		if withDelay {
			args = append(args[:3], append([]string{"--experimental_disk_cache_gc_idle_delay=10s"}, args[3:]...)...)
		}
		err := h.Gated("e5", func() error {
			out, e := h.Raw(ctx, wt, []string{"--max_idle_secs=30", "--host_jvm_args=-Xmx2g"}, args...)
			if e == nil && out.Code != 0 {
				e = fmt.Errorf("exit %d", out.Code)
			}
			return e
		})
		if err != nil {
			rep.Errorf("e5: %v", err)
			return 0, 0, false
		}
		sizeAfterBuild = duMB(dir)
		h.H.Sleep(wait)
		if !withDelay {
			_, _ = h.Raw(ctx, wt, nil, "shutdown")
		}
		return sizeAfterBuild, duMB(dir), true
	}
	_, withDelay, ok1 := gc(true, 20*time.Second)
	buildDefault, afterExit, ok2 := gc(false, time.Second)
	if ok1 {
		rep.Keys["e5_size_mb_with_idle_delay"] = withDelay
		rep.Keys["e5_gc_trimmed_with_idle_delay"] = withDelay <= 100
	}
	if ok2 {
		rep.Keys["e5_size_mb_default_delay"] = afterExit
		rep.Keys["e5_default_delay_untrimmed"] = afterExit >= buildDefault
	}
	if ok1 && ok2 {
		rep.Check("e5", withDelay <= 100 && afterExit >= buildDefault,
			"with max_idle_secs=30 and a 10 s idle delay du -sm is at most 100 after 20 s; with the default 5 m delay the size is unchanged after the server exits")
	}
}

func layoutE6(h *Harness, rep *Report, o LayoutOpts, a, b string) {
	ctx := context.Background()
	fetch := append([]string{"build", "@rules_go//go"}, o.FetchArgs...)
	if out, err := h.Bazel(ctx, a, fetch...); err != nil || out.Code != 0 {
		rep.Errorf("e6: warm fetch in A failed")
		return
	}
	offline := append([]string{"--repository_disable_download"}, fetch...)
	ok := false
	_ = h.Gated("e6", func() error {
		out, err := h.Bazel(ctx, b, offline...)
		ok = err == nil && out.Code == 0
		return nil
	})
	rep.Keys["e6_offline_build_ok"] = ok
	ob, err := h.OutputBase(b)
	if err != nil {
		rep.Errorf("e6: %v", err)
		return
	}
	total, linked, ext, err := ExternalRepos(filepath.Join(ob, "external"))
	if err != nil {
		rep.Errorf("e6: %v", err)
		return
	}
	newKB := duKB(filepath.Join(ob, "external"))
	rep.Keys["e6_plain_repos"], rep.Keys["e6_plain_repos_symlinked"], rep.Keys["e6_extension_repos_listed"] = total, linked, ext
	rep.Keys["e6_new_kb_per_worktree"] = newKB
	rep.Keys["e6_contents_cache_gc_vs_live_base"] = "not measured: undocumented (the 14-day contents-cache GC may remove a repo a live idle output base links to)"
	rep.Check("e6", ok && total > 0 && total == linked && newKB < 50*1024,
		"B succeeds offline, every non-extension external repo is a symlink into the repo contents cache, and new bytes per worktree are under 50 MB (extension repos listed, not failed)")
}
