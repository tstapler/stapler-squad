package termscan

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

// Source lists and reads the files to scan.
type Source interface {
	Files() ([]string, error)
	// Read returns the content, or (nil, nil) if the file is not readable text
	// (deleted from the working tree but still in the index).
	Read(path string) ([]byte, error)
}

// MemSource is an in-memory Source for tests.
type MemSource map[string][]byte

// Files lists the paths in sorted order.
func (m MemSource) Files() ([]string, error) {
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// Read returns the stored content.
func (m MemSource) Read(p string) ([]byte, error) { return m[p], nil }

// GitSource lists tracked files from the index and reads them from the working tree.
type GitSource struct {
	Repo *gitbase.Repo
	Root string
}

// Files lists the index.
func (g GitSource) Files() ([]string, error) { return g.Repo.Files() }

// Read reads a file from the working tree; a missing file yields (nil, nil).
func (g GitSource) Read(p string) ([]byte, error) {
	full := filepath.Join(g.Root, filepath.FromSlash(p))
	if fi, err := os.Stat(full); err == nil && fi.IsDir() {
		return nil, nil // gitlink or directory entry
	}
	data, err := os.ReadFile(full)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return data, err
}

// DirSource scans every file under Dir (website/dist, DOM text dumps).
type DirSource struct{ Dir string }

// Files walks Dir and returns slash paths relative to it.
func (d DirSource) Files() ([]string, error) {
	var out []string
	err := filepath.WalkDir(d.Dir, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, err := filepath.Rel(d.Dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

// Read reads one file.
func (d DirSource) Read(p string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.Dir, filepath.FromSlash(p)))
}

// Finding is one banned term on one line (Line 0 means the file name).
type Finding struct {
	Path string
	Line int
	Term string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: banned term %q", f.Path, f.Line, f.Term) }

// Result is a scan outcome.
type Result struct {
	Findings []Finding
	// Stale lists allow-list entries whose file no longer contains a banned term.
	Stale []AllowEntry
}

// Clean reports whether the scan found nothing to fail on.
func (r Result) Clean() bool { return len(r.Findings) == 0 && len(r.Stale) == 0 }

// Options tune a scan.
type Options struct {
	Allow []AllowEntry
	// AllPaths applies PublishedOnly terms everywhere and skips the source-only
	// skip rules (dist and media-text scans: everything there is published).
	AllPaths bool
	// ScanNames also checks each file name.
	ScanNames bool
}

func isLockfile(base string) bool {
	switch base {
	case "go.sum", "bun.lock", "bun.lockb", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "CHANGELOG.md":
		return true
	}
	return strings.HasSuffix(base, ".lock")
}

// ownDirs hold the scanner, its fixtures and allow-lists, which name the terms by design.
func ownDirs() []string {
	return []string{
		"tools/promolib/termscan/", "tools/promo/cmd/termcheck/",
		"tools/promo/internal/termcheck/", "tools/promo/termcheck/",
	}
}

// SkipPath reports whether source-mode scanning ignores p: lockfiles, the
// scanner's own directories, and any .next, node_modules or project_plans path
// component. `build` is skipped only as the first component (root build output),
// because web-app/build is real source.
func SkipPath(p string) bool {
	base := path.Base(p)
	if isLockfile(base) {
		return true
	}
	for _, d := range ownDirs() {
		if strings.HasPrefix(p, d) {
			return true
		}
	}
	parts := strings.Split(p, "/")
	for i, c := range parts[:len(parts)-1] {
		switch c {
		case ".next", "node_modules", "project_plans":
			return true
		case "build":
			if i == 0 {
				return true
			}
		}
	}
	return false
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}

func lineHits(terms []Term, p, line string, all bool) []string {
	var hits []string
	for _, t := range terms {
		if t.PublishedOnly && !all && !IsPublishedPath(p) {
			continue
		}
		if t.Matches(line) {
			hits = append(hits, t.Text)
		}
	}
	return hits
}

// Scan checks every file of src against the banned terms.
func Scan(src Source, opts Options) (Result, error) {
	files, err := src.Files()
	if err != nil {
		return Result{}, err
	}
	allowed := make(map[string]bool, len(opts.Allow))
	for _, a := range opts.Allow {
		allowed[a.Path] = true
	}
	terms := BannedTermSet()
	var res Result
	for _, p := range files {
		if !opts.AllPaths && SkipPath(p) {
			continue
		}
		fs, err := scanFile(src, p, terms, opts)
		if err != nil {
			return Result{}, err
		}
		if len(fs) == 0 {
			continue
		}
		if allowed[p] {
			continue
		}
		res.Findings = append(res.Findings, fs...)
	}
	res.Stale = staleEntries(src, opts.Allow, files, terms, opts)
	sort.Slice(res.Findings, func(i, j int) bool {
		a, b := res.Findings[i], res.Findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return res, nil
}

func scanFile(src Source, p string, terms []Term, opts Options) ([]Finding, error) {
	var out []Finding
	if opts.ScanNames {
		for _, h := range lineHits(terms, p, p, true) {
			out = append(out, Finding{Path: p, Line: 0, Term: h})
		}
	}
	data, err := src.Read(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if data == nil || isBinary(data) {
		return out, nil
	}
	for i, line := range strings.Split(string(data), "\n") {
		for _, h := range lineHits(terms, p, line, opts.AllPaths) {
			out = append(out, Finding{Path: p, Line: i + 1, Term: h})
		}
	}
	return out, nil
}

func staleEntries(src Source, allow []AllowEntry, files []string, terms []Term, opts Options) []AllowEntry {
	listed := make(map[string]bool, len(files))
	for _, f := range files {
		listed[f] = true
	}
	var stale []AllowEntry
	for _, a := range allow {
		if !listed[a.Path] {
			stale = append(stale, a)
			continue
		}
		fs, err := scanFile(src, a.Path, terms, opts)
		if err != nil || len(fs) == 0 {
			stale = append(stale, a)
		}
	}
	return stale
}
