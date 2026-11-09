package main

import (
	"bufio"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	modulePath = "github.com/tstapler/stapler-squad"
	allMarker  = "__ALL__"
)

// maxGroup caps specs per e2e_test target; a var so tests can force one spec per group.
var maxGroup = 12

// GlobalInputs loads one glob per line (blank and # lines skipped). `**` matches any depth.
func loadGlobs(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

func globToRegexp(g string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(g); i++ {
		switch {
		case strings.HasPrefix(g[i:], "**/"):
			b.WriteString("(.*/)?")
			i += 2
		case strings.HasPrefix(g[i:], "**"):
			b.WriteString(".*")
			i++
		case g[i] == '*':
			b.WriteString("[^/]*")
		case g[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(g[i])))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

func matchesAny(globs []string, file string) bool {
	for _, g := range globs {
		if globToRegexp(g).MatchString(file) {
			return true
		}
	}
	return false
}

// Group is one e2e_test target: specs of a filename family run together so one server boot is amortised.
type Group struct {
	Name  string
	Specs []string
}

func family(spec string) string {
	base := path.Base(spec)
	if i := strings.IndexAny(base, "-."); i > 0 {
		return base[:i]
	}
	return base
}

// groupSpecs puts families with 3+ specs in their own target (chunked at maxGroup) and packs the rest
// alphabetically into misc-N targets, so the target count stays inside the 20-60 band of Story 12.2.
func groupSpecs(specs []string) []Group {
	fam := map[string][]string{}
	for _, s := range specs {
		fam[family(s)] = append(fam[family(s)], s)
	}
	names := make([]string, 0, len(fam))
	for n := range fam {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []Group
	var small []string
	for _, n := range names {
		if len(fam[n]) < 3 {
			small = append(small, fam[n]...)
			continue
		}
		out = append(out, chunk(sanitize(n), fam[n])...)
	}
	sort.Strings(small)
	out = append(out, chunk("misc", small)...)
	return out
}

func chunk(name string, specs []string) []Group {
	var out []Group
	for i := 0; i < len(specs); i += maxGroup {
		end := min(i+maxGroup, len(specs))
		g := Group{Name: name, Specs: specs[i:end]}
		if len(specs) > maxGroup {
			g.Name = name + "_" + itoa(i/maxGroup+1)
		}
		out = append(out, g)
	}
	return out
}

func sanitize(s string) string {
	return regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(s, "_")
}

func itoa(i int) string { return strconv.Itoa(i) }

// Selection is the result of mapping a diff onto the manifest.
type Selection struct {
	All       bool
	Groups    []string // target names, sorted
	Specs     []string // specs inside the selected groups
	Unmanaged []string
	Total     int
	PerSpec   int      // replay-fine: specs hit before group widening
	Hit       []string // replay-fine: the specs hit before group widening, sorted
}

// Share is the fraction of specs that run (1.0 when everything is selected).
func (s Selection) Share() float64 {
	if s.All {
		return 1
	}
	if s.Total == 0 {
		return 0
	}
	return float64(len(s.Specs)) / float64(s.Total)
}

// Selector maps changed files to the specs whose observed slice contains them.
type Selector struct {
	m        *Manifest
	globals  []string
	onDisk   []string
	groups   []Group
	specPkgs map[string]map[string]bool
}

// newSelector builds a selector. closure may be nil (observed packages only); otherwise each spec's
// package set is widened by the static import closure so unexecuted-but-linked packages count.
func newSelector(m *Manifest, globals, specsOnDisk []string, closure func(pkgs []string) []string) *Selector {
	s := &Selector{m: m, globals: globals, onDisk: specsOnDisk, specPkgs: map[string]map[string]bool{}}
	all := append([]string(nil), specsOnDisk...)
	known := map[string]bool{}
	for _, e := range m.Specs {
		known[e.Spec] = true
	}
	for _, e := range m.Specs {
		if !contains(all, e.Spec) {
			all = append(all, e.Spec)
		}
		pk := e.GoPkgs
		if closure != nil && len(pk) > 0 {
			pk = closure(pk)
		}
		set := map[string]bool{}
		for _, p := range pk {
			set[p] = true
		}
		s.specPkgs[e.Spec] = set
	}
	sort.Strings(all)
	s.groups = groupSpecs(all)
	return s
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// pkgOf maps a changed file to its Go import path (the file's directory under the module).
func pkgOf(file string) string {
	d := path.Dir(file)
	if d == "." {
		return modulePath
	}
	return modulePath + "/" + d
}

// isProdGoSurface reports whether a change to file can alter the server binary's behaviour.
func isProdGoSurface(file string) bool {
	switch {
	case strings.HasSuffix(file, "_test.go"), strings.HasPrefix(file, "tests/"), strings.HasPrefix(file, "docs/"), strings.HasPrefix(file, "project_plans/"),
		strings.HasPrefix(file, "tools/"), strings.HasPrefix(file, "scripts/"), strings.HasPrefix(file, ".claude/"), strings.HasPrefix(file, ".github/"):
		return false
	case strings.HasSuffix(file, ".md"):
		return false
	}
	return true
}

// Select maps changed files (repo-relative, slash paths; deleted files included) to targets.
func (s *Selector) Select(changed []string) Selection {
	sel := Selection{Total: len(s.groupSpecsAll())}
	hit := map[string]bool{}
	byEntry := map[string]Entry{}
	for _, e := range s.m.Specs {
		byEntry[e.Spec] = e
	}
	for _, f := range changed {
		if matchesAny(s.globals, f) {
			sel.All = true
		}
		for _, sp := range s.groupSpecsAll() {
			e, managed := byEntry[sp]
			switch {
			case f == sp:
				hit[sp] = true
			case !managed || e.Excluded != "":
				// always runs; added below
			case strings.HasSuffix(f, ".go") && isProdGoSurface(f) && s.specPkgs[sp][pkgOf(f)]:
				hit[sp] = true
			case !strings.HasSuffix(f, ".go") && isProdGoSurface(f) && s.specPkgs[sp][pkgOf(f)]:
				hit[sp] = true // go:embed assets and cgo sources in an observed package dir
			case contains(e.WebFiles, f):
				hit[sp] = true
			}
		}
	}
	for _, sp := range s.groupSpecsAll() {
		e, managed := byEntry[sp]
		if !managed {
			sel.Unmanaged = append(sel.Unmanaged, sp)
			hit[sp] = true
		} else if e.Excluded != "" {
			hit[sp] = true
		}
	}
	sel.Unmanaged = uniqSorted(sel.Unmanaged)
	for _, g := range s.groups {
		for _, sp := range g.Specs {
			if hit[sp] || sel.All {
				sel.Groups = append(sel.Groups, g.Name)
				sel.Specs = append(sel.Specs, g.Specs...)
				break
			}
		}
	}
	sort.Strings(sel.Groups)
	sort.Strings(sel.Specs)
	return sel
}

func (s *Selector) groupSpecsAll() []string {
	var out []string
	for _, g := range s.groups {
		out = append(out, g.Specs...)
	}
	return out
}
