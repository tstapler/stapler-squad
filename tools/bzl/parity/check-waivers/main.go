// Command check-waivers validates owner-attestations.yaml waivers and prints the report block.
// Exit 0 when every waiver is valid (or there are none), 1 when any is rejected.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/tstapler/stapler-squad/tools/bzl/parity/waivers"
)

func main() {
	file := flag.String("file", "project_plans/bazel-conversion/implementation/owner-attestations.yaml", "attestations file")
	today := flag.String("today", "", "override today's date (YYYY-MM-DD), for tests")
	flag.Parse()
	now := time.Now().UTC()
	if *today != "" {
		t, err := time.Parse("2006-01-02", *today)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check-waivers: bad --today:", err)
			os.Exit(2)
		}
		now = t
	}
	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-waivers:", err)
		os.Exit(2)
	}
	defer f.Close()
	ws, err := waivers.Parse(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-waivers:", err)
		os.Exit(2)
	}
	ok, bad := waivers.Validate(ws, now)
	fmt.Print(waivers.Block(ok))
	for _, p := range bad {
		fmt.Printf("PARITY: FAIL WAIVER %s (%s)\n", p.Row, p.Reason)
	}
	if len(bad) > 0 {
		os.Exit(1)
	}
}
