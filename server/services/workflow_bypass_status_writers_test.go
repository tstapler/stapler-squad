package services

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// Scope (ADR-007 D1): the ratchet protects the `status` column only, matching
// the five allow-listed raw writers; archived_at and other columns are written
// alongside status inside those writers and are not gated here.
//
// Accepted blind spots (nothing else covers them; there are no DB-level
// triggers on backlog_items, so review and the Gated* tests are the backstop):
//   - a builder aliased or passed to a helper (setIt(u)): the walk is per
//     function and resolves receivers only through local assignments/params.
//   - a method value (f := u.SetStatus; f("x")): only call expressions match.
//   - SetField on an untyped ent.Mutation in a global hook: no BacklogItem
//     type is nameable there.
//   - reflection.
//   - SQL built with fmt.Sprintf or concatenation: only whole literals match.
//   - package-level const/var SQL: the walker visits function bodies only.
//   - SQL in non-Go files (migrations).
//   - builder chains split across variables for the entgo sql builder
//     (sql.Update("backlog_items") assigned, then .Set later).

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

var (
	clsSQLStmtUpdate = regexp.MustCompile("(?s)^\\s*update\\s+(?:or\\s+\\w+\\s+)?[`\"]?backlog_items[`\"]?\\s+set\\s+(.*)$")
	clsSQLStmtInsert = regexp.MustCompile("(?s)^\\s*(?:insert\\s+(?:or\\s+\\w+\\s+)?into|replace\\s+into)\\s+[`\"]?backlog_items[`\"]?\\s*\\(([^)]*)\\)(.*)$")
	clsSQLSetEnd     = regexp.MustCompile(`\b(where|returning|from)\b`)
	clsSQLUpsertSet  = regexp.MustCompile(`(?s)\bdo\s+update\s+set\s+(.*)$`)
	clsSQLStatusCol  = regexp.MustCompile(`\bstatus\b`)
	clsSQLStatusSet  = regexp.MustCompile("[`\"]?\\bstatus[`\"]?\\s*=")
)

// clsSQLWritesBacklogStatus reports whether sql contains a statement that writes
// backlog_items.status: status in an UPDATE's SET list (before WHERE, RETURNING
// or FROM), in an INSERT/REPLACE column list, or in an upsert's DO UPDATE SET.
func clsSQLWritesBacklogStatus(sql string) bool {
	for _, stmt := range strings.Split(strings.ToLower(sql), ";") {
		if m := clsSQLStmtUpdate.FindStringSubmatch(stmt); m != nil {
			set := m[1]
			if loc := clsSQLSetEnd.FindStringIndex(set); loc != nil {
				set = set[:loc[0]]
			}
			if clsSQLStatusSet.MatchString(set) {
				return true
			}
		}
		if m := clsSQLStmtInsert.FindStringSubmatch(stmt); m != nil {
			if clsSQLStatusCol.MatchString(m[1]) {
				return true
			}
			if u := clsSQLUpsertSet.FindStringSubmatch(m[2]); u != nil && clsSQLStatusSet.MatchString(u[1]) {
				return true
			}
		}
	}
	return false
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
