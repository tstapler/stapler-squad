// Package tmuxsocketscope defines a go/analysis pass that detects tmux
// command construction which bypasses socket resolution.
//
// Background: every raw tmux invocation must resolve its target socket
// through tmux.ResolveSocket (directly, or via prependSocket / Socket.Args /
// the mux package's prependIsolatedSocket) before reaching an exec
// constructor. Skipping that step means the call targets the real, shared
// default tmux socket unconditionally -- including inside a `go test`
// binary, where it can enumerate or kill sessions belonging to a separate,
// currently-running stapler-squad process on the same machine. That was the
// root cause of a string of production incidents (see
// session/tmux/tmux.go's ResolveSocket doc comment) closed by introducing
// the choke point this analyzer now guards structurally.
//
// This pass flags any safeexec.CommandContext / exec.CommandContext /
// exec.Command call that targets tmux.Binary() (or, from within package
// session/tmux, the unqualified Binary()) where the trailing argv was NOT
// derived from a call to ResolveSocket, prependSocket, prependIsolatedSocket,
// or a Socket.Args method call -- tracked per function via a simple,
// intra-procedural safety-propagation pass: a variable is "safe" once it is
// assigned from an expression that calls one of those functions or that
// references another already-safe variable (so `append([]string{"-L",
// socket}, args...)` is safe once `socket` was itself resolved).
//
// Known limitations (heuristic, not a full dataflow analysis):
//   - Only catches the direct-literal/direct-variable case actually seen in
//     production incidents. It does not trace the tmux binary path through
//     an intermediate variable (`bin := tmux.Binary(); exec.Command(bin, ...)`).
//   - Safety propagation is a single flat pass over the function body in
//     source order; it does not model control flow, so a reassignment inside
//     a branch that isn't actually taken at runtime is still treated as
//     making the variable safe from that point forward.
//
// Add a //nolint:tmuxsocketscope comment with justification for any call
// that genuinely doesn't need socket scoping (e.g. a targeted single-session
// operation reviewed and intentionally left unscoped).
package tmuxsocketscope

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the tmuxsocketscope check.
var Analyzer = &analysis.Analyzer{
	Name:     "tmuxsocketscope",
	Doc:      "detects tmux command construction (safeexec/exec Command/CommandContext targeting tmux.Binary()) whose args were not derived from ResolveSocket/prependSocket/prependIsolatedSocket/Socket.Args, which targets the real shared default tmux socket even inside a go test binary",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// sanctionedCallNames are the function/method names that prove the
// expression calling them incorporates socket resolution.
var sanctionedCallNames = map[string]bool{
	"ResolveSocket":         true,
	"prependSocket":         true,
	"prependIsolatedSocket": true,
	"Args":                  true, // Socket.Args
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.FuncDecl)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil {
			return
		}
		checkFunc(pass, fd.Body)
	})

	return nil, nil
}

// checkFunc walks a single function body (including nested closures) in
// source order, tracking which local variables are "safe" (their value
// incorporates socket resolution) and flagging any tmux exec-constructor
// call whose trailing argv is not safe.
func checkFunc(pass *analysis.Pass, body *ast.BlockStmt) {
	safe := map[string]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			recordAssignSafety(node, safe)
		case *ast.CallExpr:
			checkExecCall(pass, node, safe)
		}
		return true
	})
}

// recordAssignSafety updates safe for each identifier on the left-hand side
// of an assignment (":=" or "=") based on whether its right-hand side is a
// safe expression under the CURRENT safe map -- so safety propagates in
// source order (e.g. `socket := tmux.ResolveSocket(x)` then later
// `args = append([]string{"-L", socket}, args...)` marks args safe too).
func recordAssignSafety(assign *ast.AssignStmt, safe map[string]bool) {
	if len(assign.Rhs) != len(assign.Lhs) {
		// Multi-value return (e.g. `a, b := f()`) -- not the pattern this
		// analyzer targets; leave any existing safety record untouched.
		return
	}
	for i, lhs := range assign.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}
		safe[ident.Name] = isSafeExpr(assign.Rhs[i], safe)
	}
}

// isSafeExpr reports whether expr's subtree contains a call to one of
// sanctionedCallNames, a reference to an identifier already marked safe, or a
// literal "-L" flag -- code that already spells out "-L" explicitly (e.g. a
// dedicated isolated test-server helper juggling its own socket name) has
// made its scoping decision deliberately and isn't the omission pattern this
// analyzer targets.
func isSafeExpr(expr ast.Expr, safe map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		switch node := n.(type) {
		case *ast.Ident:
			if safe[node.Name] {
				found = true
				return false
			}
		case *ast.CallExpr:
			if sanctionedCallNames[callName(node)] {
				found = true
				return false
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				if v, err := strconv.Unquote(node.Value); err == nil && v == "-L" {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// callName returns the identifier or selector name a CallExpr invokes, e.g.
// "ResolveSocket" for both `ResolveSocket(x)` and `tmux.ResolveSocket(x)`.
func callName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// checkExecCall reports a diagnostic when call is a Command/CommandContext
// invocation targeting tmux.Binary() whose trailing argv is not safe.
func checkExecCall(pass *analysis.Pass, call *ast.CallExpr, safe map[string]bool) {
	if !isExecConstructorCall(call, pass) {
		return
	}

	binaryIdx := -1
	for i, arg := range call.Args {
		if isTmuxBinaryCall(arg, pass) {
			binaryIdx = i
			break
		}
	}
	if binaryIdx == -1 {
		return // not a tmux invocation
	}

	// Safety evidence only needs to appear ONCE anywhere among the trailing
	// args -- e.g. for a flat variadic call `Binary(), "-L", socket, "list-sessions"`,
	// only the "-L" and its socket value carry evidence; "list-sessions" never
	// will, by design. Requiring every arg to individually prove safety would
	// flag that entirely correct pattern.
	for i := binaryIdx + 1; i < len(call.Args); i++ {
		if isSafeExpr(call.Args[i], safe) {
			return
		}
	}

	if nolintcomment.Contains(pass, call.Pos(), "tmuxsocketscope") {
		return
	}
	pass.Reportf(call.Pos(),
		"tmux command built without routing through ResolveSocket/prependSocket/prependIsolatedSocket/Socket.Args -- this targets the real shared default tmux socket unconditionally, even inside a go test binary; add //nolint:tmuxsocketscope with justification if this call genuinely doesn't need socket scoping")
}

// isExecConstructorCall reports whether call resolves (via type info) to
// os/exec.Command, os/exec.CommandContext, or safeexec.CommandContext.
func isExecConstructorCall(call *ast.CallExpr, pass *analysis.Pass) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
		return false
	}
	fn := calleeFunc(pass, sel.Sel)
	if fn == nil || fn.Pkg() == nil {
		return false
	}
	path := fn.Pkg().Path()
	return path == "os/exec" || path == "executor/safeexec" || strings.HasSuffix(path, "/executor/safeexec")
}

// isTmuxBinaryCall reports whether expr is a call resolving (via type info)
// to session/tmux.Binary, called either qualified (tmux.Binary()) or
// unqualified (Binary(), from within package session/tmux itself).
func isTmuxBinaryCall(expr ast.Expr, pass *analysis.Pass) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	var selIdent *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		selIdent = fn
	case *ast.SelectorExpr:
		selIdent = fn.Sel
	default:
		return false
	}
	if selIdent.Name != "Binary" {
		return false
	}
	fn := calleeFunc(pass, selIdent)
	if fn == nil || fn.Pkg() == nil {
		return false
	}
	path := fn.Pkg().Path()
	return path == "session/tmux" || strings.HasSuffix(path, "/session/tmux")
}

// calleeFunc resolves an *ast.Ident naming a function/method to its
// *types.Func via the pass's type info, or nil if it doesn't resolve.
func calleeFunc(pass *analysis.Pass, ident *ast.Ident) *types.Func {
	obj, ok := pass.TypesInfo.Uses[ident]
	if !ok {
		return nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil
	}
	return fn
}
