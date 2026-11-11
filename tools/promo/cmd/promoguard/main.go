// Command promoguard is the one binary for the always-run PR guards.
//
//	promoguard termcheck [flags]   banned-term scan
//	promoguard mediaguard [flags]  binary media / size guard
//	promoguard flags [-repo d] [-base b]   print the rollout flags from the base blob
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tstapler/stapler-squad/tools/promo/internal/flags"
	"github.com/tstapler/stapler-squad/tools/promo/internal/mediaguard"
	"github.com/tstapler/stapler-squad/tools/promo/internal/termcheck"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: promoguard termcheck|mediaguard|flags [flags]")
		return 2
	}
	switch args[0] {
	case "termcheck":
		return termcheck.Run(args[1:], stdout, stderr)
	case "mediaguard":
		return mediaguard.Run(args[1:], stdout, stderr)
	case "flags":
		return runFlags(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "promoguard: unknown subcommand %q\n", args[0])
		return 2
	}
}

// runFlags prints the flags as JSON, read from the base blob (a PR cannot flip its own gate).
func runFlags(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("flags", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", ".", "repository root")
	base := fs.String("base", "", "base commit (default HEAD^1)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := gitbase.Open(*repo)
	if err == nil {
		var f flags.Flags
		if f, err = flags.FromBase(r, *base); err == nil {
			err = json.NewEncoder(stdout).Encode(f)
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "promoguard flags:", err)
		return 2
	}
	return 0
}
