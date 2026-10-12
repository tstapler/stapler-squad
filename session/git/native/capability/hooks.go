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
		dir = f.expandPath(e.Value)
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(f.l.root, dir) // hooks run from the worktree top
		}
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
