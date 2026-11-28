package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const servicesPkg = "server/services"

// runServicesCap checks only T9d's server/services cap, so CI can gate on it
// without replaying history or failing on the other T9 components.
func runServicesCap(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("services-cap", flag.ContinueOnError)
	root := fs.String("root", workspace(), "repository root")
	if err := parseFlags(fs, args); err != nil {
		return flagExit(err)
	}
	got, err := Measure(*root, []string{servicesPkg})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	loc, limit := got[servicesPkg].LOC, DefaultT9Targets.T9dServicesLOCMax
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
