// Package sendkeysguard provides a structural AST regression guard for
// BUG-031: a shared checker, callable from any package's tests, that fails
// if a call site concatenates submitted content and an Enter/carriage-return
// keystroke into a single write instead of sending them as two separate
// writes with a settle-wait in between (session.SubmitDriverContent). A
// single-write submit lands inside Claude Code's Ink-TUI paste-detection
// window and gets folded into the pasted block instead of registering as
// submit — see session.SubmitDriverContent's doc comment for the full story.
//
// This logic originated as a session-package-only test
// (TestSessionPackage_NoDirectSendKeysPlusEnterConcatenation); it was
// factored out here so server/mcp, server/services, and session/tymux can
// each run the same guard over their own directory, after the same
// single-write pattern independently reappeared at the MCP/RPC entry points
// and in the tymux gRPC backend.
package sendkeysguard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// CheckNoSingleWriteEnterConcatenation fails t if any non-test .go file in
// dir (other than those named in exemptFiles) calls a SendKeys-style method
// with a single argument that concatenates content with an Enter keystroke —
// either `SendKeys(x + EnterKeySequence)` or `SendKeys(BuildSubmittableInput(...))`
// / `SendKeys(BuildSubmittableInputAndSubmit(...))`.
func CheckNoSingleWriteEnterConcatenation(t *testing.T, dir string, exemptFiles ...string) {
	t.Helper()
	inspectCallExprs(t, dir, exemptFiles, func(fset *token.FileSet, call *ast.CallExpr) {
		if len(call.Args) != 1 {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SendKeys" {
			return
		}

		if isEnterConcatBinaryExpr(call.Args[0]) {
			pos := fset.Position(call.Pos())
			t.Errorf("%s: SendKeys(x + EnterKeySequence) single-write concatenation found — "+
				"this is the BUG-031 pattern; use session.SubmitDriverContent instead", pos)
		}
		if isBuildSubmittableInputCall(call.Args[0]) {
			pos := fset.Position(call.Pos())
			t.Errorf("%s: SendKeys(BuildSubmittableInput...(...)) single-write concatenation found — "+
				"this is the BUG-031 pattern; use session.SubmitDriverContent instead", pos)
		}
	})
}

// CheckNoAppendCarriageReturnConcatenation fails t if any non-test .go file
// in dir (other than those named in exemptFiles) calls append(x, 0x0D) — the
// tymux gRPC backend's original BUG-031 shape, which folded a prompt and its
// submit keystroke into one []byte before handing it to a single RPC send.
func CheckNoAppendCarriageReturnConcatenation(t *testing.T, dir string, exemptFiles ...string) {
	t.Helper()
	inspectCallExprs(t, dir, exemptFiles, func(fset *token.FileSet, call *ast.CallExpr) {
		if len(call.Args) != 2 {
			return
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "append" {
			return
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return
		}
		if lit.Value == "0x0D" || lit.Value == "0xD" || lit.Value == "13" {
			pos := fset.Position(call.Pos())
			t.Errorf("%s: append(x, 0x0D) single-write carriage-return concatenation found — "+
				"this is the BUG-031 pattern; send content and the Enter keystroke as two separate writes", pos)
		}
	})
}

// inspectCallExprs walks every non-test .go file in dir (other than those
// named in exemptFiles), invoking check on each *ast.CallExpr found.
func inspectCallExprs(t *testing.T, dir string, exemptFiles []string, check func(*token.FileSet, *ast.CallExpr)) {
	t.Helper()
	exempt := make(map[string]bool, len(exemptFiles))
	for _, f := range exemptFiles {
		exempt[f] = true
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s/*.go: %v", dir, err)
	}

	fset := token.NewFileSet()
	for _, path := range matches {
		base := filepath.Base(path)
		if exempt[base] || isGoTestFileName(base) {
			continue
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				check(fset, call)
			}
			return true
		})
	}
}

func isEnterConcatBinaryExpr(e ast.Expr) bool {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return false
	}
	return identNamed(bin.X, "EnterKeySequence") || identNamed(bin.Y, "EnterKeySequence")
}

func isBuildSubmittableInputCall(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	var name string
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name = fun.Name
	case *ast.SelectorExpr:
		name = fun.Sel.Name
	default:
		return false
	}
	return name == "BuildSubmittableInputAndSubmit" || name == "BuildSubmittableInput"
}

func identNamed(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isGoTestFileName(base string) bool {
	return len(base) > len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go"
}
