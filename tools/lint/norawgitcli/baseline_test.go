package norawgitcli_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// baselineCount is the number of //nolint:norawgitcli sites outside the
// analyzer's own package. It is a ratchet: it may only go DOWN as callers
// migrate to session/git/backend. A new site fails the analyzer in `make
// lint-custom`; adding a nolint to dodge that raises the count and fails here.
// Lowering the count without lowering this constant fails too, so the constant
// always tracks reality.
//
// Counted by the analyzer's own definition (see analyzer.go; it also reports
// stale directives), not the Story 0.1.1 audit's textual regexes, which also
// match comments, testdata, a `kill` runner call and fixture helpers (36
// constructor + 26 runner = 62 on 2026-10-10; the analyzer's 57 excludes those).
const baselineCount = 57

const nolintDirective = "//nolint:norawgitcli"

func TestBaselineCount(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	skipDirs := map[string]bool{".git": true, "node_modules": true, ".claude": true, "web-app": true, "third_party": true, "testdata": true}
	ownPkg := filepath.Join(root, "tools", "lint", "norawgitcli")

	count := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || path == ownPkg {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Parse comments instead of grepping so prose or string mentions of the
		// directive are not counted; the analyzer itself reports stale directives.
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(c.Text, "//")), strings.TrimPrefix(nolintDirective, "//")) {
					count++
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	switch {
	case count > baselineCount:
		t.Errorf("%d %s sites, baseline is %d: do not add new raw git CLI call sites; route through session/git/backend", count, nolintDirective, baselineCount)
	case count < baselineCount:
		t.Errorf("%d %s sites, baseline is %d: lower baselineCount in baseline_test.go to %d to lock in the progress", count, nolintDirective, baselineCount, count)
	}
}
