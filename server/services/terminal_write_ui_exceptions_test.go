//go:build sinkguard

package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// T-RO-11 (O7 half): the UI-exception steer path is reachable only through
// functions that take the BacklogReviewLink capability, which is constructible
// only in hidden_review_steer.go (check (a)). The Reply half of the row
// (PendingQuestionClaim) is Story 5.6 and has no code to reflect on yet.
func TestUiExceptionEntryPoints_ShouldEachRequireTheirCapabilityParam_WhenReflected(t *testing.T) {
	entryPoints := []struct{ file, recv, name string }{
		{"hidden_review_steer.go", "TerminalAccess", "BacklogSteerWriter"},
		{"hidden_review_steer.go", "SessionService", "steerHiddenReviewViaBacklogLink"},
		{"steer_authorization.go", "SessionService", "steerBacklogLinked"},
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
			if id, ok := p.Type.(*ast.Ident); ok && id.Name == "BacklogReviewLink" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s.%s must take a BacklogReviewLink parameter", ep.recv, ep.name)
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
