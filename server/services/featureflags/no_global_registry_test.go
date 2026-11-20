package featureflags

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// A process-wide registry made flag defaults depend on startup order; every reader now receives its registry.
// Catches package-level vars whose type, initialiser, same-package constructor result or same-package struct
// type mentions Registry. Limit: syntactic and single-package; it cannot see a Registry reached through another
// package's type or an interface.
func TestPackage_ShouldDeclareNoPackageLevelRegistry(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		files = append(files, file)
		names = append(names, name)
	}
	holders := registryHolders(files)
	for i, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				if mentionsRegistry(vs.Type) || anyMentionsRegistry(vs.Values) || mentionsHolder(vs.Type, holders) || valuesMentionHolder(vs.Values, holders) {
					t.Errorf("%s: package-level var %v holds a Registry; inject it instead", names[i], vs.Names)
				}
			}
		}
	}
}

// registryHolders names the package's functions returning, and struct types containing, a Registry.
func registryHolders(files []*ast.File) map[string]bool {
	h := map[string]bool{}
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Type.Results != nil && d.Recv == nil {
					for _, r := range d.Type.Results.List {
						if mentionsRegistry(r.Type) {
							h[d.Name.Name] = true
						}
					}
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name != "Registry" {
						if st, ok := ts.Type.(*ast.StructType); ok && mentionsRegistry(st) {
							h[ts.Name.Name] = true
						}
					}
				}
			}
		}
	}
	return h
}

func mentionsHolder(e ast.Expr, holders map[string]bool) bool {
	found := false
	if e == nil {
		return false
	}
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && holders[id.Name] {
			found = true
		}
		return !found
	})
	return found
}

func valuesMentionHolder(exprs []ast.Expr, holders map[string]bool) bool {
	for _, e := range exprs {
		if mentionsHolder(e, holders) {
			return true
		}
	}
	return false
}

func anyMentionsRegistry(exprs []ast.Expr) bool {
	for _, e := range exprs {
		if mentionsRegistry(e) {
			return true
		}
	}
	return false
}

// mentionsRegistry reports whether the expression names Registry anywhere, including *Registry,
// atomic.Pointer[Registry] and &Registry{} / NewRegistry(...) initialisers.
func mentionsRegistry(e ast.Expr) bool {
	found := false
	if e == nil {
		return false
	}
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && (id.Name == "Registry" || id.Name == "NewRegistry") {
			found = true
		}
		return !found
	})
	return found
}
