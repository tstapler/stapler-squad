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
func TestPackage_ShouldDeclareNoPackageLevelRegistry(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, name, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				if mentionsRegistry(vs.Type) || anyMentionsRegistry(vs.Values) {
					t.Errorf("%s: package-level var %v holds a Registry; inject it instead", name, vs.Names)
				}
			}
		}
	}
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
