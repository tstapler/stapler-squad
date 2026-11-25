package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/itemlink"
)

// Scans non-test sources so a new string-literal producer fails here until it is classified.
func TestEndReasonClassification_ShouldCoverEveryLiteralInSource(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..")
	reasonArg := map[string]int{"UpdateItemSessionEndedWithReason": 3, "UpdateItemSessionEndedWithDetail": 3, "failJulesSession": 3}
	found := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "gen", "node_modules", "ent", ".git", ".claude", "web-app", "dist": // .claude holds gitignored agent worktree checkouts
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			case *ast.Ident:
				name = fn.Name
			}
			idx, ok := reasonArg[name]
			if !ok || idx >= len(call.Args) {
				return true
			}
			if lit, ok := call.Args[idx].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				found[v] = path
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, found, "scan found no producers; the walker is broken")
	for r, path := range found {
		_, fail := itemlink.FailureEndReasons[r]
		_, non := itemlink.NonFailureEndReasons[r]
		assert.Truef(t, fail != non, "literal end_reason %q (%s) must be classified exactly once", r, path)
	}
}
