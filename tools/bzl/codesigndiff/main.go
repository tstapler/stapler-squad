// Command codesigndiff prints the designated requirement of the make-built and the Bazel-built
// signed binaries side by side and the TCC guidance for the owner's device check
// (docs/how-to/macos-codesigning.md). It exits 1 when the two requirements differ.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/codesign"
)

const tccGuidance = `TCC check (owner, on the Mac, per architecture):
  1. With the make-built binary installed and its grants in place, note them:
     sqlite3 ~/Library/Application\ Support/com.apple.TCC/TCC.db "SELECT client, auth_value FROM access WHERE client = 'com.stapler-squad';"
  2. Replace it with the signed Bazel-built binary (make BUILD_SYSTEM=bazel install-service) and relaunch.
  3. PASS only if no new permission dialog appears and the same rows remain.
  Do NOT run 'make tcc-reset' for this check; it deletes the grants being tested.`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("codesigndiff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	makeBin := fs.String("make-bin", "", "signed binary built by make (required)")
	bazelBin := fs.String("bazel-bin", "", "signed binary built by Bazel (required)")
	codesignBin := fs.String("codesign-bin", "codesign", "codesign executable")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *makeBin == "" || *bazelBin == "" {
		fmt.Fprintln(stderr, "codesigndiff: -make-bin and -bazel-bin are required")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	want, err := codesign.Designated(ctx, *codesignBin, *makeBin)
	if err != nil {
		fmt.Fprintln(stderr, "codesigndiff:", err)
		return 1
	}
	got, err := codesign.Designated(ctx, *codesignBin, *bazelBin)
	if err != nil {
		fmt.Fprintln(stderr, "codesigndiff:", err)
		return 1
	}
	fmt.Fprintf(stdout, "make : %s\nbazel: %s\n", want, got)
	status := 0
	if want == got {
		fmt.Fprintln(stdout, "designated requirements: IDENTICAL")
	} else {
		fmt.Fprintln(stdout, "designated requirements: DIFFER")
		status = 1
	}
	fmt.Fprintln(stdout, tccGuidance)
	return status
}
