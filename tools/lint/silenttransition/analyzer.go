// Package silenttransition defines a go/analysis pass that detects backlog
// status-transition and session-bookkeeping writes (session.Storage's
// TransitionBacklogItemStatus and UpdateItemSessionEnded) whose error is
// checked but only logged — the write's failure is swallowed and execution
// continues as if it had succeeded, even though the caller has already
// committed real side effects (a work session was spawned, code was
// confirmed shipped to main, a review session was confirmed stalled, etc.).
//
// This is a recurring bug shape: a status-transition write fails after side
// effects have already happened, the error only reaches the log file, the
// caller/RPC still reports success, and no sweep exists to catch the
// resulting reality/status mismatch — leaving a backlog item permanently
// stuck in a way the stuck-detector doesn't catch. Five prior bugs share this
// exact shape (BUG-030, BUG-040, BUG-041, BUG-046, BUG-048;
// docs/bugs/fixed/), plus three more instances fixed alongside this
// analyzer's introduction (server/services/backlog_service_triage.go's
// spawnSessionAfterGates and TriggerReReview,
// server/services/autonomous_orchestration_service.go's
// onAutonomousDriverComplete) — see docs/tasks/backlog-feature-improvement.md's
// 2026-07-27 update.
//
// A flagged call site must do one of:
//   - route the failure through a notification within the same if-err block
//     (a call whose selector name contains "notify"/"Notify", or a direct
//     event-bus "Publish" call — the two existing conventions in this
//     codebase, see notifyReworkCapHit/notifyTriagePersistFailure and the
//     direct a.bus.Publish(...) calls in autonomous_orchestration_service.go),
//   - propagate the failure to the caller (a return statement inside the
//     if-err block — the caller/RPC gets a real error instead of a silent
//     success), or
//   - carry a //nolint:silenttransition justification comment (same line as
//     the call, or the line immediately above) explaining why this specific
//     site's failure is safe to leave log-only (e.g. it is itself a
//     best-effort compensating action inside an already-reported error path).
package silenttransition

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

// Analyzer is the exported analysis.Analyzer for the silenttransition check.
var Analyzer = &analysis.Analyzer{
	Name:     "silenttransition",
	Doc:      "detects session.Storage TransitionBacklogItemStatus/UpdateItemSessionEnded calls whose error is only logged (not surfaced via notify/publish, propagated via return, or explicitly justified with //nolint:silenttransition)",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// targetMethods are the session.Storage (package "session") methods this
// analyzer watches — writes whose sole purpose is to keep the item's status
// (or the session-bookkeeping a reconciliation sweep depends on) in sync with
// side effects that have already happened.
var targetMethods = map[string]bool{
	"TransitionBacklogItemStatus": true,
	"UpdateItemSessionEnded":      true,
}

// targetPackagePath is the package these methods must resolve to — guards
// against unrelated same-named methods (e.g. *BacklogService's own exported
// TransitionBacklogItemStatus RPC handler in server/services, which has
// nothing to do with the storage write this analyzer cares about).
const targetPackagePath = "github.com/tstapler/stapler-squad/session"

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Statement lists this analyzer must walk are not all *ast.BlockStmt:
	// switch/select case bodies (*ast.CaseClause, *ast.CommClause) hold a bare
	// []ast.Stmt directly, with no wrapping block — a target call sitting
	// straight inside a `case ...:` (as in
	// autonomous_orchestration_service.go's onAutonomousDriverComplete) would
	// otherwise never be visited.
	nodeFilter := []ast.Node{
		(*ast.BlockStmt)(nil),
		(*ast.CaseClause)(nil),
		(*ast.CommClause)(nil),
	}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch v := n.(type) {
		case *ast.BlockStmt:
			checkStmtList(pass, v.List)
		case *ast.CaseClause:
			checkStmtList(pass, v.Body)
		case *ast.CommClause:
			checkStmtList(pass, v.Body)
		}
	})

	return nil, nil
}

// checkStmtList scans one statement list (a block body, or a switch/select
// case body) for the two shapes a target call's error check can take:
//
//	if _, err := s.storage.TransitionBacklogItemStatus(...); err != nil { ... }
//	result, err := s.storage.TransitionBacklogItemStatus(...)
//	if err != nil { ... }
func checkStmtList(pass *analysis.Pass, stmts []ast.Stmt) {
	for i, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.IfStmt:
			if assign, ok := s.Init.(*ast.AssignStmt); ok {
				if callPos, names, ok := targetCallInAssign(pass, assign); ok && isErrNilCheck(s.Cond, names) {
					checkErrBody(pass, callPos, s.Body)
				}
			}
		case *ast.AssignStmt:
			callPos, names, ok := targetCallInAssign(pass, s)
			if !ok || i+1 >= len(stmts) {
				continue
			}
			nextIf, ok := stmts[i+1].(*ast.IfStmt)
			if !ok || nextIf.Init != nil {
				continue
			}
			if isErrNilCheck(nextIf.Cond, names) {
				checkErrBody(pass, callPos, nextIf.Body)
			}
		}
	}
}

// targetCallInAssign reports whether assign's right-hand side is a call to
// one of targetMethods, returning the call's position and the set of
// non-blank identifier names assign binds (candidates for the error variable
// checked by the following/enclosing if).
func targetCallInAssign(pass *analysis.Pass, assign *ast.AssignStmt) (token.Pos, map[string]bool, bool) {
	if assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN {
		return token.NoPos, nil, false
	}
	if len(assign.Rhs) != 1 {
		return token.NoPos, nil, false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok || !isTargetCall(pass, call) {
		return token.NoPos, nil, false
	}
	names := make(map[string]bool, len(assign.Lhs))
	for _, lhs := range assign.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
			names[ident.Name] = true
		}
	}
	return call.Pos(), names, true
}

// isTargetCall reports whether call resolves (via type info) to a method
// named in targetMethods on a type declared in targetPackagePath.
func isTargetCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !targetMethods[sel.Sel.Name] {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return false
	}
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == targetPackagePath
}

// isErrNilCheck reports whether cond is exactly `<name> != nil` for some name
// in names.
func isErrNilCheck(cond ast.Expr, names map[string]bool) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	xIdent, xIsIdent := bin.X.(*ast.Ident)
	yIdent, yIsIdent := bin.Y.(*ast.Ident)
	switch {
	case xIsIdent && yIsIdent && yIdent.Name == "nil":
		return names[xIdent.Name]
	case xIsIdent && yIsIdent && xIdent.Name == "nil":
		return names[yIdent.Name]
	default:
		return false
	}
}

// checkErrBody inspects an if-err body and reports a diagnostic at callPos
// unless the body surfaces the failure (notify/publish call or a return
// statement) or callPos carries a //nolint:silenttransition justification.
func checkErrBody(pass *analysis.Pass, callPos token.Pos, body *ast.BlockStmt) {
	if nolintcomment.Contains(pass, callPos, "silenttransition") {
		return
	}

	surfaced := false
	ast.Inspect(body, func(n ast.Node) bool {
		if surfaced {
			return false
		}
		switch v := n.(type) {
		case *ast.ReturnStmt:
			surfaced = true
		case *ast.CallExpr:
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
				name := sel.Sel.Name
				if name == "Publish" || strings.Contains(strings.ToLower(name), "notify") {
					surfaced = true
				}
			}
		}
		return !surfaced
	})

	if surfaced {
		return
	}

	pass.Reportf(callPos,
		"error from TransitionBacklogItemStatus/UpdateItemSessionEnded is only logged here — the write's failure is silently swallowed after side effects already happened; route it through a notify/publish call, return it to the caller, or add a //nolint:silenttransition justification")
}
