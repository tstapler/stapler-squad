package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const servicesPkg = "server/services"

// topLevelLines counts newlines in every non-test .go file directly in dir. It reads the directory
// instead of using go list, so build-tagged, GOOS-suffixed and cgo files count too. Generated files
// count as well: a "Code generated" header must not be a way past the cap.
func topLevelLines(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		total += bytes.Count(b, []byte("\n"))
	}
	return total, nil
}

// runServicesCap checks only T9d's server/services cap, so CI can gate on it
// without replaying history or failing on the other T9 components.
func runServicesCap(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("services-cap", flag.ContinueOnError)
	root := fs.String("root", workspace(), "repository root")
	if err := parseFlags(fs, args); err != nil {
		return flagExit(err)
	}
	loc, err := topLevelLines(filepath.Join(*root, servicesPkg))
	if err != nil {
		fmt.Fprintf(stdout, "services-cap: FAIL: %v\n", err)
		return 1
	}
	limit := DefaultT9Targets.T9dServicesLOCMax
	switch {
	case loc == 0:
		fmt.Fprintf(stdout, "services-cap: FAIL: %s has no non-test lines; was it moved?\n", servicesPkg)
		return 1
	case loc > limit:
		fmt.Fprintf(stdout, "services-cap: FAIL: %s has %d top-level non-test lines, max %d\n", servicesPkg, loc, limit)
		fmt.Fprintf(stdout, "Remedy: move a concern into a leaf package under %s/ (.claude/rules/leaf-packages.md)\n", servicesPkg)
		return 1
	}
	fmt.Fprintf(stdout, "services-cap: PASS: %s=%d max=%d\n", servicesPkg, loc, limit)
	return 0
}
