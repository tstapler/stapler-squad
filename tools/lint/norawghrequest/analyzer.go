// Package norawghrequest defines a go/analysis pass that detects direct calls
// to net/http.NewRequest or net/http.NewRequestWithContext whose URL targets a
// GitHub host, outside github/http_client.go's own approved constructors.
//
// Background: github.newGHRequest/newGHRequestForHostWithToken are the only
// call sites meant to build native GitHub HTTP requests — GetPRInfoConditional
// (github/etag_cache.go) relies on every such request going through a path
// that can attach an If-None-Match header, so GitHub can answer with a
// zero-rate-limit-cost 304 instead of a full 200. A new call site that builds
// its request with a raw http.NewRequest(WithContext) skips that path
// entirely: it compiles, it passes tests (a 200 body is still a valid
// response), and it silently burns full rate-limit quota on every poll tick
// forever — the exact failure mode this analyzer exists to catch before merge,
// the same way norawgitopen catches a raw git.PlainOpen skipping
// EnableDotGitCommonDir.
package norawghrequest

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the norawghrequest check.
var Analyzer = &analysis.Analyzer{
	Name:     "norawghrequest",
	Doc:      "detects direct http.NewRequest/NewRequestWithContext calls to a GitHub host outside github's approved constructors; use github.newGHRequest()/newGHRequestForHostWithToken() so conditional-request (ETag) semantics are available",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// ghPackagePath is the real import path of the package that owns
// GhBaseURL/RestBaseURLForHost and the approved request constructors. It is
// deliberately NOT exempted wholesale the way norawexec exempts its wrapper
// packages: this is the exact package where a future ungated call site would
// live, so a package-wide exemption would defeat the analyzer. Only the
// specific constructors in exemptConstructorNames are exempted, by
// function-declaration containment (see isExemptConstructorCall).
const ghPackagePath = "github.com/tstapler/stapler-squad/github"

// exemptConstructorNames are the github-package functions allowed to call
// http.NewRequest/NewRequestWithContext directly because they ARE the
// approved wrapper this analyzer steers every other call site toward.
// NewConditionalRequest/NewConditionalRequestNoCache (github/http_client.go,
// Epic 2.2) are the conditional-aware successor constructors for new native
// call sites — one cache-backed, one an explicit opt-out.
var exemptConstructorNames = map[string]bool{
	"newGHRequestForHostWithToken":        true,
	"NewConditionalRequest":               true,
	"NewConditionalRequestNoCache":        true,
	"newGHGraphQLRequestForHostWithToken": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		call := n.(*ast.CallExpr)
		name, ok := rawHTTPNewRequestCallName(call, pass)
		if !ok {
			return true
		}
		urlArg, ok := urlArgFor(call, name)
		if !ok || !referencesGitHubHost(pass, urlArg, enclosingFuncBody(stack)) {
			return true
		}
		if isExemptConstructorCall(pass, stack) {
			return true
		}
		if nolintcomment.Contains(pass, call.Pos(), "norawghrequest") {
			return true
		}
		pass.Reportf(call.Pos(),
			"direct call to http.%s to a GitHub host — use github.newGHRequest()/newGHRequestForHostWithToken() so conditional-request semantics are available; add //nolint:norawghrequest with a justification if this genuinely cannot use the wrapper",
			name)
		return true
	})

	return nil, nil
}

// rawHTTPNewRequestCallName returns ("NewRequest"|"NewRequestWithContext",
// true) when call resolves (via type info) to that function in net/http.
func rawHTTPNewRequestCallName(call *ast.CallExpr, pass *analysis.Pass) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	name := sel.Sel.Name
	if name != "NewRequest" && name != "NewRequestWithContext" {
		return "", false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return "", false
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return "", false
	}
	if fn.Pkg() == nil || fn.Pkg().Path() != "net/http" {
		return "", false
	}
	return name, true
}

// urlArgFor returns the URL argument expression for a resolved
// NewRequest(method, url, body)/NewRequestWithContext(ctx, method, url, body)
// call — the URL is the second argument for NewRequest, the third for
// NewRequestWithContext.
func urlArgFor(call *ast.CallExpr, name string) (ast.Expr, bool) {
	idx := 1
	if name == "NewRequestWithContext" {
		idx = 2
	}
	if idx >= len(call.Args) {
		return nil, false
	}
	return call.Args[idx], true
}

// hostURLHelperFuncNames are github-package functions known to build a
// GitHub host URL by calling GhBaseURL/RestBaseURLForHost internally — one
// function-call deeper than a direct AST walk of the call site sees, so a
// call to one of these is treated as equivalent to calling GhBaseURL or
// RestBaseURLForHost directly.
var hostURLHelperFuncNames = map[string]bool{
	"graphQLURLForHost": true,
}

// maxIdentResolutionDepth bounds referencesGitHubHost's recursion through
// chained local-variable assignments (url := hostURL(); req := ...(url)),
// so a pathological chain can't spin the analyzer forever.
const maxIdentResolutionDepth = 5

// referencesGitHubHost reports whether expr's subtree references a GitHub
// host — directly via github.GhBaseURL/RestBaseURLForHost (type-resolved to
// ghPackagePath), via a known wrapped helper (hostURLHelperFuncNames, e.g.
// graphQLURLForHost, itself one call deeper), or indirectly through a local
// variable assigned from either of those elsewhere in scope (the enclosing
// function's body, searched via enclosingFuncBody). Call sites may be
// qualified (from another package) or unqualified (from within the github
// package itself).
func referencesGitHubHost(pass *analysis.Pass, expr ast.Expr, scope ast.Node) bool {
	return referencesGitHubHostDepth(pass, expr, scope, 0)
}

func referencesGitHubHostDepth(pass *analysis.Pass, expr ast.Expr, scope ast.Node, depth int) bool {
	if depth > maxIdentResolutionDepth {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			if isGitHubHostURLCall(pass, node) {
				found = true
				return false
			}
		case *ast.Ident:
			rhs, ok := resolveLocalIdentRHS(pass, node, scope)
			if ok && referencesGitHubHostDepth(pass, rhs, scope, depth+1) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// isGitHubHostURLCall reports whether call resolves (via type info) to
// GhBaseURL, RestBaseURLForHost, or a known hostURLHelperFuncNames entry, all
// in ghPackagePath.
func isGitHubHostURLCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	var callee *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		callee = fn
	case *ast.SelectorExpr:
		callee = fn.Sel
	}
	if callee == nil {
		return false
	}
	if callee.Name != "GhBaseURL" && callee.Name != "RestBaseURLForHost" && !hostURLHelperFuncNames[callee.Name] {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[callee]
	if !ok {
		return false
	}
	fn, ok := obj.(*types.Func)
	return ok && fn.Pkg() != nil && fn.Pkg().Path() == ghPackagePath
}

// resolveLocalIdentRHS reports whether ident is a use of a local variable
// assigned (via ":=" or "=") somewhere in scope, returning the right-hand
// side expression of its first such assignment. This lets referencesGitHubHost
// see through a pattern like `url := GhBaseURL()+"..."; http.NewRequest("GET",
// url, nil)`, where urlArg is a bare *ast.Ident with no nested CallExpr of
// its own.
func resolveLocalIdentRHS(pass *analysis.Pass, ident *ast.Ident, scope ast.Node) (ast.Expr, bool) {
	if scope == nil {
		return nil, false
	}
	obj, ok := pass.TypesInfo.Uses[ident]
	if !ok {
		return nil, false
	}
	var rhs ast.Expr
	found := false
	ast.Inspect(scope, func(n ast.Node) bool {
		if found {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			lhsIdent, ok := lhs.(*ast.Ident)
			if !ok || i >= len(assign.Rhs) {
				continue
			}
			lhsObj := pass.TypesInfo.Defs[lhsIdent]
			if lhsObj == nil {
				lhsObj = pass.TypesInfo.Uses[lhsIdent]
			}
			if lhsObj == obj {
				rhs = assign.Rhs[i]
				found = true
			}
		}
		return true
	})
	return rhs, found
}

// enclosingFuncBody walks the ancestor stack (as provided by
// inspector.WithStack) to find the nearest enclosing function body — a
// FuncDecl or FuncLit — used as the search scope for resolveLocalIdentRHS.
func enclosingFuncBody(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		switch fn := stack[i].(type) {
		case *ast.FuncDecl:
			if fn.Body != nil {
				return fn.Body
			}
		case *ast.FuncLit:
			if fn.Body != nil {
				return fn.Body
			}
		}
	}
	return nil
}

// isExemptConstructorCall reports whether the call at the top of stack sits
// inside one of exemptConstructorNames' function declarations in the github
// package itself — the same function-declaration-containment technique used
// elsewhere in this codebase (see entfullscan's enclosingFuncBody) to
// self-exempt a wrapper's own approved call from the rule it enforces on
// every other site.
func isExemptConstructorCall(pass *analysis.Pass, stack []ast.Node) bool {
	if pass.Pkg.Path() != ghPackagePath {
		return false
	}
	fn := enclosingFuncDecl(stack)
	return fn != nil && fn.Name != nil && exemptConstructorNames[fn.Name.Name]
}

// enclosingFuncDecl walks the ancestor stack (as provided by
// inspector.WithStack) to find the nearest enclosing function declaration.
func enclosingFuncDecl(stack []ast.Node) *ast.FuncDecl {
	for i := len(stack) - 1; i >= 0; i-- {
		if fd, ok := stack[i].(*ast.FuncDecl); ok {
			return fd
		}
	}
	return nil
}
