// Command soak-check validates soak-ledger.yaml and prints the Story 7.6 soak verdict.
// Exit 0 soak met, 1 not met, 2 ledger invalid or unreadable.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tstapler/stapler-squad/tools/bzl/soak"
)

func main() {
	file := flag.String("file", "project_plans/bazel-conversion/implementation/soak-ledger.yaml", "ledger file")
	parity := flag.Bool("parity-pass", false, "the parity gate currently prints PARITY: PASS")
	validateOnly := flag.Bool("validate", false, "only validate the schema")
	flag.Parse()
	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "soak-check:", err)
		os.Exit(2)
	}
	defer f.Close()
	l, err := soak.Parse(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "soak-check:", err)
		os.Exit(2)
	}
	if errs := l.Validate(); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "soak-check:", e)
		}
		os.Exit(2)
	}
	if *validateOnly {
		fmt.Printf("soak ledger OK (%d runs)\n", len(l.Runs))
		return
	}
	rep := l.Evaluate(*parity)
	fmt.Println(rep)
	if !rep.Met {
		os.Exit(1)
	}
}
