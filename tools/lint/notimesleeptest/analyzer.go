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
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
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

func checkFile(pass *analysis.Pass, f *ast.File) {
	file := pass.Fset.File(f.Pos())
	ast.Inspect(f, func(n ast.Node) bool {
		var id *ast.Ident
		skipChildren := false
		switch x := n.(type) {
		case *ast.SelectorExpr: // time.Sleep(...) and the func value `time.Sleep`
			id, skipChildren = x.Sel, true // children would re-report Sel as a bare Ident
		case *ast.Ident: // dot-import: Sleep(...)
			id = x
		default:
			return true
		}
		if id.Name != "Sleep" || !isTimeSleep(pass, id) {
			return !skipChildren
		}
		switch reason := nolintReason(f, file, id.Pos()); reason {
		case reasonOK:
		case reasonMissing:
			pass.Reportf(id.Pos(), "//nolint:%s requires a reason, e.g. //nolint:%s waits on real subprocess exit", directive, directive)
		default:
			pass.Reportf(id.Pos(), "time.Sleep in a test (ADR-003): use a fake clock, channels, or require.Eventually; move genuinely wall-clock tests to tests/realtime/ or add //nolint:%s <reason>", directive)
		}
		return !skipChildren
	})
}

func isTimeSleep(pass *analysis.Pass, id *ast.Ident) bool {
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == "time" && fn.Name() == "Sleep"
}

func isRealtimePackage(path string) bool {
	return strings.Contains(path, "/tests/realtime") || strings.HasPrefix(path, "tests/realtime")
}

type nolintResult int

const (
	reasonNone nolintResult = iota // no directive
	reasonMissing
	reasonOK
)

// nolintReason looks for //nolint:notimesleeptest <reason> on pos's line or the one above.
func nolintReason(f *ast.File, file *token.File, pos token.Pos) nolintResult {
	line := file.Line(pos)
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if cl := file.Line(c.Pos()); cl != line && cl != line-1 {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, "nolint") {
				continue
			}
			_, after, found := strings.Cut(text, directive)
			if !found {
				continue
			}
			if strings.TrimSpace(after) == "" {
				return reasonMissing
			}
			return reasonOK
		}
	}
	return reasonNone
}
