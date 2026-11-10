// Command citime reads `make --trace` output on stdin (it prints nothing; tee the log) and reports where the
// wall-clock time went: per make target (time between consecutive "update target" lines) and per Go package
// (gotestsum "pkgname" lines). Usage: make --trace ready > make.log 2>&1 & citime -in make.log -done make.log.done -out timing.json -summary "$GITHUB_STEP_SUMMARY"
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stderr, time.Now)) }

func run(args []string, in io.Reader, errw io.Writer, now func() time.Time) int {
	fs := flag.NewFlagSet("citime", flag.ContinueOnError)
	fs.SetOutput(errw)
	jsonPath := fs.String("out", "", "write the timing report as JSON to this file")
	summary := fs.String("summary", "", "append a Markdown table to this file (GITHUB_STEP_SUMMARY)")
	top := fs.Int("top", 25, "rows per table")
	inPath := fs.String("in", "", "follow this log file instead of reading stdin")
	done := fs.String("done", "", "with -in: the file whose existence marks the end of the log")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *inPath != "" {
		f, err := os.Open(*inPath)
		if err != nil {
			fmt.Fprintln(errw, "citime:", err)
			return 0
		}
		defer f.Close()
		in = followReader{f: f, done: *done}
	}
	rep, err := scan(in, now)
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
