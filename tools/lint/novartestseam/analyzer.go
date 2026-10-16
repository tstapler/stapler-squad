// Package novartestseam defines a go/analysis pass that detects a package-level mutable var
// whose only reassignment sites live in _test.go files — a sign the value should be a
// constant, injected into whatever function needs to vary it via a parameter, rather than
// mutated as global state for the sake of a test.
//
// Background: cmd/ssq-hooks/main.go originally declared
// `var remoteClassifyBaseURL = defaultSsqHooksBaseURL` purely so a test could point
// tryRemoteClassify at an httptest.Server, by reassigning the package var and restoring it in
// a defer. That's a global mutable test seam: it silently breaks under t.Parallel() (two
// tests racing to set/restore the same var), and it hides a real dependency (the base URL) as
// ambient state instead of an explicit function argument. The actual fix was to make the
// value a const again and pass it into tryRemoteClassify as a parameter, which tests can then
// vary per-call with no shared mutable state at all. This analyzer generalizes that fix into a
// mechanical check so the pattern doesn't recur elsewhere.
//
// A var carrying its own doc comment is exempted (not just //nolint:novartestseam) — a
// repo-wide sweep at introduction time found a dozen legitimate instances of this exact shape
// (e.g. session/headless/pool.go's maxQueueWait, session/unfinished/gogit_vcs_reader.go's
// mergeBaseBFSLimit), each already carrying a "var, not a const, so tests can shrink it"
// comment and each used from many call sites deep in an algorithm, where threading a
// parameter through every one would be real, unwanted surgery. remoteClassifyBaseURL had
// neither: zero explanatory comment, and exactly one non-test use site. The doc-comment check
// tells those two situations apart without demanding a redundant machine tag on top of
// prose the original author already wrote to justify a deliberate tradeoff.
package novartestseam

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the novartestseam check.
var Analyzer = &analysis.Analyzer{
	Name:     "novartestseam",
	Doc:      "detects a package-level var whose only reassignments live in _test.go files — make it a const and inject the value as a function parameter instead of mutating global state for tests",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// varInfo records enough about a tracked package-level var to report on it later.
type varInfo struct {
	name string
	pos  token.Pos
}

func run(pass *analysis.Pass) (interface{}, error) {
	pkgVars := collectConstLikePkgVars(pass)
	if len(pkgVars) == 0 {
		return nil, nil
	}

	assignedInTest, assignedOutsideTest := collectAssignmentSites(pass, pkgVars)

	for obj, info := range pkgVars {
		if !assignedInTest[obj] || assignedOutsideTest[obj] {
			// Never reassigned in a test: not this pattern. Also reassigned outside
			// tests: a real runtime-mutable var, not a test-only seam.
			continue
		}
		if nolintcomment.Contains(pass, info.pos, "novartestseam") {
			continue
		}
		pass.Reportf(info.pos,
			"var %s is only reassigned in test files — make it a const and inject the value as a function parameter instead of overriding global state in tests; add //nolint:novartestseam with a justification if this genuinely needs to be a package var",
			info.name)
	}
	return nil, nil
}

// collectConstLikePkgVars finds every package-level `var NAME = <const-like-expr>`
// declaration (single-name, non-const, initializer that could legally be a const
// expression), keyed by its types.Object so later identifier resolution is exact, not
// name-based.
func collectConstLikePkgVars(pass *analysis.Pass) map[types.Object]varInfo {
	pkgVars := map[types.Object]varInfo{}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			collectFromValueSpecs(pass, gd, pkgVars)
		}
	}
	return pkgVars
}

func collectFromValueSpecs(pass *analysis.Pass, gd *ast.GenDecl, pkgVars map[types.Object]varInfo) {
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 || !isConstExpr(pass, vs.Values[0]) {
			continue
		}
		if gd.Doc != nil || vs.Doc != nil {
			continue // already carries its own justification — see package doc comment
		}
		ident := vs.Names[0]
		if ident.Name == "_" {
			continue
		}
		if obj := pass.TypesInfo.Defs[ident]; obj != nil {
			pkgVars[obj] = varInfo{name: ident.Name, pos: ident.Pos()}
		}
	}
}

// collectAssignmentSites finds every mutation of a tracked var — `ident = expr`, compound
// assignment (`+=`), `ident++`/`ident--`, and `&ident` (the address may be handed to a setter
// like flag.StringVar) — split by whether it lives in a _test.go file. The declaration itself
// and `:=` are not mutations.
func collectAssignmentSites(pass *analysis.Pass, tracked map[types.Object]varInfo) (inTest, outsideTest map[types.Object]bool) {
	inTest = map[types.Object]bool{}
	outsideTest = map[types.Object]bool{}

	record := func(expr ast.Expr, pos token.Pos) {
		id, ok := expr.(*ast.Ident)
		if !ok {
			return
		}
		obj := pass.TypesInfo.Uses[id]
		if _, ok := tracked[obj]; !ok {
			return
		}
		if strings.HasSuffix(pass.Fset.Position(pos).Filename, "_test.go") {
			inTest[obj] = true
		} else {
			outsideTest[obj] = true
		}
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.AssignStmt)(nil), (*ast.IncDecStmt)(nil), (*ast.UnaryExpr)(nil)}, func(n ast.Node) {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				return
			}
			for _, lhs := range n.Lhs {
				record(lhs, n.Pos())
			}
		case *ast.IncDecStmt:
			record(n.X, n.Pos())
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				record(n.X, n.Pos())
			}
		}
	})
	return inTest, outsideTest
}

// isConstExpr reports whether `const NAME = expr` would be legal, per the type checker —
// deliberately not an AST-shape guess, since e.g. otherpkg.SomeConst and otherpkg.SomeFunc are
// both just a SelectorExpr, but only one is a legal const initializer.
func isConstExpr(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	return ok && tv.Value != nil
}
