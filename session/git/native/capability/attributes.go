package capability

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

const maxAttributesSize = 8 << 20

func isNotDir(err error) bool { return errors.Is(err, syscall.ENOTDIR) }

// filters reports a clean/smudge filter driver that applies: some attribute source assigns
// filter=<name> (directly or through a macro) and the effective config defines
// filter.<name>.clean, .smudge, .process or .required. A global `git lfs install` defines
// filter.lfs.* for every repository, so config alone must not route; the attribute has to
// reference it. With no driver defined at all there is nothing to look for, so the worktree is
// not scanned.
func (f *facts) filters() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	defined := definedFilters(cfg)
	if len(defined) == 0 {
		return nil, nil
	}
	if treeChangingOps[f.op] {
		// the attributes that apply after the operation come from the tree it brings in
		names := make([]string, 0, len(defined))
		for n := range defined {
			names = append(names, n)
		}
		sort.Strings(names)
		return one(backend.CapLFS, "filter="+names[0]+":attributes may arrive with the operation"), nil
	}
	paths, err := f.attributeSources(cfg)
	if err != nil {
		return nil, err
	}
	sources := make([][]byte, 0, len(paths))
	for _, p := range paths {
		data, err := readAttributes(p)
		if err != nil {
			return nil, err
		}
		sources = append(sources, data)
	}
	for _, n := range assignedFilterNames(sources) {
		if defined[n] {
			return one(backend.CapLFS, "filter="+n), nil
		}
	}
	return nil, nil
}

// definedFilters are the driver names the config configures. "required" counts: git dies when a
// required filter is assigned but has no working driver, which the CLI must report.
func definedFilters(cfg *gitconfig.Result) map[string]bool {
	names := map[string]bool{}
	for _, e := range cfg.InSection("filter") {
		if !e.HasSubsection {
			continue
		}
		switch e.Key {
		case "clean", "smudge", "process", "required":
			names[e.Subsection] = true
		}
	}
	return names
}

// maxWalkEntries bounds the worktree walk Add needs; a worktree this large is left to git.
const maxWalkEntries = 500_000

// treeChangingOps bring files, and so .gitattributes files, into the worktree from another
// tree; the attributes in force afterwards are not the ones on disk now.
var treeChangingOps = map[backend.OperationName]bool{
	backend.OpSwitchBranch: true, backend.OpCheckoutCommit: true, backend.OpStashPop: true, backend.OpReset: true,
	backend.OpRestore: true, backend.OpPull: true, backend.OpAddWorktree: true, backend.OpAddWorktreeForExistingBranch: true,
}

// attributeSources lists every file git consults for attributes: .gitattributes in the root and
// in every directory holding tracked files (which covers a new untracked file in a tracked
// directory, e.g. after `git lfs track`), info/attributes, core.attributesFile (default under
// XDG) and the system files. Add also walks the worktree, because it takes in untracked
// directories whose attribute files appear in no index.
func (f *facts) attributeSources(cfg *gitconfig.Result) ([]string, error) {
	srcs := []string{filepath.Join(f.l.root, ".gitattributes")}
	idx, err := f.indexFacts()
	if err != nil {
		return nil, err
	}
	for _, dir := range idx.dirs {
		srcs = append(srcs, filepath.Join(f.l.root, filepath.FromSlash(dir), ".gitattributes"))
	}
	if f.op == backend.OpAdd {
		walked, _, err := f.walkWorktree(idx)
		if err != nil {
			return nil, err
		}
		srcs = append(srcs, walked...)
	}
	srcs = append(srcs, filepath.Join(f.l.commonDir, "info", "attributes"))
	if e, ok := cfg.Lookup("core.attributesFile"); ok && e.HasValue && e.Value != "" {
		p := f.expandPath(e.Value)
		if !filepath.IsAbs(p) {
			p = filepath.Join(f.l.root, p)
		}
		srcs = append(srcs, p)
	} else if xdg := envGet(f.env, "XDG_CONFIG_HOME"); xdg != "" {
		srcs = append(srcs, filepath.Join(xdg, "git", "attributes"))
	} else if home := envGet(f.env, "HOME"); home != "" {
		srcs = append(srcs, filepath.Join(home, ".config", "git", "attributes"))
	}
	return append(srcs, f.d.systemAttributes()...), nil
}

// walkWorktree lists every .gitattributes and every nested repository not recorded as a
// gitlink. Nested repositories are not entered (git adds them as gitlinks). Symlinked
// directories are not followed, as in git.
func (f *facts) walkWorktree(idx *indexFacts) (attrs, nested []string, err error) {
	entries := 0
	werr := filepath.WalkDir(f.l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == f.l.root {
				return err
			}
			return nil // a directory that vanished or cannot be listed has no attribute file git could read either
		}
		if entries++; entries > maxWalkEntries {
			return errWalkTooLarge
		}
		if ctxErr := f.ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			if path != f.l.root {
				if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
					if rel, rerr := filepath.Rel(f.l.root, path); rerr == nil && !idx.gitlinkPaths[filepath.ToSlash(rel)] {
						nested = append(nested, path)
					}
					return fs.SkipDir
				}
			}
			return nil
		}
		if d.Name() == ".gitattributes" {
			attrs = append(attrs, path)
		}
		return nil
	})
	switch {
	case errors.Is(werr, errWalkTooLarge):
		return nil, nil, detectErr(backend.DetectAttributesUnread, werr)
	case f.ctx.Err() != nil:
		return nil, nil, detectErr(backend.DetectCanceled, f.ctx.Err())
	case werr != nil:
		return nil, nil, detectErr(backend.DetectAttributesUnread, werr)
	}
	return attrs, nested, nil
}

var errWalkTooLarge = errors.New("worktree too large to scan for attribute files")

func (d *Detector) systemAttributes() []string {
	if d.cfg.SystemAttributes != nil {
		return d.cfg.SystemAttributes
	}
	out := []string{"/etc/gitattributes"}
	for _, c := range append(append([]string(nil), d.cfg.SystemVendor...), d.cfg.SystemDefault...) {
		out = append(out, filepath.Join(filepath.Dir(c), "gitattributes"))
	}
	return out
}

// readAttributes returns an attributes file's content. A missing file is empty; one that exists
// but cannot be read fails closed.
func readAttributes(path string) ([]byte, error) {
	if path == os.DevNull {
		return nil, nil // core.attributesFile=/dev/null is the usual way to disable the file
	}
	data, err := gitconfig.ReadRegularFile(path, maxAttributesSize)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
			return nil, nil
		}
		return nil, detectErr(backend.DetectAttributesUnread, err)
	}
	return data, nil
}

// assignedFilterNames returns every filter driver name any source assigns. `[attr]` macros are
// collected from all sources first (git only honours them in the top-level, info, global and
// system files; taking them from every file errs toward routing) and expanded recursively.
func assignedFilterNames(sources [][]byte) []string {
	macros := map[string][]string{} // macro name -> its attribute list
	var lines [][]string
	for _, data := range sources {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64<<10), maxAttributesSize)
		for sc.Scan() {
			text := strings.TrimSpace(sc.Text())
			if text == "" || strings.HasPrefix(text, "#") {
				continue
			}
			fields := splitAttrFields(text)
			if len(fields) < 2 {
				continue
			}
			if name, ok := strings.CutPrefix(fields[0], "[attr]"); ok {
				macros[name] = fields[1:]
				continue
			}
			lines = append(lines, fields[1:])
		}
	}
	names := make([]string, 0, len(lines))
	for _, attrs := range lines {
		names = append(names, expandFilters(attrs, macros, map[string]bool{}, 0)...)
	}
	return names
}

const maxMacroDepth = 10

func expandFilters(attrs []string, macros map[string][]string, seen map[string]bool, depth int) []string {
	var names []string
	for _, a := range attrs {
		if v, ok := strings.CutPrefix(a, "filter="); ok && v != "" {
			names = append(names, v)
			continue
		}
		body, isMacro := macros[a]
		if !isMacro || seen[a] || depth >= maxMacroDepth {
			continue
		}
		seen[a] = true
		names = append(names, expandFilters(body, macros, seen, depth+1)...)
		delete(seen, a)
	}
	return names
}

// splitAttrFields splits on whitespace, keeping a quoted leading pattern whole.
func splitAttrFields(s string) []string {
	if strings.HasPrefix(s, `"`) {
		if end := strings.Index(s[1:], `"`); end >= 0 {
			return append([]string{s[:end+2]}, strings.Fields(s[end+2:])...)
		}
	}
	return strings.Fields(s)
}
