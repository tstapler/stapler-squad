package main

import (
	"path/filepath"
	"time"
)

// LayeringOpts configures Task 0.12f / E8: disk and remote cache layers compose.
type LayeringOpts struct {
	Target     string
	RemoteBin  string   // bazel-remote binary (not installed by this repo; flag names below are UNVERIFIED)
	RemoteArgs []string // overrides the default argument template when the binary's flags differ
	Port       string   // loopback HTTP port from the manual dev block (62876-62880)
	KillAfter  time.Duration
}

func (o LayeringOpts) remoteArgs(dir string) []string {
	if len(o.RemoteArgs) > 0 {
		return o.RemoteArgs
	}
	return []string{"--dir", dir, "--max_size", "5", "--http_address", "127.0.0.1:" + o.Port, "--grpc_address", "127.0.0.1:0"}
}

// RemoteHits counts spawns served by the remote cache.
func RemoteHits(sp []Spawn) int {
	n := 0
	for _, s := range sp {
		if s.Runner == "remote cache hit" {
			n++
		}
	}
	return n
}

// Layering runs E8 on loopback: A uploads, B (empty disk cache) builds with both layers, C builds with the remote
// killed mid-build and is compared with a no-remote run.
func Layering(h *Harness, o LayeringOpts) *Report {
	rep := NewReport()
	remoteURL := "http://127.0.0.1:" + o.Port
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
	a, b, c, d := mk("y-a", 0), mk("y-b", 2), mk("y-c", 1), mk("y-d", 3)
	if a == "" || b == "" || c == "" || d == "" {
		return rep
	}
	rdir := filepath.Join(h.Dir, "bazel-remote-data")
	proc, err := h.X.Start(Cmd{Name: o.RemoteBin, Args: o.remoteArgs(rdir)})
	if err != nil {
		rep.Errorf("start bazel-remote: %v", err)
		return rep
	}
	defer func() { _ = proc.Kill() }()
	h.H.Sleep(2 * time.Second)
	diskB := filepath.Join(h.Dir, "y-disk-b")
	if _, _, ok := h.timedBuild(rep, a, "e8-a", "--remote_cache="+remoteURL, o.Target); !ok {
		return rep
	}
	before := duKB(diskB)
	_, sp, ok := h.timedBuild(rep, b, "e8-b", "--remote_cache="+remoteURL, "--disk_cache="+diskB, o.Target)
	if !ok {
		return rep
	}
	hits, after := RemoteHits(sp), duKB(diskB)
	rep.Keys["b_remote_hits"], rep.Keys["b_disk_cache_kb_before"], rep.Keys["b_disk_cache_kb_after"] = hits, before, after
	noRemote, _, ok2 := h.timedBuild(rep, d, "e8-d-noremote", "--disk_cache="+filepath.Join(h.Dir, "y-disk-d"), o.Target)
	if !ok2 {
		return rep
	}
	killed := make(chan struct{})
	go func() {
		h.H.Sleep(o.KillAfter)
		_ = proc.Kill()
		close(killed)
	}()
	withKill, _, ok3 := h.timedBuild(rep, c, "e8-c-killed", "--remote_cache="+remoteURL, "--disk_cache="+filepath.Join(h.Dir, "y-disk-c"), o.Target)
	<-killed
	if !ok3 {
		return rep
	}
	rep.Keys["c_delta_s"] = withKill - noRemote
	rep.Check("e8", hits > 0 && after > before && withKill-noRemote <= 10,
		"B's hits are remote hits, B's disk cache grew, and C completes with at most 10 s added over a no-remote run with the remote killed mid-build")
	return rep
}
