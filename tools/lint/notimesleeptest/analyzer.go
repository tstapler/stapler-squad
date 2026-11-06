// Package notimesleeptest defines a go/analysis pass that rejects time.Sleep
// in *_test.go files (ADR-003: docs/adr/003-no-static-sleeps-in-tests.md).
//
// Static sleeps make tests slow and timing-sensitive. Use a fake clock,
// channels, or testutil.WaitForCondition / require.Eventually instead.
// Exempt: packages under tests/realtime/ (the one place wall-clock behavior is
// the thing under test) and sites annotated //nolint:notimesleeptest <reason>.
package notimesleeptest

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the notimesleeptest check.
var Analyzer = &analysis.Analyzer{
	Name: "notimesleeptest",
	Doc:  "rejects time.Sleep in _test.go files outside tests/realtime/; use a fake clock, channels, or require.Eventually, or add //nolint:notimesleeptest <reason>",
	Run:  run,
}

const directive = "notimesleeptest"

func run(pass *analysis.Pass) (interface{}, error) {
	if isRealtimePackage(pass.Pkg.Path()) {
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

// checkFile reports on identifiers only: pass.TypesInfo.Uses holds the Sel of
// time.Sleep (direct, aliased, func value) and the bare Sleep of a dot-import alike.
func checkFile(pass *analysis.Pass, f *ast.File) {
	nolints := nolintcomment.NewIndex(pass.Fset, f)
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "Sleep" || !isTimeSleep(pass, id) {
			return true
		}
		switch nolints.Lookup(id.Pos(), directive) {
		case nolintcomment.OK:
		case nolintcomment.Missing:
			pass.Reportf(id.Pos(), "//nolint:%s requires a reason, e.g. //nolint:%s waits on real subprocess exit", directive, directive)
		default:
			pass.Reportf(id.Pos(), "time.Sleep in a test (ADR-003): use a fake clock, channels, or require.Eventually; move genuinely wall-clock tests to tests/realtime/ or add //nolint:%s <reason>", directive)
		}
		return true
	})
}

func isTimeSleep(pass *analysis.Pass, id *ast.Ident) bool {
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "time" && fn.Name() == "Sleep"
}

// isRealtimePackage matches the tests/realtime package and its subpackages by
// path segment, including the external "_test" variant.
func isRealtimePackage(path string) bool {
	path = strings.TrimSuffix(path, "_test")
	return path == "tests/realtime" || strings.HasSuffix(path, "/tests/realtime") ||
		strings.HasPrefix(path, "tests/realtime/") || strings.Contains(path, "/tests/realtime/")
}
