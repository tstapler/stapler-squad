// Package noliveinstanceraw defines a go/analysis pass that detects a raw
// nil-comparison against SessionService.FindLiveInstance's result inside one
// of a watched set of liveness/kill-decision methods (monitoredFuncNames),
// which must instead go through SessionService.findConfirmedLiveInstance.
//
// Background: FindLiveInstance answers "is this session tracked in the live
// in-memory poller's map right now" — a cheap, usually-correct, but not
// infallible proxy for "is this session's process actually alive." A session
// can transiently drop out of that map during a reconciliation hiccup while
// its real tmux process keeps running. AutoReopenAfterFailedReview's reuse
// check and spawnSessionAfterGates' step-8b guard both trusted a raw
// `FindLiveInstance(...) != nil` (via SessionService.IsSessionLive) as the
// full answer, wrongly concluded a still-running work session was dead, and
// let a duplicate session spawn into the same shared worktree the original
// was still writing to (2026-09-12 incident). findConfirmedLiveInstance is
// now the single place that answers this correctly (map fast path, falling
// back to a direct tmux/process truth check before concluding dead) — every
// other liveness-for-spawn/kill-decision call site must go through it instead
// of re-deriving the same raw check.
//
// This analyzer deliberately watches a named list of methods
// (monitoredFuncNames) rather than banning the `FindLiveInstance(...) == /
// != nil` idiom everywhere in server/services: most of FindLiveInstance's
// callers (SteerActiveSession, ArchiveSessionByUUID, SessionProgram,
// IsReadyForSteer, TimeSinceLastMeaningfulOutput, IsRetryPending)
// legitimately need "is there an operable in-memory instance to act on right
// now" — a different question findConfirmedLiveInstance's shadow-instance
// fallback cannot answer for them (a reconstructed shadow instance has no PTY
// or controller wiring). Blanket-banning the idiom would just force nolint
// spam onto those unrelated, correct call sites; a watchlist instead encodes
// "these specific liveness/kill-decision methods must never regress to a raw
// check" precisely, and grows only when a new such method is added.
package noliveinstanceraw

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the noliveinstanceraw check.
var Analyzer = &analysis.Analyzer{
	Name:     "noliveinstanceraw",
	Doc:      "detects a raw FindLiveInstance(...) nil-comparison used as a liveness decision outside SessionService.findConfirmedLiveInstance; use findConfirmedLiveInstance so a live-poller-map miss falls back to a direct tmux/process truth check before concluding a session is dead",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// servicesPackagePath is the package that owns FindLiveInstance and the
// approved findConfirmedLiveInstance wrapper. Not exempted wholesale — this
// is the exact package a future raw call site would live in.
const servicesPackagePath = "github.com/tstapler/stapler-squad/server/services"

// monitoredFuncNames are the servicesPackagePath methods this analyzer
// guards: each one previously contained (or, per the same bug class, is at
// risk of regressing to) a raw `FindLiveInstance(...) == / != nil` liveness
// decision that must instead go through findConfirmedLiveInstance. A
// FindLiveInstance nil-comparison anywhere else in the package is out of
// scope — see the package doc comment for why that is deliberate, not an
// oversight.
var monitoredFuncNames = map[string]bool{
	"IsSessionLive":     true,
	"KillTmuxPaneOnly":  true,
	"StopSessionByUUID": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	if pass.Pkg.Path() != servicesPackagePath {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.BinaryExpr)(nil),
	}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		bin := n.(*ast.BinaryExpr)
		if bin.Op != token.EQL && bin.Op != token.NEQ {
			return true
		}
		_, ok := findLiveInstanceCallOperand(bin, pass, enclosingFuncBody(stack))
		if !ok {
			return true
		}
		if !isMonitoredFuncCall(stack) {
			return true
		}
		if nolintcomment.Contains(pass, bin.Pos(), "noliveinstanceraw") {
			return true
		}
		pass.Reportf(bin.Pos(),
			"raw FindLiveInstance(...) nil-comparison used as a liveness decision — use findConfirmedLiveInstance so a live-poller-map miss falls back to a direct tmux/process truth check before concluding dead; add //nolint:noliveinstanceraw with a justification if this is genuinely not a liveness decision (e.g. steering/archiving an in-memory-only operation)")
		return true
	})

	return nil, nil
}

// findLiveInstanceCallOperand reports whether one operand of bin is a call
// resolving (via type info) to a method literally named FindLiveInstance
// declared in servicesPackagePath — directly, or through a local variable
// assigned from such a call earlier in scope (the `inst := s.FindLiveInstance(id);
// if inst == nil {...}` shape the real pre-fix bug used) — and the other
// operand is the literal nil identifier. Returns that call expression on a
// match.
func findLiveInstanceCallOperand(bin *ast.BinaryExpr, pass *analysis.Pass, scope ast.Node) (*ast.CallExpr, bool) {
	x, xOK := asFindLiveInstanceCall(bin.X, pass, scope)
	if xOK && isNilIdent(bin.Y) {
		return x, true
	}
	y, yOK := asFindLiveInstanceCall(bin.Y, pass, scope)
	if yOK && isNilIdent(bin.X) {
		return y, true
	}
	return nil, false
}

// isNilIdent reports whether expr is the predeclared "nil" identifier.
func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// asFindLiveInstanceCall reports whether expr is a call expression whose
// callee resolves (via type info) to a method named FindLiveInstance declared
// in servicesPackagePath — directly, or (when expr is a bare identifier) via
// the local variable it refers to, resolved against scope.
func asFindLiveInstanceCall(expr ast.Expr, pass *analysis.Pass, scope ast.Node) (*ast.CallExpr, bool) {
	if ident, ok := expr.(*ast.Ident); ok {
		rhs, ok := resolveLocalIdentRHS(pass, ident, scope)
		if !ok {
			return nil, false
		}
		expr = rhs
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "FindLiveInstance" {
		return nil, false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return nil, false
	}
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != servicesPackagePath {
		return nil, false
	}
	return call, true
}

// resolveLocalIdentRHS reports whether ident is a use of a local variable
// assigned (via ":=" or "=") somewhere in scope, returning the right-hand
// side expression of its first such assignment. Mirrors
// norawghrequest.resolveLocalIdentRHS.
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
// FuncDecl or FuncLit. Mirrors norawghrequest.enclosingFuncBody.
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

// isMonitoredFuncCall reports whether the innermost enclosing function
// declaration in stack is one of monitoredFuncNames.
func isMonitoredFuncCall(stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if fd, ok := stack[i].(*ast.FuncDecl); ok {
			return fd.Name != nil && monitoredFuncNames[fd.Name.Name]
		}
	}
	return false
}
