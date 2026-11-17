package services

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// Scope decision (ADR-007 D1): the ratchet protects the `status` column only,
// matching the five allow-listed raw writers. archived_at and the other columns
// are written alongside status inside those writers and are not gated here.

// clsStatusWriterMethods are the generated BacklogItem builder methods that
// write the status column: SetStatus and SetNillableStatus on
// Create/Update/UpdateOne/Upsert builders and the mutation, and UpdateStatus on
// the OnConflict upsert builders (sql/upsert feature).
var clsStatusWriterMethods = map[string]bool{
	"SetStatus":         true,
	"SetNillableStatus": true,
	"UpdateStatus":      true,
}

// clsRawStatusSQL matches a raw write of backlog_items.status.
var clsRawStatusSQL = regexp.MustCompile(`(?is)\b(update\s+"?backlog_items"?\b.*\bstatus\b|insert\s+into\s+"?backlog_items"?\b.*\bstatus\b)`)

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
	return ""
}

// clsNamesStatusField reports whether args name the status column via
// backlogitem.FieldStatus or the literal "status".
func clsNamesStatusField(args []ast.Expr) bool {
	for _, a := range args {
		switch v := a.(type) {
		case *ast.SelectorExpr:
			if v.Sel.Name == "FieldStatus" {
				return true
			}
		case *ast.BasicLit:
			if v.Kind == token.STRING && v.Value == `"status"` {
				return true
			}
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
	return clsRawStatusSQL.MatchString(s)
}
