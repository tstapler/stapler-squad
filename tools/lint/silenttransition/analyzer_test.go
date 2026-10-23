package silenttransition_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/silenttransition"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, silenttransition.Analyzer, "a")
}

// registryFile is the caller registry, read as source because this module
// cannot import package session.
const registryFile = "../../../session/transition_callers.go"

// TestSilentTransition_OperatorCallerSetMatchesRegistry fails when the
// analyzer's operator set drifts from the registry's operator-class (specOp)
// entries, so a new operator caller cannot dodge the check.
func TestSilentTransition_OperatorCallerSetMatchesRegistry(t *testing.T) {
	registry := operatorCallersInRegistry(t)
	if len(registry) == 0 {
		t.Fatal("found no operator-class (specOp) entries in " + registryFile)
	}

	var analyzer []string
	for name := range silenttransition.OperatorCallers {
		analyzer = append(analyzer, name)
	}
	sort.Strings(analyzer)

	if !equalStrings(analyzer, registry) {
		t.Fatalf("silenttransition.OperatorCallers drifted from the specOp entries in session/transition_callers.go\nanalyzer: %v\nregistry: %v", analyzer, registry)
	}
}

// operatorCallersInRegistry returns the sorted string values of the Caller
// constants passed as the first argument of specOp(...) in the registry.
func operatorCallersInRegistry(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), registryFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", registryFile, err)
	}

	values := map[string]string{} // constant name -> string value
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != len(spec.Values) {
			return true
		}
		for i, name := range spec.Names {
			if lit, ok := spec.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if v, err := strconv.Unquote(lit.Value); err == nil {
					values[name.Name] = v
				}
			}
		}
		return true
	})

	var operators []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "specOp" {
			return true
		}
		first, ok := call.Args[0].(*ast.Ident)
		if !ok {
			t.Fatalf("specOp's first argument at %v is not a named constant", call.Pos())
		}
		v, ok := values[first.Name]
		if !ok {
			t.Fatalf("specOp(%s, ...) names a constant with no string value in %s", first.Name, registryFile)
		}
		operators = append(operators, v)
		return true
	})
	sort.Strings(operators)
	return operators
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
