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
	"strconv"
)

// reporter is the narrow slice of *testing.T this package needs — every
// exported Check function takes one instead of a concrete *testing.T so this
// package's own tests can verify detection against a fake recorder instead
// of a real (permanently-failing) *testing.T subtest.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// CheckNoSingleWriteEnterConcatenation fails t if any non-test .go file in
// dir (other than those named in exemptFiles) calls a SendKeys-style method
// with a single argument that concatenates content with an Enter keystroke —
// either `SendKeys(x + EnterKeySequence)` or `SendKeys(BuildSubmittableInput(...))`
// / `SendKeys(BuildSubmittableInputAndSubmit(...))`.
func CheckNoSingleWriteEnterConcatenation(t reporter, dir string, exemptFiles ...string) {
	t.Helper()
	inspectFiles(t, dir, exemptFiles, func(fset *token.FileSet, file *ast.File) {
		// assigns lets a SendKeys(text) call be traced back to text's most
		// recent same-file assignment before checking it — otherwise
		// `text := content + EnterKeySequence; inst.SendKeys(text)` bypasses
		// the check entirely just by naming the concatenated value first.
		// Single-file, last-assignment-wins is an approximation (no real
		// scoping/control-flow analysis), but is enough to catch the
		// straightforward "extract a variable" refactor this guards against.
		assigns := collectSimpleAssignments(file)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SendKeys" {
				return true
			}

			arg := resolveIdent(call.Args[0], assigns)
			if isEnterConcatBinaryExpr(arg) {
				pos := fset.Position(call.Pos())
				t.Errorf("%s: SendKeys(x + EnterKeySequence) single-write concatenation found — "+
					"this is the BUG-031 pattern; use session.SubmitDriverContent instead", pos)
			}
			if isBuildSubmittableInputCall(arg) {
				pos := fset.Position(call.Pos())
				t.Errorf("%s: SendKeys(BuildSubmittableInput...(...)) single-write concatenation found — "+
					"this is the BUG-031 pattern; use session.SubmitDriverContent instead", pos)
			}
			return true
		})
	})
}

// CheckNoAppendCarriageReturnConcatenation fails t if any non-test .go file
// in dir (other than those named in exemptFiles) calls append(x, 0x0D) — the
// tymux gRPC backend's original BUG-031 shape, which folded a prompt and its
// submit keystroke into one []byte before handing it to a single RPC send.
func CheckNoAppendCarriageReturnConcatenation(t reporter, dir string, exemptFiles ...string) {
	t.Helper()
	inspectCallExprs(t, dir, exemptFiles, func(fset *token.FileSet, call *ast.CallExpr) {
		if len(call.Args) != 2 {
			return
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != "append" {
			return
		}
		if isCarriageReturnByteLiteral(call.Args[1]) {
			pos := fset.Position(call.Pos())
			t.Errorf("%s: append(x, <carriage-return byte>) single-write carriage-return concatenation found — "+
				"this is the BUG-031 pattern; send content and the Enter keystroke as two separate writes", pos)
		}
	})
}

// inspectFiles parses every non-test .go file in dir (other than those named
// in exemptFiles) and invokes visit once per file.
func inspectFiles(t reporter, dir string, exemptFiles []string, visit func(*token.FileSet, *ast.File)) {
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
		visit(fset, file)
	}
}

// inspectCallExprs walks every non-test .go file in dir (other than those
// named in exemptFiles), invoking check on each *ast.CallExpr found.
func inspectCallExprs(t reporter, dir string, exemptFiles []string, check func(*token.FileSet, *ast.CallExpr)) {
	t.Helper()
	inspectFiles(t, dir, exemptFiles, func(fset *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				check(fset, call)
			}
			return true
		})
	})
}

// collectSimpleAssignments builds a same-file map of identifier name → its
// most recently assigned expression, for `x := expr`, `x = expr`, and
// `var x = expr` — enough to resolve one level of "extract a variable"
// indirection before a single-write BUG-031 pattern check runs.
func collectSimpleAssignments(file *ast.File) map[string]ast.Expr {
	assigns := make(map[string]ast.Expr)
	ast.Inspect(file, func(n ast.Node) bool {
		recordSimpleAssignment(assigns, n)
		return true
	})
	return assigns
}

// recordSimpleAssignment records n's single-name/single-value assignment (an
// `x := expr`/`x = expr` AssignStmt or a `var x = expr` ValueSpec) into
// assigns, if n is one of those shapes.
func recordSimpleAssignment(assigns map[string]ast.Expr, n ast.Node) {
	switch node := n.(type) {
	case *ast.AssignStmt:
		if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
			return
		}
		if id, ok := node.Lhs[0].(*ast.Ident); ok {
			assigns[id.Name] = node.Rhs[0]
		}
	case *ast.ValueSpec:
		if len(node.Names) == 1 && len(node.Values) == 1 {
			assigns[node.Names[0].Name] = node.Values[0]
		}
	}
}

// resolveIdent follows e through assigns while it's a bare identifier with a
// known assignment, up to a small bound (guards against a self-referential
// or cyclic map from an unusual assignment shape), returning e unchanged
// once it's no longer a resolvable identifier.
func resolveIdent(e ast.Expr, assigns map[string]ast.Expr) ast.Expr {
	for range 5 {
		id, ok := e.(*ast.Ident)
		if !ok {
			return e
		}
		next, found := assigns[id.Name]
		if !found {
			return e
		}
		e = next
	}
	return e
}

// carriageReturnLiterals are the ways a Go source file can spell "\r" as a
// string literal — both the interpreted form (`"\r"`) and the raw form
// (a literal containing an actual CR byte) go through go/scanner the same
// way once parsed, so comparing the literal's decoded value (not its raw
// source text) catches both.
const carriageReturn = "\r"

func isEnterConcatBinaryExpr(e ast.Expr) bool {
	bin, ok := e.(*ast.BinaryExpr)
	if !ok || bin.Op != token.ADD {
		return false
	}
	return isEnterKeySequenceRef(bin.X) || isEnterKeySequenceRef(bin.Y) ||
		isCarriageReturnLiteral(bin.X) || isCarriageReturnLiteral(bin.Y)
}

// isEnterKeySequenceRef matches a reference to EnterKeySequence whether
// written as the bare identifier (inside the session package itself) or
// package-qualified (session.EnterKeySequence, as every call site outside
// the session package must write it).
func isEnterKeySequenceRef(e ast.Expr) bool {
	switch expr := e.(type) {
	case *ast.Ident:
		return expr.Name == "EnterKeySequence"
	case *ast.SelectorExpr:
		return expr.Sel.Name == "EnterKeySequence"
	default:
		return false
	}
}

// isCarriageReturnLiteral matches the literal "\r" spelled directly in
// source — the exact shape steerSession's old PTY fallback used
// (`message + "\r"`) instead of referencing the named EnterKeySequence
// constant at all.
func isCarriageReturnLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	val, err := strconv.Unquote(lit.Value)
	return err == nil && val == carriageReturn
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

// isCarriageReturnByteLiteral matches a literal spelling of the carriage
// return byte (13 / 0x0D / 0xd / '\r') regardless of base or case — an
// append(x, 0x0d) or append(x, '\r') is the same BUG-031 shape as
// append(x, 0x0D), just spelled differently.
func isCarriageReturnByteLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok {
		return false
	}
	switch lit.Kind {
	case token.INT:
		n, err := strconv.ParseInt(lit.Value, 0, 64)
		return err == nil && n == '\r'
	case token.CHAR:
		r, _, _, err := strconv.UnquoteChar(lit.Value[1:len(lit.Value)-1], '\'')
		return err == nil && r == '\r'
	default:
		return false
	}
}

func isGoTestFileName(base string) bool {
	return len(base) > len("_test.go") && base[len(base)-len("_test.go"):] == "_test.go"
}
