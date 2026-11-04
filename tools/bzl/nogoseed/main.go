// Command nogoseed proves nogo bites: it builds every seeded-violation target under
// tools/bzl/nogo/testdata (all manual, all expected to FAIL) and requires each analyzer to report
// a finding in its own seed. A seed that builds clean, or fails for another reason, is an error.
//
// Usage: BAZEL_REAL=$(command -v bazelisk) go run ./tools/bzl/nogoseed [-bazel tools/bazel] [-log <captured bazel output>]
// Extra bazel flags come from $BAZEL_FLAGS.
// With -log it only parses a captured `bazel build -k` output (what the unit test does).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var analyzersList = regexp.MustCompile(`(?s)\nANALYZERS = \[(.*?)\n\]`)
var quoted = regexp.MustCompile(`"([a-z0-9]+)"`)

func main() {
	bazel := flag.String("bazel", "tools/bazel", "bazel wrapper")
	logPath := flag.String("log", "", "parse this captured output instead of running bazel")
	flag.Parse()

	build, err := os.ReadFile("tools/bzl/nogo/BUILD.bazel")
	if err != nil {
		fmt.Fprintln(os.Stderr, "nogoseed:", err)
		os.Exit(2)
	}
	var names []string
	if m := analyzersList.FindSubmatch(build); m != nil {
		for _, q := range quoted.FindAllSubmatch(m[1], -1) {
			names = append(names, string(q[1]))
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "nogoseed: no ANALYZERS in tools/bzl/nogo/BUILD.bazel")
		os.Exit(2)
	}

	var out []byte
	if *logPath != "" {
		out, err = os.ReadFile(*logPath)
	} else {
		args := []string{"build", "--keep_going"}
		args = append(args, strings.Fields(os.Getenv("BAZEL_FLAGS"))...)
		for _, n := range names {
			args = append(args, "//tools/bzl/nogo/testdata:seed_"+n)
		}
		// Exit 1 is the expected build failure; anything else (analysis errors) shows up as a missing finding.
		out, err = exec.Command(*bazel, args...).CombinedOutput() //nolint:norawexec one blocking bazel build from a dev tool, nothing long-lived to reap
		if _, isExit := err.(*exec.ExitError); isExit {
			err = nil
		}
		out = withFullLog(out)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nogoseed:", err)
		os.Exit(2)
	}

	missing := missingFindings(names, string(out))
	for _, n := range names {
		status := "FAILS (finding reported)"
		if slices.Contains(missing, n) {
			status = "MISSING"
		}
		fmt.Printf("%-22s %s\n", n, status)
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "nogoseed: no nogo finding for the seed of: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}
}

var fullLog = regexp.MustCompile(`(?m)^full log: (.+)$`)

// withFullLog appends the log file the tools/bazel wrapper names when it trims a long error list.
func withFullLog(out []byte) []byte {
	if m := fullLog.FindSubmatch(out); m != nil {
		if full, err := os.ReadFile(string(m[1])); err == nil {
			return append(out, full...)
		}
	}
	return out
}

// missingFindings returns the analyzers with no diagnostic line "<seed path>:L:C: message (<analyzer>)".
func missingFindings(names []string, out string) []string {
	var missing []string
	for _, n := range names {
		re := regexp.MustCompile(`(?m)^.*testdata/seed/` + regexp.QuoteMeta(n) + `/.*\.go:\d+:\d+: .*\(` + regexp.QuoteMeta(n) + `\)\s*$`)
		if !re.MatchString(out) {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	return missing
}
