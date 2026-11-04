// Command bazel-gates runs the freshness gates of Task 7.2a. Each failure prints the remedy command, the
// same text the pre-commit hook uses, so an agent can fix drift without reading CI logs.
//
//	bazel-gates [gazelle] [mod-tidy] [gen-freshness]   (default: all three, in that order)
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Gate is one freshness check: Cmd must exit 0 when nothing drifted.
type Gate struct {
	Name   string
	Cmd    []string
	Remedy string
}

// Gates are the three gates in run order.
func Gates() []Gate {
	return []Gate{
		{"gazelle", []string{"bazel", "run", "//:gazelle", "--", "-mode=diff"}, "Run: make gazelle"},
		{"mod-tidy", []string{"sh", "-c", "bazel mod tidy --lockfile_mode=update && git diff --exit-code -- MODULE.bazel MODULE.bazel.lock"}, "Run: bazel mod tidy"},
		{"gen-freshness", []string{"make", "gen-freshness"}, "run: make sync"},
	}
}

// Runner executes a command and returns its combined output and exit code.
type Runner func(cmd []string) (out string, code int)

// Check runs the named gates and returns the failure messages (empty when all are fresh).
func Check(r Runner, gates []Gate) []string {
	var failures []string
	for _, g := range gates {
		if out, code := r(g.Cmd); code != 0 {
			failures = append(failures, fmt.Sprintf("bazel-gates: %s drift (exit %d)\n%s\n%s", g.Name, code, out, g.Remedy))
		}
	}
	return failures
}

func osRunner(cmd []string) (string, int) {
	var b bytes.Buffer
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdout, c.Stderr = &b, &b
	err := c.Run()
	if err == nil {
		return b.String(), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return b.String(), ee.ExitCode()
	}
	return b.String() + err.Error(), 127
}

func main() { os.Exit(run(os.Args[1:], osRunner, os.Stdout, os.Stderr)) }

func run(args []string, r Runner, stdout, stderr io.Writer) int {
	all := Gates()
	sel := all
	if len(args) > 0 {
		sel = nil
		for _, a := range args {
			found := false
			for _, g := range all {
				if g.Name == a {
					sel, found = append(sel, g), true
				}
			}
			if !found {
				fmt.Fprintf(stderr, "bazel-gates: unknown gate %q\n", a)
				return 2
			}
		}
	}
	failures := Check(r, sel)
	for _, f := range failures {
		fmt.Fprintln(stderr, f)
	}
	if len(failures) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "bazel-gates: %d gate(s) fresh\n", len(sel))
	return 0
}
