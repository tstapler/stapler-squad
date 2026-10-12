package capability

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// hooks reports a repository with hooks: any executable regular file (or symlink to one) in the
// effective hooks directory whose name does not end in ".sample", or any hook.* config key. No
// list of hook names is kept; git's list is long and grows (ADR-006). Never cached.
func (f *facts) hooks() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
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
	if rel, err := filepath.Rel(f.l.root, dir); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "core.hooksPath:inside-worktree"
	}
	return dir, ""
}
