package main

import (
	"bufio"
	"io"
	"path"
	"sort"
	"strings"
)

// Change is one replayed commit or PR: the repo-relative paths it touched.
type Change struct {
	ID    string
	Paths []string
}

// ParseGitLog reads `git log --name-only --pretty=@@%H` output, newest commit first.
func ParseGitLog(r io.Reader) []Change {
	var out []Change
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r\n")
		switch {
		case strings.HasPrefix(line, "@@"):
			out = append(out, Change{ID: line[2:]})
		case line != "" && len(out) > 0:
			out[len(out)-1].Paths = append(out[len(out)-1].Paths, line)
		}
	}
	return out
}

const webEmbedDir = "server/web"

// Resolver maps a historical path to its package node in the current tree. Renames (old path to new path, from
// `git diff -M`) let a replay of old commits follow files that a refactor moved to another package.
type Resolver struct {
	Model   *Model
	Renames map[string]string
}

// Resolve returns the node and whether the path is a test file; ok is false for non-Go paths and unknown packages.
func (r Resolver) Resolve(p string) (node string, isTest, ok bool) {
	if strings.HasPrefix(p, "web-app/") {
		// the SPA is embedded by server/web, so a web-app change rebuilds that package
		if _, known := r.Model.Lib[webEmbedDir]; known {
			return webEmbedDir, false, true
		}
	}
	if !strings.HasSuffix(p, ".go") {
		return "", false, false
	}
	if n, moved := r.Renames[p]; moved {
		p = n
	}
	d := path.Dir(p)
	if _, known := r.Model.Lib[d]; !known || IsGenerated(d) {
		return "", false, false
	}
	return d, strings.HasSuffix(p, "_test.go"), true
}

// Split returns the library and test nodes a change touches.
func (r Resolver) Split(c Change) (libs, tests map[string]bool) {
	libs, tests = map[string]bool{}, map[string]bool{}
	for _, p := range c.Paths {
		if n, isTest, ok := r.Resolve(p); ok {
			if isTest {
				tests[n] = true
			} else {
				libs[n] = true
			}
		}
	}
	return libs, tests
}

// goProdNodes returns the packages whose non-test Go files a change touches (web-app changes excluded: T9b is
// about Go production commits).
func (r Resolver) goProdNodes(c Change) map[string]bool {
	out := map[string]bool{}
	for _, p := range c.Paths {
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		if n, isTest, ok := r.Resolve(p); ok && !isTest {
			out[n] = true
		}
	}
	return out
}

// Metrics are the T9 components over one change window.
type Metrics struct {
	NChanges          int
	TestLOCShare      float64
	LibLOCShare       float64
	UniversalTouch    float64
	LibLOCByChange    map[string]int
	AffectedLibsMean  float64
	AffectedTestsMean float64
}

// Replay computes mean affected test-LOC share, mean affected library-LOC share and the share of production Go
// changes that touch a universal-slice package.
func Replay(m *Model, r Resolver, changes []Change) Metrics {
	res := Metrics{LibLOCByChange: map[string]int{}}
	totLib, totTest := total(m.LOC), total(m.TestLOC)
	var sumTest, sumLib float64
	prod, hit := 0, 0
	for _, c := range changes {
		libs, tests := r.Split(c)
		if len(libs)+len(tests) == 0 {
			continue
		}
		res.NChanges++
		al, at := m.Affected(libs, tests)
		sumTest += float64(sumLOC(at, m.TestLOC)) / float64(totTest)
		loc := sumLOC(al, m.LOC)
		sumLib += float64(loc) / float64(totLib)
		res.LibLOCByChange[c.ID] = loc
		if goProd := r.goProdNodes(c); len(goProd) > 0 {
			prod++
			for n := range goProd {
				if universalDirs[n] {
					hit++
					break
				}
			}
		}
	}
	if res.NChanges > 0 {
		res.TestLOCShare = sumTest / float64(res.NChanges)
		res.LibLOCShare = sumLib / float64(res.NChanges)
	}
	if prod > 0 {
		res.UniversalTouch = float64(hit) / float64(prod)
	}
	return res
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// PRMedians returns the median target-model share and the median merged (RA1) share over the Go-touching PRs.
func PRMedians(m *Model, r Resolver, prs []Change) (target, merged float64, n int) {
	var ts, ms []float64
	for _, c := range prs {
		libs, tests := r.Split(c)
		if len(libs)+len(tests) == 0 {
			continue
		}
		ts = append(ts, m.TargetShare(libs, tests))
		all := map[string]bool{}
		for k := range libs {
			all[k] = true
		}
		for k := range tests {
			all[k] = true
		}
		ms = append(ms, m.MergedShare(all))
	}
	return median(ts), median(ms), len(ts)
}

// materialDelta is how far affected library LOC must fall, as a share of all library LOC, to count as lower: a
// handful of relocated lines is noise, not an invalidation win.
const materialDelta = 0.001

// LowerShare is the share of changes whose affected library LOC fell materially below the stored baseline.
func LowerShare(base, now map[string]int, totalLOC int) float64 {
	minDrop := int(materialDelta * float64(totalLOC))
	n, lower := 0, 0
	for id, b := range base {
		v, ok := now[id]
		if !ok {
			continue
		}
		n++
		if b-v > minDrop {
			lower++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(lower) / float64(n)
}
