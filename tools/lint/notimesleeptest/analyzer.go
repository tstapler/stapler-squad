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
	"slices"
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

// checkFile reports on identifiers only: pass.TypesInfo.Uses holds the Sel of
// time.Sleep (direct, aliased, func value) and the bare Sleep of a dot-import alike.
func checkFile(pass *analysis.Pass, f *ast.File) {
	file := pass.Fset.File(f.Pos())
	firstNode := firstNodePosByLine(file, f)
	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != "Sleep" || !isTimeSleep(pass, id) {
			return true
		}
		switch nolintFor(f, file, firstNode, id.Pos()) {
		case reasonOK:
		case reasonMissing:
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

type nolintResult int

const (
	reasonNone nolintResult = iota // no directive
	reasonMissing
	reasonOK
)

// firstNodePosByLine maps each line to the earliest AST node start on it, so a
// trailing comment can be told apart from a standalone one.
func firstNodePosByLine(file *token.File, f *ast.File) map[int]token.Pos {
	first := map[int]token.Pos{}
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if _, isFile := n.(*ast.File); isFile {
			return true
		}
		l := file.Line(n.Pos())
		if p, ok := first[l]; !ok || n.Pos() < p {
			first[l] = n.Pos()
		}
		return true
	})
	return first
}

// nolintFor finds //nolint:<list> <reason> on pos's line, or on a standalone
// comment line directly above (a trailing comment never covers the next line).
func nolintFor(f *ast.File, file *token.File, firstNode map[int]token.Pos, pos token.Pos) nolintResult {
	line := file.Line(pos)
	result := reasonNone
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			cl := file.Line(c.Pos())
			sameLine := cl == line
			prevStandalone := cl == line-1 && !hasCodeBefore(firstNode, cl, c.Pos())
			if !sameLine && !prevStandalone {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			list, reason, _ := strings.Cut(strings.TrimPrefix(text, "nolint:"), " ")
			if !strings.HasPrefix(text, "nolint:") || !slices.Contains(strings.Split(list, ","), directive) {
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

func hasCodeBefore(firstNode map[int]token.Pos, line int, commentPos token.Pos) bool {
	p, ok := firstNode[line]
	return ok && p < commentPos
}
