package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Harness is the shared plumbing of the S6 check tools: scratch worktrees, the wrapper, the quiet-host gate.
type Harness struct {
	X          Exec
	H          Host
	Repo, Ref  string
	Dir        string // scratch base, under ~/.cache/tmp/<task>/, never /tmp
	DiskCache  string
	AllowNoisy bool
	QuietWait  time.Duration
	Slots      string // BAZEL_WRAPPER_SLOTS passed to the wrapper; "" keeps the wrapper default (2)
	Warn       func(string)
	Readings   []Reading
}

func (h *Harness) warn(s string) {
	if h.Warn != nil {
		h.Warn(s)
	}
}

// Gated runs fn between a Gate and a Finish and keeps the reading for not_quiet tagging.
func (h *Harness) Gated(label string, fn func() error) error {
	r, err := Gate(h.H, label, h.AllowNoisy, h.QuietWait, h.warn)
	if err != nil {
		return err
	}
	err = fn()
	r.Finish(h.H)
	h.Readings = append(h.Readings, r)
	return err
}

func (h *Harness) git(args ...string) error {
	o, err := h.X.Run(context.Background(), Cmd{Name: "git", Args: append([]string{"-C", h.Repo}, args...)})
	if err == nil && o.Code != 0 {
		err = fmt.Errorf("exit %d: %s", o.Code, strings.TrimSpace(o.Stderr))
	}
	if err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// AddWT creates a detached worktree at Dir/name/d0/.../wt (depth nested directories, so path length varies).
func (h *Harness) AddWT(name string, depth int) (string, error) {
	p := filepath.Join(h.Dir, name)
	for i := 0; i < depth; i++ {
		p = filepath.Join(p, fmt.Sprintf("d%d", i))
	}
	p = filepath.Join(p, "wt")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	return p, h.git("worktree", "add", "--quiet", "--detach", p, h.Ref)
}

// DropWT shuts the worktree's server down, removes its output base and the worktree.
func (h *Harness) DropWT(wt string) {
	ctx := context.Background()
	ob, _ := h.OutputBase(wt)
	_, _ = h.Bazel(ctx, wt, "shutdown")
	if ob != "" {
		_ = os.RemoveAll(ob)
	}
	_ = h.git("worktree", "remove", "--force", wt)
}

func (h *Harness) env(extra ...string) []string {
	bz, _ := lookPath("bazelisk")
	e := []string{"BAZEL_REAL=" + bz, "BAZEL_AUTO_DETACH_SECS=0"}
	if h.Slots != "" {
		e = append(e, "BAZEL_WRAPPER_SLOTS="+h.Slots)
	}
	return append(e, extra...)
}

// Bazel runs the repo's tools/bazel wrapper (slot queue, memory scope) in wt.
func (h *Harness) Bazel(ctx context.Context, wt string, args ...string) (Out, error) {
	return h.X.Run(ctx, Cmd{Dir: wt, Env: h.env(), Name: filepath.Join(wt, "tools", "bazel"), Args: args})
}

// BazeliskDirect bypasses the wrapper's queue (true concurrency), still with the startup flags the wrapper sets.
func (h *Harness) BazeliskDirect(ctx context.Context, wt string, args ...string) (Out, error) {
	return h.Raw(ctx, wt, []string{"--host_jvm_args=-Xmx2g", "--max_idle_secs=300"}, args...)
}

// Raw runs bazelisk with explicit startup options and no wrapper.
func (h *Harness) Raw(ctx context.Context, wt string, startup []string, args ...string) (Out, error) {
	return h.X.Run(ctx, Cmd{Dir: wt, Name: "bazelisk", Args: append(append([]string(nil), startup...), args...)})
}

func (h *Harness) info(ctx context.Context, wt, key string) (string, error) {
	o, err := h.Bazel(ctx, wt, "info", key)
	if err == nil && o.Code != 0 {
		err = fmt.Errorf("bazel info %s exit %d", key, o.Code)
	}
	return strings.TrimSpace(o.Stdout), err
}

func (h *Harness) OutputBase(wt string) (string, error) {
	return h.info(context.Background(), wt, "output_base")
}

// BuildLogged builds with the shared disk cache and an execution log; the returned log path is JSON lines.
func (h *Harness) BuildLogged(ctx context.Context, wt, label string, extra ...string) (Out, string, error) {
	execLog := filepath.Join(h.Dir, label+".execlog.json")
	args := append([]string{"build", "--execution_log_json_file=" + execLog}, extra...)
	o, err := h.Bazel(ctx, wt, args...)
	_ = os.WriteFile(filepath.Join(h.Dir, label+".log"), []byte(o.Stdout+o.Stderr), 0o644)
	return o, execLog, err
}

// NoCache is the flag set of a cache-disabled build.
func NoCache() string { return "--disk_cache=" }

func (h *Harness) DiskFlag() string { return "--disk_cache=" + h.DiskCache }
