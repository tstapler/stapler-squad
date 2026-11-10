// Command citime reads `make --trace` output on stdin, passes it through unchanged, and reports where the
// wall-clock time went: per make target (time between consecutive "update target" lines) and per Go package
// (gotestsum "pkgname" lines). Usage: make --trace ready 2>&1 | citime -out timing.json -summary "$GITHUB_STEP_SUMMARY"
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, time.Now)) }

func run(args []string, in io.Reader, out, errw io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("citime", flag.ContinueOnError)
	fs.SetOutput(errw)
	jsonPath := fs.String("out", "", "write the timing report as JSON to this file")
	summary := fs.String("summary", "", "append a Markdown table to this file (GITHUB_STEP_SUMMARY)")
	top := fs.Int("top", 25, "rows per table")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep, err := scan(in, out, now)
	if err != nil {
		fmt.Fprintln(errw, "citime:", err)
	}
	if *jsonPath != "" {
		if werr := writeJSON(*jsonPath, rep); werr != nil {
			fmt.Fprintln(errw, "citime:", werr)
		}
	}
	if *summary != "" {
		if werr := appendFile(*summary, rep.markdown(*top)); werr != nil {
			fmt.Fprintln(errw, "citime:", werr)
		}
	}
	return 0 // advisory: never changes the pipeline's result
}
