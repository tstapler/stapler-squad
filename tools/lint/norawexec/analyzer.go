// Package norawexec defines a go/analysis pass that detects direct calls to
// os/exec.Command and os/exec.CommandContext outside of the approved executor
// packages.
//
// Background: exec.CommandContext does not automatically set cmd.WaitDelay.
// Without WaitDelay, when the context expires and the process is killed, if a
// grandchild process (e.g. git credential helper, shell wrapper) holds the
// stdout/stderr pipes open, cmd.Wait() blocks indefinitely. In a polling loop
// that runs every few seconds this accumulates 40+ zombie processes per 30s
// window. (Root cause of the 2026-05-05 incident: 5683 total zombies.)
//
// The fix is to use safeexec.CommandContext(), which always sets
// cmd.WaitDelay = 2 * time.Second. This analyzer enforces that rule
// structurally: every site that calls exec.Command or exec.CommandContext
// directly must either be inside an approved wrapper package or carry a
// //nolint:norawexec justification comment explaining why the raw API is
// required (e.g. long-running cmd.Start() control-mode processes).
//
// Approved (exempt) packages:
//
//	.../executor         — TimeoutExecutor wraps exec.CommandContext correctly
//	.../executor/safeexec — the thin wrapper itself
package norawexec

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the norawexec check.
var Analyzer = &analysis.Analyzer{
	Name:     "norawexec",
	Doc:      "detects direct calls to os/exec.Command or os/exec.CommandContext outside approved wrapper packages; use safeexec.CommandContext() instead to prevent zombie process accumulation",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// exemptPackageSuffixes are the package path suffixes that are allowed to
// call exec.Command / exec.CommandContext directly because they ARE the
// approved wrappers.
var exemptPackageSuffixes = []string{
	"/executor",
	"/executor/safeexec",
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Skip if this package is an approved wrapper.
	pkgPath := pass.Pkg.Path()
	for _, suffix := range exemptPackageSuffixes {
		if strings.HasSuffix(pkgPath, suffix) {
			return nil, nil
		}
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if !isRawExecCall(call, pass) {
			return
		}
		if nolintcomment.Contains(pass, call.Pos(), "norawexec") {
			return
		}
		pass.Reportf(call.Pos(),
			"direct call to os/exec.%s — use safeexec.CommandContext() to ensure WaitDelay is set and prevent zombie process accumulation; add //nolint:norawexec with a justification if a long-running cmd.Start() process genuinely requires the raw API",
			calleeName(call))
	})

	return nil, nil
}

// isRawExecCall returns true when the call resolves (via type info) to
// os/exec.Command or os/exec.CommandContext.
func isRawExecCall(call *ast.CallExpr, pass *analysis.Pass) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	if name != "Command" && name != "CommandContext" {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[sel.Sel]
	if !ok {
		return false
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	return fn.Pkg() != nil && fn.Pkg().Path() == "os/exec"
}

// calleeName returns "Command" or "CommandContext" for use in the diagnostic.
func calleeName(call *ast.CallExpr) string {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		return sel.Sel.Name
	}
	return "Command"
}

