package capability

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

const maxAttributesSize = 8 << 20

func isNotDir(err error) bool { return errors.Is(err, syscall.ENOTDIR) }

// filters reports a clean/smudge filter driver that applies: some attribute source assigns
// filter=<name> and the effective config defines filter.<name>.clean, .smudge or .process. A
// global `git lfs install` defines filter.lfs.* for every repository, so config alone must not
// route; the attribute has to reference it.
func (f *facts) filters() ([]backend.Capability, error) {
	cfg, err := f.config()
	if err != nil {
		return nil, err
	}
	defined := func(name string) bool {
		for _, k := range []string{"clean", "smudge", "process"} {
			if _, ok := cfg.Lookup("filter." + name + "." + k); ok {
				return true
			}
		}
		return false
	}
	idx, err := f.indexFacts()
	if err != nil {
		return nil, err
	}
	for _, p := range f.attributeSources(cfg, idx) {
		names, err := filterNames(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if defined(n) {
				return one(backend.CapLFS, "filter="+n), nil
			}
		}
	}
	return nil, nil
}

// attributeSources lists every file git consults for attributes: the root .gitattributes, every
// tracked one, info/attributes, core.attributesFile (default under XDG) and the system files.
func (f *facts) attributeSources(cfg *gitconfig.Result, idx *indexFacts) []string {
	srcs := []string{filepath.Join(f.l.root, ".gitattributes")}
	for _, dir := range idx.attributeDirs {
		if dir != "." {
			srcs = append(srcs, filepath.Join(f.l.root, filepath.FromSlash(dir), ".gitattributes"))
		}
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
	return append(srcs, f.d.systemAttributes()...)
}

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

// filterNames returns every filter driver name an attributes file assigns, following `[attr]`
// macros defined in the same file. A missing file has none; one that exists but cannot be read
// fails closed.
func filterNames(path string) ([]string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || isNotDir(err) {
			return nil, nil
		}
		return nil, detectErr(backend.DetectAttributesUnread, err)
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxAttributesSize {
		return nil, detectErr(backend.DetectAttributesUnread, errors.New("not a plain, reasonably sized file"))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, detectErr(backend.DetectAttributesUnread, err)
	}
	return parseFilterNames(data), nil
}

// parseFilterNames scans attribute lines ("<pattern> attr attr=value ...", "[attr]macro attrs...").
func parseFilterNames(data []byte) []string {
	macros := map[string][]string{} // macro name -> filter names it assigns
	type line struct{ attrs []string }
	var lines []line
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
			macros[name] = assignedFilters(fields[1:], nil)
			continue
		}
		lines = append(lines, line{attrs: fields[1:]})
	}
	names := make([]string, 0, len(lines))
	for _, l := range lines {
		names = append(names, assignedFilters(l.attrs, macros)...)
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

func assignedFilters(attrs []string, macros map[string][]string) []string {
	var names []string
	for _, a := range attrs {
		if v, ok := strings.CutPrefix(a, "filter="); ok && v != "" {
			names = append(names, v)
			continue
		}
		names = append(names, macros[a]...)
	}
	return names
}
