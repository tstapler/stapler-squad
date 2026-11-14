package backlogsvc

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callsByFunc maps each top-level function of the package's non-test files to the
// function and method names it calls.
func callsByFunc(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	fset := token.NewFileSet()
	out := map[string]map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		require.NoError(t, err)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				out[fn.Name.Name] = calledNames(fn.Body)
			}
		}
	}
	return out
}

func calledNames(body *ast.BlockStmt) map[string]bool {
	calls := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := c.Fun.(type) {
		case *ast.SelectorExpr:
			calls[fun.Sel.Name] = true
		case *ast.Ident:
			calls[fun.Name] = true
		}
		return true
	})
	return calls
}

// Class guard for "swallowed base-commit stamp": the spawn paths once wrote
// `_ = SetItemSessionBaseCommit(...)` behind an `err == nil` check, so a failure left the
// session with no base and the PR-ownership guard blind. Writes must go through
// stampBaseCommit (which logs and notifies), and every path that creates a work or
// re-review ItemSession must call it.
func TestBaseCommitStamping_should_GoThroughStampBaseCommit_OnEverySpawnPath(t *testing.T) {
	t.Parallel()
	calls := callsByFunc(t)

	for fn, c := range calls {
		if fn != "stampBaseCommit" {
			assert.Falsef(t, c["SetItemSessionBaseCommit"], "%s writes the base commit directly; use stampBaseCommit so failures are reported", fn)
		}
	}
	for _, fn := range []string{"spawnSessionAfterGates", "AttachSessionToItem", "TriggerReReview"} {
		require.Truef(t, calls[fn]["CreateItemSession"], "%s no longer creates an ItemSession; update this guard", fn)
		assert.Truef(t, calls[fn]["stampBaseCommit"], "%s creates an ItemSession but never stamps its base commit", fn)
	}
}
