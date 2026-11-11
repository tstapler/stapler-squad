// Package main selects the Bazel test targets (and non-Bazel CI jobs) affected by a git diff.
//
// The selection is a pure function over a Graph, so it is testable without Bazel. Deleted, renamed-away
// and non-graph files never under-select: they widen to a package or to //... (Story 7.3).
package main

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
)

const allTargets = "//..."

// e2ePrereqs must be built whenever web-app sources change so the e2e jobs have their binary and bundle.
var e2ePrereqs = []string{"//:stapler-squad", "//web-app:dist"}

// ChangeStatus is the git name-status letter of one changed path.
type ChangeStatus byte

const (
	Modified ChangeStatus = 'M'
	Deleted  ChangeStatus = 'D'
)

// Change is one path in the diff. A rename is a Deleted change for the old path plus a Modified one for the new.
type Change struct {
	Path   string
	Status ChangeStatus
}

// Graph is the part of Bazel the selection needs.
type Graph interface {
	// PackageOf returns the label prefix ("//session") of the nearest enclosing BUILD package, or false when
	// the path has none or sits under a .bazelignore'd directory.
	PackageOf(p string) (string, bool)
	// InGraph reports whether the file label resolves to a target (a source file named by some rule).
	InGraph(label string) bool
	// Tests returns the test targets among the reverse dependencies of the seeds.
	Tests(seeds []string) ([]string, error)
}

// Rules holds the two checked-in path lists.
type Rules struct {
	Global   []string        // globs that force //...
	NonBazel []NonBazelInput // globs that select non-Bazel jobs
}

// NonBazelInput maps a glob to the CI jobs it selects.
type NonBazelInput struct {
	Glob string
	Jobs []string
}

// Result is the selection: Targets are Bazel labels, Jobs are non-Bazel job names.
type Result struct {
	Targets []string
	Jobs    []string
}

// ParseGlobList reads one glob per line; blank lines and # comments are skipped.
func ParseGlobList(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if l := cleanLine(sc.Text()); l != "" {
			out = append(out, l)
		}
	}
	return out, sc.Err()
}

// ParseNonBazel reads `glob job[,job...]` lines.
func ParseNonBazel(r io.Reader) ([]NonBazelInput, error) {
	var out []NonBazelInput
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		l := cleanLine(sc.Text())
		if l == "" {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, fmt.Errorf("line %d: want `glob job[,job]`, got %q", n, l)
		}
		out = append(out, NonBazelInput{Glob: f[0], Jobs: strings.Split(f[1], ",")})
	}
	return out, sc.Err()
}

func cleanLine(s string) string {
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// GlobMatch matches a repo-relative path: '**' crosses directories, '*' and '?' do not.
func GlobMatch(glob, p string) bool {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch c := glob[i]; {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			b.WriteString(".*")
			i++
			if i+1 < len(glob) && glob[i+1] == '/' {
				i++
				b.WriteString("(?:/)?")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String()).MatchString(p)
}

func matchAny(globs []string, p string) bool {
	for _, g := range globs {
		if GlobMatch(g, p) {
			return true
		}
	}
	return false
}

// Select computes the affected set. Any error from the graph widens to //... instead of failing.
func Select(changes []Change, rules Rules, g Graph) Result {
	jobs := map[string]bool{}
	seeds := map[string]bool{}
	webChanged := false
	for _, c := range changes {
		if matchAny(rules.Global, c.Path) {
			return Result{Targets: []string{allTargets}, Jobs: jobNames(jobs, rules, changes)}
		}
		if strings.HasPrefix(c.Path, "web-app/src/") {
			webChanged = true
		}
		if mapped := nonBazelJobs(rules, c.Path); len(mapped) > 0 {
			for _, j := range mapped {
				jobs[j] = true
			}
			continue
		}
		seed, ok := seedFor(c, g)
		if !ok {
			return Result{Targets: []string{allTargets}, Jobs: sortedKeys(jobs)}
		}
		seeds[seed] = true
	}
	var targets []string
	if len(seeds) > 0 {
		ts, err := g.Tests(sortedKeys(seeds))
		if err != nil {
			return Result{Targets: []string{allTargets}, Jobs: sortedKeys(jobs)}
		}
		targets = ts
	}
	if webChanged {
		targets = append(targets, e2ePrereqs...)
	}
	return Result{Targets: dedupe(targets), Jobs: sortedKeys(jobs)}
}

func jobNames(jobs map[string]bool, rules Rules, changes []Change) []string {
	for _, c := range changes {
		for _, j := range nonBazelJobs(rules, c.Path) {
			jobs[j] = true
		}
	}
	return sortedKeys(jobs)
}

func nonBazelJobs(rules Rules, p string) []string {
	var out []string
	for _, nb := range rules.NonBazel {
		if GlobMatch(nb.Glob, p) {
			out = append(out, nb.Jobs...)
		}
	}
	return out
}

// seedFor picks the rdeps seed for one path: the file label, its package for a deleted or unreferenced file,
// or nothing (false) when the path is outside the graph and the caller must select //....
func seedFor(c Change, g Graph) (string, bool) {
	pkg, ok := g.PackageOf(c.Path)
	if !ok {
		return "", false
	}
	if c.Status == Deleted {
		return pkg + ":all", true
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(c.Path, strings.TrimPrefix(pkg, "//")), "/")
	label := pkg + ":" + rel
	if g.InGraph(label) {
		return label, true
	}
	return pkg + ":all", true
}

// ParseNameStatus parses `git diff --name-status` lines (M/A/D/R<score>/C<score>, tab separated).
func ParseNameStatus(r io.Reader) ([]Change, error) {
	var out []Change
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Split(strings.TrimRight(sc.Text(), "\r\n"), "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		switch f[0][0] {
		case 'D':
			out = append(out, Change{path.Clean(f[1]), Deleted})
		case 'R':
			if len(f) < 3 {
				return nil, fmt.Errorf("rename line needs two paths: %q", sc.Text())
			}
			out = append(out, Change{path.Clean(f[1]), Deleted}, Change{path.Clean(f[2]), Modified})
		default:
			out = append(out, Change{path.Clean(f[len(f)-1]), Modified})
		}
	}
	return out, sc.Err()
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupe(in []string) []string {
	m := map[string]bool{}
	for _, s := range in {
		m[s] = true
	}
	return sortedKeys(m)
}
