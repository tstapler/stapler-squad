// Package main replays the three existing CI selection mechanisms over a sample of merged PRs and counts the
// PRs where a mechanism would run work that tools/ci/affected-targets does not select (Task 7.3c). A miss means
// affected-targets under-selects relative to that mechanism; zero misses for all three is the condition for
// keeping affected-targets instead of switching to bazel-diff.
package main

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Selection is the output of affected-targets: Bazel labels plus non-Bazel job names.
type Selection struct {
	Targets []string
	Jobs    []string
}

// ParseSelection splits affected-targets output; `job:<name>` lines are jobs, everything else is a label.
func ParseSelection(r io.Reader) (Selection, error) {
	var s Selection
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "":
		case strings.HasPrefix(l, "job:"):
			s.Jobs = append(s.Jobs, strings.TrimPrefix(l, "job:"))
		default:
			s.Targets = append(s.Targets, l)
		}
	}
	return s, sc.Err()
}

func (s Selection) hasJob(names ...string) bool {
	for _, j := range s.Jobs {
		for _, n := range names {
			if j == n {
				return true
			}
		}
	}
	return false
}

// hasTargetIn reports whether any label is `//...` or lives under one of the package prefixes (//server/mcp).
func (s Selection) hasTargetIn(prefixes ...string) bool {
	for _, t := range s.Targets {
		if t == "//..." {
			return true
		}
		for _, p := range prefixes {
			if strings.HasPrefix(t, p+":") || strings.HasPrefix(t, p+"/") {
				return true
			}
		}
	}
	return false
}

// Mechanism names, as printed in the report keys.
const (
	Regex  = "regex"
	Script = "script"
	Paths  = "paths"
)

// buildJobs are the affected-targets jobs that stand for what build.yml's relevant=true runs besides Bazel targets.
var buildJobs = []string{"go-test", "web-build", "e2e"}

// Fired is what each mechanism would do for one PR.
type Fired struct{ Regex, Script, Paths bool }

// Misses returns the mechanisms that would run work for the PR that the selection does not cover.
//
//	regex   build.yml `relevant=true` runs the Go/web pipeline: covered by any Bazel target or a build/test/e2e job.
//	script  detect-feature-changes.sh true records features: covered by the registry-freshness or e2e job, or //....
//	paths   mcp-integration.yml paths match runs the MCP suites: covered by a target under //server/mcp or
//	        //server/services (or //...), or by the mcp-integration job.
func Misses(f Fired, sel Selection) []string {
	var out []string
	if f.Regex && len(sel.Targets) == 0 && !sel.hasJob(buildJobs...) {
		out = append(out, Regex)
	}
	if f.Script && !sel.hasJob("registry-freshness", "e2e") && !sel.hasTargetIn() {
		out = append(out, Script)
	}
	if f.Paths && !sel.hasJob("mcp-integration") && !sel.hasTargetIn("//server/mcp", "//server/services") {
		out = append(out, Paths)
	}
	return out
}

// RegexFromWorkflow extracts the pattern of the `grep -qE '<pattern>'` change detector in build.yml.
func RegexFromWorkflow(src string) (*regexp.Regexp, error) {
	const marker = "grep -qE '"
	i := strings.Index(src, marker)
	if i < 0 {
		return nil, fmt.Errorf("no `%s...'` detector found", marker)
	}
	rest := src[i+len(marker):]
	j := strings.Index(rest, "'")
	if j < 0 {
		return nil, fmt.Errorf("unterminated detector pattern")
	}
	return regexp.Compile(rest[:j])
}

// PathsFromWorkflow returns on.pull_request.paths of a workflow file.
func PathsFromWorkflow(src []byte) ([]string, error) {
	var wf struct {
		On struct {
			PullRequest struct {
				Paths []string `yaml:"paths"`
			} `yaml:"pull_request"`
		} `yaml:"on"`
	}
	if err := yaml.Unmarshal(src, &wf); err != nil {
		return nil, err
	}
	if len(wf.On.PullRequest.Paths) == 0 {
		return nil, fmt.Errorf("workflow has no on.pull_request.paths")
	}
	return wf.On.PullRequest.Paths, nil
}

// PathsMatch applies GitHub `paths:` semantics: a PR matches when any changed file matches any pattern.
// `**` spans directories, `*` and `?` stay inside one path segment.
func PathsMatch(patterns, files []string) bool {
	for _, p := range patterns {
		ps := strings.Split(p, "/")
		for _, f := range files {
			if segMatch(ps, strings.Split(f, "/")) {
				return true
			}
		}
	}
	return false
}

func segMatch(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}
	if pat[0] == "**" {
		for skip := 0; skip <= len(name); skip++ {
			if segMatch(pat[1:], name[skip:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
		return false
	}
	return segMatch(pat[1:], name[1:])
}

// PR is one replayed pull request.
type PR struct {
	Number int
	Files  []string
}

// Replayer supplies the two mechanisms that need a process: affected-targets and the feature-detection script.
type Replayer struct {
	Regex    *regexp.Regexp
	Patterns []string
	Affected func(files []string) (Selection, error)
	Script   func(files []string) (bool, error)
}

// Row is one PR's verdict.
type Row struct {
	PR       PR
	Fired    Fired
	Selected Selection
	Missed   []string
}

// Summary is the full replay.
type Summary struct {
	Rows   []Row
	Misses map[string]int
	// Extra counts PRs where the selection covers work the mechanism would not have run (informational).
	Extra map[string]int
}

// Replay runs every PR through the mechanisms and the selection.
func (rp Replayer) Replay(prs []PR) (Summary, error) {
	sum := Summary{Misses: map[string]int{Regex: 0, Script: 0, Paths: 0}, Extra: map[string]int{Regex: 0, Script: 0, Paths: 0}}
	for _, pr := range prs {
		sel, err := rp.Affected(pr.Files)
		if err != nil {
			return sum, fmt.Errorf("PR #%d: affected-targets: %w", pr.Number, err)
		}
		script, err := rp.Script(pr.Files)
		if err != nil {
			return sum, fmt.Errorf("PR #%d: feature script: %w", pr.Number, err)
		}
		fired := Fired{Regex: anyLineMatches(rp.Regex, pr.Files), Script: script, Paths: PathsMatch(rp.Patterns, pr.Files)}
		row := Row{PR: pr, Fired: fired, Selected: sel, Missed: Misses(fired, sel)}
		for _, m := range row.Missed {
			sum.Misses[m]++
		}
		if !fired.Regex && (len(sel.Targets) > 0 || sel.hasJob(buildJobs...)) {
			sum.Extra[Regex]++
		}
		if !fired.Script && sel.hasJob("registry-freshness", "e2e") {
			sum.Extra[Script]++
		}
		if !fired.Paths && (sel.hasJob("mcp-integration") || sel.hasTargetIn("//server/mcp", "//server/services")) {
			sum.Extra[Paths]++
		}
		sum.Rows = append(sum.Rows, row)
	}
	return sum, nil
}

func anyLineMatches(re *regexp.Regexp, files []string) bool {
	for _, f := range files {
		if re.MatchString(f) {
			return true
		}
	}
	return false
}

// Write prints the report: one `misses_<mechanism>: N` key per mechanism, then the missed PRs.
func (s Summary) Write(w io.Writer) {
	fmt.Fprintf(w, "prs: %d\n", len(s.Rows))
	for _, m := range []string{Regex, Script, Paths} {
		fmt.Fprintf(w, "misses_%s: %d\n", m, s.Misses[m])
	}
	for _, m := range []string{Regex, Script, Paths} {
		fmt.Fprintf(w, "extra_%s: %d\n", m, s.Extra[m])
	}
	for _, r := range s.Rows {
		for _, m := range r.Missed {
			fmt.Fprintf(w, "MISS %s PR #%d files=%s targets=%d jobs=%s\n", m, r.PR.Number, strings.Join(r.PR.Files, ","), len(r.Selected.Targets), strings.Join(r.Selected.Jobs, ","))
		}
	}
}
