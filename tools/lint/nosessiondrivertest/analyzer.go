// Package nosessiondrivertest defines a go/analysis pass that rejects a raw
// session.StartSessionDriver call in *_test.go files outside package session.
//
// A driver left running after its test polls every 2s and can write config.json
// into a later test's STAPLER_SQUAD_TEST_DIR. session.StartSessionDriverForTest
// registers the stop+join cleanup at start time so it cannot be forgotten.
// Exempt: package session's own tests (they exercise the start/stop semantics
// directly) and sites annotated //nolint:nosessiondrivertest <reason>.
package nosessiondrivertest

import (
	"go/ast"
	"go/types"
	"path"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the exported analysis.Analyzer for the nosessiondrivertest check.
var Analyzer = &analysis.Analyzer{
	Name: "nosessiondrivertest",
	Doc:  "rejects session.StartSessionDriver in _test.go files outside package session; use session.StartSessionDriverForTest, or add //nolint:nosessiondrivertest <reason>",
	Run:  run,
}

const directive = "nosessiondrivertest"

func run(pass *analysis.Pass) (interface{}, error) {
	if isSessionPackage(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, f := range pass.Files {
		if !strings.HasSuffix(pass.Fset.File(f.Pos()).Name(), "_test.go") {
			continue
		}
		checkFile(pass, f)
	}
	return nil, nil
}

func checkFile(pass *analysis.Pass, f *ast.File) {
	file := pass.Fset.File(f.Pos())
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "StartSessionDriver" || !isSessionStart(pass, id) {
			return true
		}
		line := file.Line(id.Pos())
		switch nolint(pass, f, line) {
		case reasonOK:
		case reasonMissing:
			pass.Reportf(id.Pos(), "//nolint:%s requires a reason", directive)
		default:
			pass.Reportf(id.Pos(), "raw session.StartSessionDriver in a test leaks the driver goroutine if cleanup is forgotten: use session.StartSessionDriverForTest(t, inst, path), or add //nolint:%s <reason>", directive)
		}
		return true
	})
}

func isSessionStart(pass *analysis.Pass, id *ast.Ident) bool {
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && isSessionPackage(fn.Pkg().Path())
}

func isSessionPackage(p string) bool {
	return path.Base(strings.TrimSuffix(p, "_test")) == "session"
}

type nolintResult int

const (
	reasonNone nolintResult = iota
	reasonMissing
	reasonOK
)

// nolint finds //nolint:nosessiondrivertest <reason> on line, or on the line directly above.
func nolint(pass *analysis.Pass, f *ast.File, line int) nolintResult {
	file := pass.Fset.File(f.Pos())
	result := reasonNone
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			cl := file.Line(c.Pos())
			if cl != line && cl != line-1 {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, "nolint:") {
				continue
			}
			list, reason, _ := strings.Cut(strings.TrimPrefix(text, "nolint:"), " ")
			if !strings.Contains(","+list+",", ","+directive+",") {
				continue
			}
			if strings.TrimSpace(reason) != "" {
				return reasonOK
			}
			result = reasonMissing
		}
	}
	return result
}
