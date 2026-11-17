package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const moduleImportPrefix = "github.com/tstapler/stapler-squad/"

// Types live in the package that owns them (session/domain, session/domain/backlogdto, session/queue,
// ...). Re-exporting one from package session with `type X = pkg.Y` keeps old import paths alive and
// binds session back to the packages it was split from, so this guard keeps the bridge from coming back.
func TestNoReexportAliasesInSession(t *testing.T) {
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
		imports := moduleImportNames(file)
		for _, ts := range typeSpecs(file) {
			if pkg, name, ok := reexportAliasTarget(ts, imports); ok {
				t.Errorf("%s: `type %s = %s.%s` re-exports a type from package session; delete the alias, "+
					"import %s and use %s.%s at every use site (see CLAUDE.md, Package layout: no lasting type aliases)",
					fset.Position(ts.Pos()), ts.Name.Name, pkg, name, imports[pkg], pkg, name)
			}
		}
	}
}

// moduleImportNames maps each in-module import's local name to its import path.
func moduleImportNames(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, im := range file.Imports {
		path, err := strconv.Unquote(im.Path.Value)
		if err != nil || !strings.HasPrefix(path, moduleImportPrefix) {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		out[name] = path
	}
	return out
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

// reexportAliasTarget reports the package name and type name when ts is an alias (`=`) of a type from
// another package of this module, including instantiated generics (`type X = pkg.Y[T]`).
func reexportAliasTarget(ts *ast.TypeSpec, imports map[string]string) (pkg, name string, ok bool) {
	if !ts.Assign.IsValid() {
		return "", "", false
	}
	expr := ts.Type
	switch e := expr.(type) {
	case *ast.IndexExpr:
		expr = e.X
	case *ast.IndexListExpr:
		expr = e.X
	}
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel {
		return "", "", false
	}
	id, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", "", false
	}
	if _, inModule := imports[id.Name]; !inModule {
		return "", "", false
	}
	return id.Name, sel.Sel.Name, true
}
