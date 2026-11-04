package main

import (
	"strconv"
	"strings"
)

const slowSuiteSeconds = 300

// checkSlowSuites reads the inventory table of docs/reference/bazel-slow-suites.md and returns the suites over
// 300 s p50 whose classification is empty or not one of rdeps, manifest, full: <reason> (Task 12.7a).
func checkSlowSuites(markdown string) []string {
	var bad []string
	for _, line := range strings.Split(markdown, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cells) < 3 || strings.HasPrefix(strings.TrimSpace(cells[0]), "---") || strings.TrimSpace(cells[0]) == "Suite" {
			continue
		}
		name, p50, class := strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2])
		secs, err := strconv.Atoi(p50)
		if err != nil || secs <= slowSuiteSeconds {
			continue
		}
		ok := class == "rdeps" || class == "manifest" || (strings.HasPrefix(class, "full:") && len(strings.TrimSpace(class[5:])) > 0)
		if !ok {
			bad = append(bad, name+": over 300 s p50 and unclassified")
		}
	}
	return bad
}
