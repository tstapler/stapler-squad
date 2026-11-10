// Command e2eguard is the revision guard of ADR-009 decision 2 / Task 12.3a: the server binary reaches the
// e2e tests out of band (TEST_SERVER_BINARY), so before any e2e run it must be proven to come from HEAD.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	commitLine  = regexp.MustCompile(`commit:\s*([0-9a-f]{7,40})`)
	versionLine = regexp.MustCompile(`version\s+([0-9a-f]{7,40})(-dirty)?\b`)
)

// binaryRevision extracts the embedded git revision from `stapler-squad version` output; "" if none.
func binaryRevision(versionOutput string) string {
	if m := commitLine.FindStringSubmatch(versionOutput); m != nil {
		return m[1]
	}
	if m := versionLine.FindStringSubmatch(versionOutput); m != nil {
		return m[1]
	}
	return ""
}

// check returns an error carrying the one-line remedy when the binary is not provably built from head.
func check(versionOutput, head string) error {
	rev := binaryRevision(versionOutput)
	switch {
	case rev == "":
		return fmt.Errorf("the binary reports no embedded git revision")
	case !strings.HasPrefix(head, rev) && !strings.HasPrefix(rev, head):
		return fmt.Errorf("binary revision %s differs from HEAD %s", rev, head)
	}
	return nil
}

func main() {
	bin := os.Getenv("TEST_SERVER_BINARY")
	if len(os.Args) > 1 {
		bin = os.Args[1]
	}
	if bin == "" {
		fmt.Fprintln(os.Stderr, "e2eguard: TEST_SERVER_BINARY is unset\nRemedy: rebuild the e2e binary (bazel build //:stapler-squad) and export TEST_SERVER_BINARY")
		os.Exit(1)
	}
	head, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2eguard: git rev-parse HEAD:", err)
		os.Exit(1)
	}
	out, err := exec.Command(bin, "version").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2eguard: %s version: %v\nRemedy: rebuild the e2e binary (bazel build //:stapler-squad)\n", bin, err)
		os.Exit(1)
	}
	if err := check(string(out), strings.TrimSpace(string(head))); err != nil {
		fmt.Fprintf(os.Stderr, "e2eguard: %v\nRemedy: rebuild the e2e binary (bazel build //:stapler-squad)\n", err)
		os.Exit(1)
	}
}
