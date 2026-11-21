package main

import (
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
