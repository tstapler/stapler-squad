package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Granularity is how finely a spec's observed Go execution is compared with a diff.
type Granularity string

// The three granularities S14 compares: the Go package (ADR-009's slice), the executed file, the executed lines.
const (
	GranPackage Granularity = "package"
	GranFile    Granularity = "file"
	GranLine    Granularity = "line"
	// GranLineStrict ignores hunks outside every statement block (declarations, comments): an optimistic bound,
	// not a sound selector, reported only to show how narrow the executed-line data could get.
	GranLineStrict Granularity = "line-strict"
)

// Line is an inclusive 1-based line range.
type Line [2]int

// FineCapture is one spec's e2ecapture output (the fields this tool reads).
type FineCapture struct {
	Spec           string            `json:"spec"`
	GoPkgs         []string          `json:"go_pkgs"`
	GoFiles        []string          `json:"go_files"`
	GoLines        map[string][]Line `json:"go_lines"`
	GoAll          map[string][]Line `json:"go_all"`
	WebFiles       []string          `json:"web_files"`
	Excluded       string            `json:"excluded"`
	PlaywrightExit int               `json:"playwright_exit"`
}

type fineSpec struct {
	spec                string
	pkgs, goFiles, webs map[string]bool
	lines               map[string][]Line
	alwaysRuns          bool
}

// FineSelector selects specs for a commit from per-spec observed Go files/lines and web files.
type FineSelector struct {
	specs   []fineSpec
	allBlk  map[string][]Line
	globals []string
	groups  []Group
	gran    Granularity
	hunks   func(sha string, files []string) map[string][]Line
}

func setOf(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// loadFineCaptures reads every *.json under dir; specsOnDisk not captured are always-run (counted as selected).
func loadFineCaptures(dir string) ([]FineCapture, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []FineCapture
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c FineCapture
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func newFineSelector(caps []FineCapture, specsOnDisk, globals []string, gran Granularity, hunks func(string, []string) map[string][]Line) *FineSelector {
	s := &FineSelector{globals: globals, gran: gran, hunks: hunks, allBlk: map[string][]Line{}}
	by := map[string]FineCapture{}
	for _, c := range caps {
		by[c.Spec] = c
		for f, ls := range c.GoAll {
			if _, ok := s.allBlk[f]; !ok {
				s.allBlk[f] = ls
			}
		}
	}
	for _, sp := range specsOnDisk {
		c, ok := by[sp]
		s.specs = append(s.specs, fineSpec{
			spec: sp, pkgs: setOf(append(append([]string{}, c.GoPkgs...), dirsOf(c.GoFiles)...)), goFiles: setOf(c.GoFiles), webs: setOf(c.WebFiles), lines: c.GoLines,
			alwaysRuns: !ok || c.Excluded != "" || len(c.GoFiles) == 0,
		})
	}
	s.groups = groupSpecs(specsOnDisk)
	return s
}

// dirsOf lists the import paths of the directories of executed files. `covdata percent` rounds a package with
// a handful of executed statements to 0.0%, so go_pkgs alone under-reports small packages (found by the S14
// mutation backtest: config/background_models.go ran but package config was absent).
func dirsOf(files []string) []string {
	var out []string
	for _, f := range files {
		out = append(out, pkgOf(f))
	}
	return out
}

func overlaps(a, b Line) bool { return a[0] <= b[1] && b[0] <= a[1] }

func anyOverlap(h Line, ranges []Line) bool {
	i := sort.Search(len(ranges), func(i int) bool { return ranges[i][1] >= h[0] })
	return i < len(ranges) && overlaps(h, ranges[i])
}

// inBlock reports whether the whole hunk lies inside instrumented statement blocks.
func inBlock(h Line, blocks []Line) bool {
	for l := h[0]; l <= h[1]; l++ {
		if !anyOverlap(Line{l, l}, blocks) {
			return false
		}
	}
	return true
}

// lineHit: executed-line overlap, or (conservative) any hunk line outside every statement block (declarations,
// imports, comments), which the block data cannot rule out.
func (s *FineSelector) lineHit(sp *fineSpec, file string, hs []Line) bool {
	if !sp.goFiles[file] {
		return false
	}
	for _, h := range hs {
		if anyOverlap(h, sp.lines[file]) {
			return true
		}
		if blocks, ok := s.allBlk[file]; s.gran == GranLine && (!ok || !inBlock(h, blocks)) {
			return true
		}
	}
	return false
}

// Select returns the per-spec selection for one commit.
func (s *FineSelector) Select(c Commit) Selection {
	sel := Selection{Total: len(s.specs)}
	var goFiles []string
	for _, f := range c.Files {
		if matchesAny(s.globals, f) {
			sel.All = true
		}
		if strings.HasSuffix(f, ".go") && isProdGoSurface(f) {
			goFiles = append(goFiles, f)
		}
	}
	var hunks map[string][]Line
	if (s.gran == GranLine || s.gran == GranLineStrict) && len(goFiles) > 0 {
		hunks = s.hunks(c.SHA, goFiles)
	}
	hit := map[string]bool{}
	for i := range s.specs {
		sp := &s.specs[i]
		if sp.alwaysRuns {
			hit[sp.spec] = true
			continue
		}
		for _, f := range c.Files {
			switch {
			case f == sp.spec, sp.webs[f]:
				hit[sp.spec] = true
			case strings.HasSuffix(f, ".go") && isProdGoSurface(f):
				switch s.gran {
				case GranPackage:
					hit[sp.spec] = hit[sp.spec] || sp.pkgs[pkgOf(f)]
				case GranFile:
					hit[sp.spec] = hit[sp.spec] || sp.goFiles[f]
				case GranLine, GranLineStrict:
					hs, ok := hunks[f]
					if !ok { // no diff text (rename, binary): fall back to the file
						hit[sp.spec] = hit[sp.spec] || sp.goFiles[f]
					} else {
						hit[sp.spec] = hit[sp.spec] || s.lineHit(sp, f, hs)
					}
				}
			case isProdGoSurface(f) && !strings.HasPrefix(f, "web-app/") && sp.pkgs[pkgOf(f)]:
				hit[sp.spec] = true // go:embed assets and cgo sources in an observed package directory
			}
		}
	}
	for _, g := range s.groups {
		for _, sp := range g.Specs {
			if hit[sp] || sel.All {
				sel.Specs = append(sel.Specs, g.Specs...)
				break
			}
		}
	}
	sel.Unmanaged = nil
	perSpec := 0
	for _, h := range hit {
		if h {
			perSpec++
		}
	}
	sel.PerSpec = perSpec
	for sp, h := range hit {
		if h {
			sel.Hit = append(sel.Hit, sp)
		}
	}
	sort.Strings(sel.Hit)
	return sel
}

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// parseHunks reads `git diff -U0` output into new-side line ranges per file (a pure deletion maps to its line).
func parseHunks(diff string) map[string][]Line {
	out := map[string][]Line{}
	cur := ""
	for _, l := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "+++ b/"):
			cur = strings.TrimPrefix(l, "+++ b/")
			if _, seen := out[cur]; !seen {
				out[cur] = nil // a file with no hunks still maps to an entry
			}
		case strings.HasPrefix(l, "+++ /dev/null"):
			cur = ""
		case strings.HasPrefix(l, "@@") && cur != "":
			m := hunkHeader.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			start, _ := strconv.Atoi(m[1])
			n := 1
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
			}
			if n == 0 {
				n = 1
			}
			out[cur] = append(out[cur], Line{start, start + n - 1})
		}
	}
	return out
}

func gitHunks(repo string) func(string, []string) map[string][]Line {
	return func(sha string, files []string) map[string][]Line {
		args := append([]string{"diff", "-U0", "--no-color", sha + "^1", sha, "--"}, files...)
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		var buf bytes.Buffer
		cmd.Stdout = &buf
		if cmd.Run() != nil {
			return nil
		}
		return parseHunks(buf.String())
	}
}

// FineStats is the Story 0.15 distribution for one granularity, per spec and per Bazel group.
type FineStats struct {
	Granularity      Granularity `json:"granularity"`
	Commits          int         `json:"commits"`
	Relevant         int         `json:"relevant"`
	ShareMedian      float64     `json:"share_median_spec"`
	ShareP90         float64     `json:"share_p90_spec"`
	GroupShareMedian float64     `json:"share_median_group"`
	GroupShareP90    float64     `json:"share_p90_group"`
	AllFallbackRate  float64     `json:"all_fallback_rate"`
	AlwaysRunSpecs   int         `json:"always_run_specs"`
	ShareMean        float64     `json:"share_mean_spec"`
	ShareMin         float64     `json:"share_min_spec"`
	CommitsBelowHalf int         `json:"commits_share_le_0_5"`
	Specs            int         `json:"specs"`
}

func replayFine(commits []Commit, sel *FineSelector) FineStats {
	var perSpec, perGroup []float64
	all, rel := 0, 0
	for _, c := range commits {
		if !relevant(c.Files) {
			continue
		}
		rel++
		s := sel.Select(c)
		if s.All {
			all++
			perSpec, perGroup = append(perSpec, 1), append(perGroup, 1)
			continue
		}
		perSpec = append(perSpec, float64(s.PerSpec)/float64(s.Total))
		perGroup = append(perGroup, s.Share())
	}
	sort.Float64s(perSpec)
	sort.Float64s(perGroup)
	st := FineStats{Granularity: sel.gran, Commits: len(commits), Relevant: rel, Specs: len(sel.specs),
		ShareMedian: median(perSpec), ShareP90: percentile(perSpec, 0.90),
		GroupShareMedian: median(perGroup), GroupShareP90: percentile(perGroup, 0.90)}
	if rel > 0 {
		st.AllFallbackRate = float64(all) / float64(rel)
		sum := 0.0
		for _, v := range perSpec {
			sum += v
			if v <= 0.5 {
				st.CommitsBelowHalf++
			}
		}
		st.ShareMean, st.ShareMin = sum/float64(rel), perSpec[0]
	}
	for _, sp := range sel.specs {
		if sp.alwaysRuns {
			st.AlwaysRunSpecs++
		}
	}
	return st
}
