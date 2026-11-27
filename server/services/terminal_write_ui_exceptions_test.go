//go:build sinkguard

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// T-RO-11: each UI-exception path is reachable only through functions that take
// its capability: the O7 steer takes the BacklogReviewLink (constructible only in
// hidden_review_steer.go) and Reply takes the PendingQuestionClaim (constructible
// only in questionkit/pending_question_store.go), check (a).
func TestUiExceptionEntryPoints_ShouldEachRequireTheirCapabilityParam_WhenReflected(t *testing.T) {
	entryPoints := []struct{ file, recv, name, param string }{
		{"hidden_review_steer.go", "", "newBacklogSteerWriter", "TerminalAccess"},
		{"hidden_review_steer.go", "", "newBacklogSteerWriter", "BacklogReviewLink"},
		{"hidden_review_steer.go", "SessionService", "steerHiddenReviewViaBacklogLink", "BacklogReviewLink"},
		{"steer_authorization.go", "SessionService", "steerBacklogLinked", "BacklogReviewLink"},
		{"qareply/pane_reply_core.go", "", "newQuestionReplyWriter", "TerminalAccess"},
		{"qareply/pane_reply_core.go", "", "newQuestionReplyWriter", "PendingQuestionClaim"},
		{"qareply/pane_reply_core.go", "PaneReplyCore", "writeUnderLease", "PendingQuestionClaim"},
	}
	for _, ep := range entryPoints {
		f, err := parser.ParseFile(token.NewFileSet(), ep.file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var fn *ast.FuncDecl
		for _, d := range f.Decls {
			if decl, ok := d.(*ast.FuncDecl); ok && decl.Name.Name == ep.name && recvName(decl) == ep.recv {
				fn = decl
			}
		}
		if fn == nil {
			t.Errorf("%s.%s not found in %s", ep.recv, ep.name, ep.file)
			continue
		}
		found := false
		for _, p := range fn.Type.Params.List {
			switch pt := p.Type.(type) {
			case *ast.Ident:
				found = found || pt.Name == ep.param
			case *ast.SelectorExpr: // a type of another package, e.g. access.TerminalAccess
				found = found || pt.Sel.Name == ep.param
			}
		}
		if !found {
			t.Errorf("%s.%s must take a %s parameter", ep.recv, ep.name, ep.param)
		}
	}
}

func recvName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
