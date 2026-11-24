// Package nosessiondrivertest defines a go/analysis pass that rejects a raw
// driver.StartSessionDriver call in *_test.go files outside package driver.
//
// A driver left running after its test polls every 2s and can write config.json
// into a later test's STAPLER_SQUAD_TEST_DIR. driver.StartSessionDriverForTest
// registers the stop+join cleanup at start time so it cannot be forgotten.
// Exempt: package driver's own tests (they exercise the start/stop semantics
// directly) and sites annotated //nolint:nosessiondrivertest <reason>.
package nosessiondrivertest

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the nosessiondrivertest check.
var Analyzer = &analysis.Analyzer{
	Name: "nosessiondrivertest",
	Doc:  "rejects driver.StartSessionDriver in _test.go files outside package driver; use driver.StartSessionDriverForTest, or add //nolint:nosessiondrivertest <reason>",
	Run:  run,
}

const directive = "nosessiondrivertest"

func run(pass *analysis.Pass) (interface{}, error) {
	if isDriverPackage(pass.Pkg.Path()) {
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
	nolints := nolintcomment.NewIndex(pass.Fset, f)
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "StartSessionDriver" || !isDriverStart(pass, id) {
			return true
		}
		switch nolints.Lookup(id.Pos(), directive) {
		case nolintcomment.OK:
		case nolintcomment.Missing:
			pass.Reportf(id.Pos(), "//nolint:%s requires a reason", directive)
		default:
			pass.Reportf(id.Pos(), "raw driver.StartSessionDriver in a test leaks the driver goroutine if cleanup is forgotten: use driver.StartSessionDriverForTest(t, inst, path), or add //nolint:%s <reason>", directive)
		}
		return true
	})
}

func isDriverStart(pass *analysis.Pass, id *ast.Ident) bool {
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && isDriverPackage(fn.Pkg().Path()) && fn.Type().(*types.Signature).Recv() == nil
}

// driverPkgPath is this module's session driver package; a var only so analysistest
// can point it at a testdata stub.
var driverPkgPath = "github.com/tstapler/stapler-squad/session/driver"

func isDriverPackage(p string) bool {
	return strings.TrimSuffix(p, "_test") == driverPkgPath
}
