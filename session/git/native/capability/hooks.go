package capability

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

// hooks reports a repository with hooks: any executable regular file (or symlink to one) in the
// effective hooks directory whose name does not end in ".sample", or any hook.* config key. No
// list of hook names is kept; git's list is long and grows (ADR-006). Never cached. Read-only
// operations only see the config-driven programs real git runs for them (programs).
func (f *facts) hooks() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	if progs := f.programs(cfg); progs != nil {
		return progs, nil
	}
	if !f.op.Mutating() {
		return nil, nil
	}
	if hs := cfg.InSection("hook"); len(hs) > 0 {
		return one(backend.CapHooks, "config:"+hs[0].Name()), nil
	}
	dir := filepath.Join(f.l.commonDir, "hooks")
	if e, ok := cfg.Lookup("core.hooksPath"); ok && e.HasValue && e.Value != "" {
		resolved, detail := f.hooksPathDir(e.Value)
		if detail != "" {
			return one(backend.CapHooks, detail), nil
		}
		dir = resolved
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) || isNotDir(err) {
			return nil, nil // git finds no hook in a directory that does not exist
		}
		return nil, detectErr(backend.DetectHooksUnreadable, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sample") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fi, err := os.Stat(path) // follows a symlink to the hook
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if executable(path, fi) {
			return one(backend.CapHooks, e.Name()), nil
		}
	}
	return nil, nil
}

// hooksPathDir resolves core.hooksPath. A path git expands in ways this package does not
// (~user/, %(prefix)/) and a directory inside the worktree both report hooks outright: the
// latter is the husky/.githooks pattern, where the operation itself (checkout, merge, reset)
// can bring the hook files into existence after this check ran.
func (f *facts) hooksPathDir(value string) (dir, detail string) {
	if (strings.HasPrefix(value, "~") && !strings.HasPrefix(value, "~/")) || strings.HasPrefix(value, "%(") {
		return "", "core.hooksPath:unexpanded"
	}
	dir = f.expandPath(value)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(f.l.root, dir) // hooks run from the worktree top
	}
	// a symlink on either side gives the same directory two spellings: compare resolved paths
	// (the deepest existing ancestor of dir resolved, since the directory itself may not exist yet)
	for _, d := range []string{dir, resolveExisting(dir)} {
		for _, root := range []string{f.l.root, f.l.logical} {
			if rel, err := filepath.Rel(root, d); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", "core.hooksPath:inside-worktree"
			}
		}
	}
	return dir, ""
}

// resolveExisting resolves symlinks in the longest existing prefix of p and re-appends the rest.
func resolveExisting(p string) string {
	rest := ""
	for cur := p; ; cur = filepath.Dir(cur) {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		if parent := filepath.Dir(cur); parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
	}
}

// programs reports config-driven programs real git runs for the operation: core.fsmonitor (a
// command, run by status, add and diff) and diff.external or any diff.<driver>.textconv/command
// (run by diff). They are not hooks, but like hooks the in-process backend would silently skip them.
func (f *facts) programs(cfg *gitconfig.Result) []backend.Capability {
	if v, set, ok := cfg.Bool("core.fsmonitor"); set && (v || !ok) {
		return one(backend.CapHooks, "core.fsmonitor")
	}
	if f.op == backend.OpDiff || f.op == backend.OpDiffNumstat || f.op.Mutating() {
		if e, ok := cfg.Lookup("diff.external"); ok && e.HasValue && e.Value != "" {
			return one(backend.CapHooks, "diff.external")
		}
		for _, e := range cfg.InSection("diff") {
			if e.HasSubsection && (e.Key == "textconv" || e.Key == "command") {
				return one(backend.CapHooks, "diff."+e.Subsection+"."+e.Key)
			}
		}
	}
	return nil
}
