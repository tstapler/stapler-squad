// Command helper is a test binary used by executor package tests. It supports
// a set of flags that produce predictable, controllable behavior for testing
// ShortLivedCmd and ManagedProcess.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	printStr := flag.String("print", "", "Write string to stdout and exit 0")
	printStderr := flag.String("print-stderr", "", "Write string to stderr and exit 0")
	printBoth := flag.Bool("print-both", false, `Write "out" to stdout and "err" to stderr and exit 0`)
	printCwd := flag.Bool("print-cwd", false, "Print os.Getwd() to stdout and exit 0")
	printEnv := flag.String("print-env", "", "Print the value of the named environment variable and exit 0")
	printLines := flag.Int("print-lines", 0, "Print n lines (\"line 1\\n\"...) then exit 0")
	exitCode := flag.Int("exit-code", 0, "Exit with this code immediately after flags are parsed")
	sleepDur := flag.String("sleep", "", "Sleep for this duration then exit 0 (e.g. '10s')")
	trapSIGTERM := flag.Bool("trap-sigterm", false, "Ignore SIGTERM (use with --sleep to test gracePeriod)")

	flag.Parse()

	if *trapSIGTERM {
		// Install SIGTERM trap: ignore it so the process only dies on SIGKILL.
		signal.Ignore(syscall.SIGTERM)
	}

	if *printStr != "" {
		fmt.Println(*printStr)
		os.Exit(0)
	}

	if *printStderr != "" {
		fmt.Fprintln(os.Stderr, *printStderr)
		os.Exit(0)
	}

	if *printBoth {
		fmt.Fprint(os.Stdout, "out")
		fmt.Fprint(os.Stderr, "err")
		os.Exit(0)
	}

	if *printCwd {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(cwd)
		os.Exit(0)
	}

	if *printEnv != "" {
		fmt.Println(os.Getenv(*printEnv))
		os.Exit(0)
	}

	if *printLines > 0 {
		for i := 1; i <= *printLines; i++ {
			fmt.Printf("line %d\n", i)
		}
		os.Exit(0)
	}

	if *sleepDur != "" {
		d, err := time.ParseDuration(*sleepDur)
		if err != nil {
			// Try parsing as seconds integer for compatibility.
			secs, serr := strconv.ParseInt(*sleepDur, 10, 64)
			if serr != nil {
				fmt.Fprintf(os.Stderr, "invalid sleep duration: %v\n", err)
				os.Exit(1)
			}
			d = time.Duration(secs) * time.Second
		}
		time.Sleep(d)
		os.Exit(0)
	}

	os.Exit(*exitCode)
}
