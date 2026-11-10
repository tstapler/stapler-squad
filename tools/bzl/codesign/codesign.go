// Package codesign reads macOS designated requirements for the install-service and
// Bazel-vs-make codesign checks. It shells out to the codesign binary, so it runs on macOS only
// outside tests (tests pass a fake codesign).
package codesign

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	drPrefix   = "designated =>"
	beginMark  = "<!-- designated-requirement:begin -->"
	endMark    = "<!-- designated-requirement:end -->"
	noRecorded = "no designated requirement recorded between the designated-requirement markers"
)

// Designated returns the designated requirement line (starting "designated =>") that
// `codesign -d -r- <path>` prints for path.
func Designated(ctx context.Context, codesignBin, path string) (string, error) {
	out, err := exec.CommandContext(ctx, codesignBin, "-d", "-r-", path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s -d -r- %s: %w: %s", codesignBin, path, err, strings.TrimSpace(string(out)))
	}
	if dr, ok := findDR(string(out)); ok {
		return dr, nil
	}
	return "", fmt.Errorf("%s -d -r- %s printed no %q line", codesignBin, path, drPrefix)
}

// Recorded returns the designated requirement the owner recorded in doc between the
// designated-requirement HTML comment markers.
func Recorded(doc string) (string, error) {
	_, rest, ok := strings.Cut(doc, beginMark)
	if !ok {
		return "", fmt.Errorf("%s (begin marker missing)", noRecorded)
	}
	region, _, ok := strings.Cut(rest, endMark)
	if !ok {
		return "", fmt.Errorf("%s (end marker missing)", noRecorded)
	}
	if dr, ok := findDR(region); ok {
		return dr, nil
	}
	return "", fmt.Errorf("%s", noRecorded)
}

func findDR(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, drPrefix) {
			return line, true
		}
	}
	return "", false
}
