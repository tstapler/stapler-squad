// Package entfullscan defines a go/analysis pass that detects ent query builder
// call chains ending in .All(ctx) with no .Where(...) filter anywhere in the
// same function — a full-table scan.
//
// Background: session/storage.go's FindInstanceDataByID used to call
// ListInstanceData(), an unfiltered ent query (EntRepository.ListWithOptions),
// and then linear-scan the result for one ID match. It was called once per
// session inside BacklogLifecycleListener.reconcileTerminalItemSessions, a
// 60-second-cadence reconciliation loop. A CPU profile on 2026-09-02 found
// this consuming ~25% of the live process's CPU. Fixed by adding
// EntRepository.FindByIDWithOptions, an indexed WHERE query (see its doc
// comment in session/ent_repository.go).
//
// This analyzer targets the other half of that bug class: a *new* raw ent
// query introduced without a WHERE filter should require the same explicit
// "yes, this really needs every row" sign-off that review should have caught
// here. It does not (and structurally cannot) catch the original bug's exact
// shape — calling an existing, already-reviewed full-scan helper function from
// a single-lookup call site — that is a call-graph-shaped problem, not a
// syntactic one; see tstapler/kibitzer#30 for a proposed check aimed at that
// half instead.
//
// Detection: a call `X.All(ctx)` is flagged when X's static type is an ent
// generated query builder (a named type whose name ends in "Query", matching
// ent's generation convention for every entity — *ent.SessionQuery,
// *ent.WorkflowQuery, etc.) AND no call to a method named "Where" appears
// anywhere in X's expression subtree AND no reassignment to X's root
// identifier anywhere else in the enclosing function's body contains a
// "Where" call either (this second check is what lets a query built across
// multiple statements, e.g. `q := repo.Query(); if cond { q = q.Where(...) };
// q.All(ctx)`, pass without a nolint — the Where clause exists, just not in
// the same expression as the .All(ctx) call).
//
// Suppress a genuine full scan with a //nolint:entfullscan comment (same line
// or the line above) explaining why every row is needed — see this package's
// analyzer_test.go / testdata for the exact pattern.
package entfullscan

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the entfullscan check.
var Analyzer = &analysis.Analyzer{
	Name:     "entfullscan",
	Doc:      "detects ent query .All(ctx) calls with no .Where(...) filter anywhere in the enclosing function — a full-table scan that should be filtered, replaced with an indexed lookup, or explicitly allowlisted with //nolint:entfullscan and a reason",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// generatedPackagePathMarkers are package-path substrings identifying ent's
// own generated code, which implements .All(ctx) internally regardless of
// whether a caller's query had a WHERE clause — not a real signal here.
// Mirrors .golangci.yml's `path: "session/ent/"` exclusion for other linters.
var generatedPackagePathMarkers = []string{
	"/session/ent",
}

func run(pass *analysis.Pass) (interface{}, error) {
	pkgPath := pass.Pkg.Path()
	for _, marker := range generatedPackagePathMarkers {
		if strings.Contains(pkgPath, marker) {
			return nil, nil
		}
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "All" {
			return true
		}
		if !isQueryBuilderType(pass, sel.X) {
			return true
		}
		if subtreeHasWhere(sel.X) {
			return true
		}
		if identReassignedWithWhere(pass, rootIdent(sel.X), enclosingFuncBody(stack)) {
			return true
		}
		if nolintcomment.Contains(pass, call.Pos(), "entfullscan") {
			return true
		}
		pass.Reportf(call.Pos(),
			"ent query .All(ctx) with no .Where(...) filter — full table scan; add a filter, use an indexed lookup (see EntRepository.FindByIDWithOptions for the pattern), or add //nolint:entfullscan with a reason if every row is genuinely needed")
		return true
	})

	return nil, nil
}

// isQueryBuilderType reports whether expr's static type looks like an ent
// generated query builder: a named type (optionally behind a pointer) whose
// name ends in "Query" — the convention ent's codegen uses for every entity
// (SessionQuery, WorkflowQuery, BacklogItemQuery, ...). Matching on the name
// suffix rather than a specific import path keeps this analyzer working
// against the generated session/ent package without importing it, and
// naturally covers any future entity ent generates a query type for.
func isQueryBuilderType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	return strings.HasSuffix(named.Obj().Name(), "Query")
}

// subtreeHasWhere reports whether expr's AST subtree contains a call to a
// method named "Where" anywhere — covering both a fluent chain
// (q.Where(...).Order(...)) and a Where call nested inside an argument
// (applyLoadOptions(q.Where(...), opts)).
func subtreeHasWhere(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Where" {
			found = true
			return false
		}
		return true
	})
	return found
}

// rootIdent returns the identifier at the root of a fluent method-call chain
// or plain selector/call expression — e.g. `query` for `query.All(ctx)`,
// `q` for `q.WithSource().All(ctx)`. Returns nil when the root isn't a simple
// identifier (e.g. `applyLoadOptions(...)`, whose Fun is itself an identifier
// naming a function, not a chain rooted in a variable).
func rootIdent(expr ast.Expr) *ast.Ident {
	switch x := expr.(type) {
	case *ast.Ident:
		return x
	case *ast.SelectorExpr:
		return rootIdent(x.X)
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			return rootIdent(sel.X)
		}
		return nil
	}
	return nil
}

// enclosingFuncBody walks the ancestor stack (as provided by
// inspector.WithStack) to find the nearest enclosing function's body.
func enclosingFuncBody(stack []ast.Node) *ast.BlockStmt {
	for i := len(stack) - 1; i >= 0; i-- {
		switch f := stack[i].(type) {
		case *ast.FuncDecl:
			return f.Body
		case *ast.FuncLit:
			return f.Body
		}
	}
	return nil
}

// identReassignedWithWhere reports whether some assignment to ident's
// underlying object elsewhere in body has a right-hand side whose subtree
// contains a Where call — i.e. the query variable was filtered in a separate
// statement from the one containing .All(ctx).
func identReassignedWithWhere(pass *analysis.Pass, ident *ast.Ident, body *ast.BlockStmt) bool {
	if ident == nil || body == nil {
		return false
	}
	targetObj := pass.TypesInfo.ObjectOf(ident)
	if targetObj == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			lhsIdent, ok := lhs.(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(lhsIdent) != targetObj {
				continue
			}
			if i < len(assign.Rhs) && subtreeHasWhere(assign.Rhs[i]) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
