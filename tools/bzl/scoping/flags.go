package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// parseFlags parses args with the stdlib flag package, so `--flag value` and `--flag=value` behave identically.
// Unlike fs.Parse alone it rejects stray positional arguments instead of silently ignoring them, which is how
// `budget --base=origin/main` once skipped the raise check.
func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		err := fmt.Errorf("scoping %s: unexpected argument %q (flags are --name value or --name=value)", fs.Name(), fs.Arg(0))
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// flagExit maps a parseFlags error to an exit code: -h/--help prints usage and exits 0, anything else is a usage error (2).
func flagExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}
