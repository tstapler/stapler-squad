// Command slowsuites verifies that the slow suites listed in docs/reference/bazel-slow-suites.md still run in
// their own CI jobs and have not been moved into the Bazel path (ADR-009, Task 12.7a).
//
// The inventory's "Own CI job" column holds `workflow.yml#job` (optionally `~text`, which must appear in a
// non-comment line of that workflow) or `-` when the suite has no workflow of its own.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	inventoryPath = "docs/reference/bazel-slow-suites.md"
	workflowsDir  = ".github/workflows"
	ownJobColumn  = 4
)

type suite struct {
	name, workflow, job, marker string
}

// parseInventory returns the suites that name an own CI job, and the rows whose job cell is malformed.
func parseInventory(markdown string) ([]suite, []string) {
	var suites []suite
	var bad []string
	for _, line := range strings.Split(markdown, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) <= ownJobColumn {
			continue
		}
		name, cell := strings.TrimSpace(cells[0]), strings.TrimSpace(cells[ownJobColumn])
		if name == "" || name == "Suite" || strings.HasPrefix(name, "---") || cell == "-" {
			continue
		}
		ref, marker, _ := strings.Cut(strings.Trim(cell, "`"), "~")
		wf, job, ok := strings.Cut(ref, "#")
		if !ok || wf == "" || job == "" {
			bad = append(bad, fmt.Sprintf("%s: own CI job %q is not workflow.yml#job", name, cell))
			continue
		}
		suites = append(suites, suite{name, wf, job, strings.TrimSpace(marker)})
	}
	return suites, bad
}

func codeLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			out = append(out, l)
		}
	}
	return out
}

var bazelInvocation = regexp.MustCompile(`\b(bazel|bazelisk)\b`)

// checkSuite validates one suite against the contents of its workflow file.
func checkSuite(s suite, workflow string) []string {
	var bad []string
	jobDecl := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(s.job) + `:\s*$`)
	if !jobDecl.MatchString(workflow) {
		bad = append(bad, fmt.Sprintf("%s: job %q not found in %s", s.name, s.job, s.workflow))
	}
	code := codeLines(workflow)
	if s.marker != "" && !strings.Contains(strings.Join(code, "\n"), s.marker) {
		bad = append(bad, fmt.Sprintf("%s: %s no longer contains %q", s.name, s.workflow, s.marker))
	}
	for _, l := range code {
		if bazelInvocation.MatchString(l) {
			bad = append(bad, fmt.Sprintf("%s: %s invokes Bazel (%s); slow suites must stay outside the Bazel path", s.name, s.workflow, strings.TrimSpace(l)))
			break
		}
	}
	return bad
}

// checkBazelWorkflows flags bazel*.yml files that reuse (`uses:`) or chain (`workflow_run`) a slow-suite workflow.
func checkBazelWorkflows(suites []suite, bazelWorkflows map[string]string) []string {
	var bad []string
	seen := map[string]bool{}
	for _, s := range suites {
		if seen[s.workflow] {
			continue
		}
		seen[s.workflow] = true
		for file, text := range bazelWorkflows {
			for _, l := range codeLines(text) {
				t := strings.TrimSpace(l)
				if strings.Contains(t, s.workflow) && (strings.HasPrefix(t, "uses:") || strings.HasPrefix(t, "- uses:")) {
					bad = append(bad, fmt.Sprintf("%s: %s reuses %s", s.name, file, s.workflow))
				}
			}
		}
	}
	return bad
}

func run(root string, errw io.Writer) int {
	md, err := os.ReadFile(filepath.Join(root, inventoryPath))
	if err != nil {
		fmt.Fprintln(errw, "slowsuites:", err)
		return 2
	}
	suites, bad := parseInventory(string(md))
	if len(suites) == 0 {
		bad = append(bad, "no suites with an own CI job found in "+inventoryPath)
	}
	for _, s := range suites {
		text, err := os.ReadFile(filepath.Join(root, workflowsDir, s.workflow))
		if err != nil {
			bad = append(bad, fmt.Sprintf("%s: workflow %s missing", s.name, s.workflow))
			continue
		}
		bad = append(bad, checkSuite(s, string(text))...)
	}
	bazelFiles, _ := filepath.Glob(filepath.Join(root, workflowsDir, "bazel*.yml"))
	bazelWorkflows := map[string]string{}
	for _, f := range bazelFiles {
		if b, err := os.ReadFile(f); err == nil {
			bazelWorkflows[filepath.Base(f)] = string(b)
		}
	}
	bad = append(bad, checkBazelWorkflows(suites, bazelWorkflows)...)
	for _, m := range bad {
		fmt.Fprintln(errw, "slowsuites:", m)
	}
	if len(bad) > 0 {
		return 1
	}
	return 0
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	os.Exit(run(root, os.Stderr))
}
