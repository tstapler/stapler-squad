package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// The backlog DTOs live in session/domain/backlogdto; the temporary bridge that re-exported them from
// package session is gone, and this guard keeps it from coming back.
func TestNoBacklogDTOAliasesInSession(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, ts := range typeSpecs(file) {
			if name, ok := backlogdtoAliasTarget(ts); ok {
				t.Errorf("%s: `type %s = backlogdto.%s` re-exports a backlog DTO from package session; "+
					"import session/domain/backlogdto and use backlogdto.%s directly instead",
					fset.Position(ts.Pos()), ts.Name.Name, name, name)
			}
		}
	}
}

func typeSpecs(file *ast.File) []*ast.TypeSpec {
	var out []*ast.TypeSpec
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, s := range gd.Specs {
			out = append(out, s.(*ast.TypeSpec))
		}
	}
	return out
}

// backlogdtoAliasTarget reports the target name when ts aliases a backlogdto type.
func backlogdtoAliasTarget(ts *ast.TypeSpec) (string, bool) {
	sel, ok := ts.Type.(*ast.SelectorExpr)
	if !ts.Assign.IsValid() || !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "backlogdto" {
		return "", false
	}
	return sel.Sel.Name, true
}
