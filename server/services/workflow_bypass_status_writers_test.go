package services

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// Scope (ADR-007 D1): the ratchet protects the `status` column only, matching
// the five allow-listed raw writers; archived_at and other columns are written
// alongside status inside those writers and are not gated here.
//
// This is a BEST-EFFORT static heuristic over Go AST call expressions and Go
// string literals, not a guarantee. Known uncovered (there are no DB-level
// triggers on backlog_items, so review and the Gated* tests are the backstop):
//   - a builder aliased or passed to a helper (setIt(u)), or a method value
//     (f := u.SetStatus): only direct call expressions are matched.
//   - SetField on an untyped ent.Mutation in a global hook, and reflection.
//   - SQL built at runtime (fmt.Sprintf, concatenation), SQL in package-level
//     const/var (the walker visits function bodies only), SQL in non-Go files.
//   - SQL dialect forms the heuristic does not enumerate (see
//     workflow_bypass_sql_test.go): REPLACE INTO backlog_items (cols) with an
//     explicit column list omitting status is not flagged (the row is
//     re-inserted, so status resets to its default); a quote inside a
//     double-quoted identifier can open a phantom string; a top-level
//     FROM/WHERE/RETURNING-like keyword inside the SET list (SET t = x IS NOT
//     DISTINCT FROM y, status = 'z') truncates it so a later status assignment
//     is missed; an unterminated "(" drops the rest of the statement.
//   - Entgo sql builder chains split across variables.

// clsStatusWriterMethods are the generated BacklogItem builder methods that
// can write the status column: SetStatus and SetNillableStatus on
// Create/Update/UpdateOne/Upsert builders and the mutation, UpdateStatus on the
// OnConflict upsert builders (sql/upsert feature), and UpdateNewValues, which
// rewrites every column of an upsert (status included) from the create values.
var clsStatusWriterMethods = map[string]bool{
	"SetStatus":         true,
	"SetNillableStatus": true,
	"UpdateStatus":      true,
	"UpdateNewValues":   true,
}

// clsStatusWriterName returns the status-writing method a call invokes on a
// BacklogItem builder, or "" if the call is not one.
func clsStatusWriterName(fd *ast.FuncDecl, call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if clsStatusWriterMethods[sel.Sel.Name] && clsMentionsBacklogItemBuilder(fd, sel.X, 0) {
		return sel.Sel.Name
	}
	if sel.Sel.Name == "SetField" && clsNamesStatusField(call.Args) && clsMentionsBacklogItemBuilder(fd, sel.X, 0) {
		return "SetField(status)"
	}
	if clsEntSQLBuilderWritesStatus(sel, call.Args) {
		return "sql builder " + sel.Sel.Name + "(status)"
	}
	return ""
}

// clsEntSQLBuilderWritesStatus matches the entgo sql builder written as one
// chain: sql.Update("backlog_items")...Set("status", v) or
// sql.Insert("backlog_items")...Columns("status", ...).
func clsEntSQLBuilderWritesStatus(sel *ast.SelectorExpr, args []ast.Expr) bool {
	var opener string
	switch sel.Sel.Name {
	case "Set":
		opener = "Update"
	case "Columns":
		opener = "Insert"
	default:
		return false
	}
	return clsNamesStatusField(args) && clsChainOpensOnBacklogTable(sel.X, opener)
}

func clsChainOpensOnBacklogTable(e ast.Expr, opener string) bool {
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == opener && len(call.Args) == 1 && clsIsStringLit(call.Args[0], "backlog_items") {
			return true
		}
		e = sel.X
	}
}

func clsIsStringLit(e ast.Expr, want string) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	return err == nil && s == want
}

// clsNamesStatusField reports whether args name the status column via
// backlogitem.FieldStatus or the literal "status".
func clsNamesStatusField(args []ast.Expr) bool {
	for _, a := range args {
		if sel, ok := a.(*ast.SelectorExpr); ok && sel.Sel.Name == "FieldStatus" {
			return true
		}
		if clsIsStringLit(a, "status") {
			return true
		}
	}
	return false
}

// clsRawStatusSQLLiteral reports whether n is a string literal that writes
// backlog_items.status. Only literals inside function bodies are visible.
func clsRawStatusSQLLiteral(n ast.Node) bool {
	lit, ok := n.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		s = strings.Trim(lit.Value, "`")
	}
	return clsSQLWritesBacklogStatus(s)
}
