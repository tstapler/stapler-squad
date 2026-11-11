package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
)

// runBaseline writes a grandfathered sidecar for every in-scope feature that
// lacks docs and has no sidecar yet. Existing sidecars are never rewritten, so a
// second run changes nothing.
func runBaseline(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	in, err := loadInput(*repo)
	if err != nil {
		fmt.Fprintln(stderr, "docscoverage baseline:", err)
		return 2
	}
	res := check.Assess(in)
	created := 0
	for _, e := range res.Entries {
		if _, has := in.Sidecars.ByID[e.ID]; has || e.Status != model.StatusGap {
			continue
		}
		changedFile, err := registry.WriteSidecar(*repo, model.Sidecar{ID: e.ID, Status: model.Grandfathered})
		if err != nil {
			fmt.Fprintln(stderr, "docscoverage baseline:", err)
			return 2
		}
		if changedFile {
			created++
		}
	}
	fmt.Fprintf(stdout, "docs-coverage baseline: %d in scope, %d sidecars created, %d already present or covered\n",
		res.Counts.InScope, created, res.Counts.InScope-created)
	return 0
}
