package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

// runRefresh re-measures a shard's output directory after post-processing.
func runRefresh(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("refresh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "", "recorder output directory holding media-manifest.json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *dir == "" {
		fmt.Fprintln(stderr, "mediatool refresh: -dir is required")
		return 2
	}
	m, err := manifest.RefreshFromDisk(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "mediatool refresh:", err)
		return 2
	}
	fmt.Fprintf(stdout, "refreshed %d manifest entries in %s\n", len(m.Entries), *dir)
	return 0
}
