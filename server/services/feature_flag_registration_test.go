package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// flagNameConsts returns the value of every string const named *FlagName (or *FlagName-suffixed alias to a
// config.* const) declared in dir's non-test files, resolving config.X selectors against ../../config.
func flagNameConsts(t *testing.T, dir string) map[string]string {
	t.Helper()
	cfgConsts := parseStringConsts(t, filepath.Join("..", "..", "config"))
	own := parseStringConsts(t, dir)
	out := map[string]string{}
	for name, v := range own {
		if !strings.HasSuffix(name, "FlagName") && !strings.HasSuffix(name, "flagName") {
			continue
		}
		if strings.HasPrefix(v, "config.") {
			v = cfgConsts[strings.TrimPrefix(v, "config.")]
		}
		out[name] = v
	}
	return out
}

// parseStringConsts returns const name -> literal value, or "config.X" for a selector initializer.
func parseStringConsts(t *testing.T, dir string) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		collectConsts(f, out)
	}
	return out
}

// TestEveryFlagNameConstIsRegistered fails when a string const named *FlagName in services or any leaf has
// no Spec in the registry, so a new flag cannot ship without its Spec entry.
func TestEveryFlagNameConstIsRegistered(t *testing.T) {
	t.Parallel()
	// Not flags: the const names a gated RPC or a non-feature-flag config key.
	notFlags := map[string]bool{}
	reg := testFlagRegistry()
	checked := 0
	for _, dir := range servicesSourceDirs(t) {
		for constName, value := range flagNameConsts(t, dir) {
			if notFlags[constName] {
				continue
			}
			checked++
			if value == "" {
				t.Errorf("%s: could not resolve its value", constName)
				continue
			}
			if _, ok := reg.Lookup(value); !ok {
				t.Errorf("%s = %q has no featureflags.Spec; add it to a Spec list assembled by FlagRegistry", constName, value)
			}
		}
	}
	if checked < 20 {
		t.Fatalf("scanned only %d flag-name consts; the scan is broken", checked)
	}
}

// collectConsts records every string-literal or config.X const of f into out.
func collectConsts(f *ast.File, out map[string]string) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					if v, ok := constValue(vs.Values[i]); ok {
						out[n.Name] = v
					}
				}
			}
		}
	}
}

// constValue returns a string literal's value, or "config.X" for a config selector.
func constValue(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(v.Value)
		return s, err == nil && v.Kind == token.STRING
	case *ast.SelectorExpr:
		if id, ok := v.X.(*ast.Ident); ok && id.Name == "config" {
			return "config." + v.Sel.Name, true
		}
	}
	return "", false
}
