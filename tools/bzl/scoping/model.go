package main

import (
	"sort"
	"strings"
)

const modulePath = "github.com/tstapler/stapler-squad"

// universalDirs are the packages ADR-010 assumes sit in every e2e slice (T9b).
var universalDirs = map[string]bool{
	"session": true, "server/services": true, "server": true, "config": true, ".": true,
}

// Model is the package graph with library and test edges kept apart: a Bazel test target is a leaf, so a
// test-only import must not enlarge the dependents of the tested package (ADR-010 "target model").
type Model struct {
	Lib     map[string]map[string]bool
	Test    map[string]map[string]bool
	HasTest map[string]bool
	LOC     map[string]int
	TestLOC map[string]int
}

// IsGenerated reports whether a package directory holds checked-out-never generated code.
func IsGenerated(dir string) bool {
	return strings.HasPrefix(dir, "gen/") || (strings.HasPrefix(dir, "session/ent") && dir != "session/ent/schema")
}

// RelDir converts an import path to a module-relative directory ("." for the root package).
func RelDir(importPath string) string {
	if importPath == modulePath {
		return "."
	}
	return strings.TrimPrefix(importPath, modulePath+"/")
}

func inModule(importPath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

// reverse inverts an edge map restricted to known nodes.
func reverse(edges map[string]map[string]bool) map[string][]string {
	rev := map[string][]string{}
	for p, ims := range edges {
		for i := range ims {
			if _, ok := edges[i]; ok {
				rev[i] = append(rev[i], p)
			}
		}
	}
	return rev
}

// closure returns every node reachable from seeds over rev, seeds included.
func closure(rev map[string][]string, seeds map[string]bool) map[string]bool {
	seen := map[string]bool{}
	var stack []string
	for s := range seeds {
		seen[s] = true
		stack = append(stack, s)
	}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, y := range rev[x] {
			if !seen[y] {
				seen[y] = true
				stack = append(stack, y)
			}
		}
	}
	return seen
}

// Affected returns the libraries and test targets invalidated by a change: libraries by reverse library edges, a
// test target when its own package or any library it imports is affected, plus packages whose test files changed.
func (m *Model) Affected(libs, tests map[string]bool) (affLibs, affTests map[string]bool) {
	affLibs = closure(reverse(m.Lib), libs)
	affTests = map[string]bool{}
	for p := range m.HasTest {
		hit := affLibs[p]
		for i := range m.Test[p] {
			if affLibs[i] {
				hit = true
			}
		}
		if hit {
			affTests[p] = true
		}
	}
	for p := range tests {
		if m.HasTest[p] {
			affTests[p] = true
		}
	}
	return affLibs, affTests
}

// MergedShare is the RA1 proxy: the share of packages in the reverse closure over library and test edges merged.
func (m *Model) MergedShare(changed map[string]bool) float64 {
	merged := map[string]map[string]bool{}
	for p := range m.Lib {
		merged[p] = map[string]bool{}
		for i := range m.Lib[p] {
			merged[p][i] = true
		}
		for i := range m.Test[p] {
			merged[p][i] = true
		}
	}
	seeds := map[string]bool{}
	for c := range changed {
		if _, ok := merged[c]; ok {
			seeds[c] = true
		}
	}
	return float64(len(closure(reverse(merged), seeds))) / float64(len(merged))
}

// TargetShare is the share of all Bazel targets (libraries plus test targets) a change affects.
func (m *Model) TargetShare(libs, tests map[string]bool) float64 {
	l, t := m.Affected(libs, tests)
	return float64(len(l)+len(t)) / float64(len(m.Lib)+len(m.HasTest))
}

func sumLOC(set map[string]bool, loc map[string]int) int {
	n := 0
	for p := range set {
		n += loc[p]
	}
	return n
}

func total(loc map[string]int) int {
	n := 0
	for _, v := range loc {
		n += v
	}
	return n
}

// Inversion is a package whose tests import a package that (transitively) depends on it.
type Inversion struct {
	Pkg      string `json:"pkg"`
	Imported string `json:"imported"`
}

// Inversions lists the layering inversions (ADR-010 Tier 0), sorted for stable output.
func (m *Model) Inversions() []Inversion {
	var out []Inversion
	rev := reverse(m.Lib)
	for p, ims := range m.Test {
		dependents := closure(rev, map[string]bool{p: true})
		for q := range ims {
			if q != p && dependents[q] {
				out = append(out, Inversion{Pkg: p, Imported: q})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pkg != out[j].Pkg {
			return out[i].Pkg < out[j].Pkg
		}
		return out[i].Imported < out[j].Imported
	})
	return out
}

// ParseAllowlist reads "pkg imported" lines ('#' comments and blanks ignored).
func ParseAllowlist(s string) []Inversion {
	var out []Inversion
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		if f := strings.Fields(line); len(f) == 2 {
			out = append(out, Inversion{Pkg: f[0], Imported: f[1]})
		}
	}
	return out
}

// Ratchet compares found inversions with the allowlist: new ones are violations, and allowlisted ones that no
// longer exist are stale (the list may only shrink).
func Ratchet(found, allowed []Inversion) (violations, stale []Inversion) {
	have, ok := map[Inversion]bool{}, map[Inversion]bool{}
	for _, f := range found {
		have[f] = true
	}
	for _, a := range allowed {
		ok[a] = true
		if !have[a] {
			stale = append(stale, a)
		}
	}
	for _, f := range found {
		if !ok[f] {
			violations = append(violations, f)
		}
	}
	return violations, stale
}
