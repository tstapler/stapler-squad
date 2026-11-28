package sessioncreate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The pipeline itself needs a real tmux start to reach this call, so the call is
// pinned in the source.
func TestAutonomyWiring_ShouldBeCalledByTheCreationPipeline(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "pipeline.go", nil, 0)
	require.NoError(t, err)
	var calls int
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "RunResolutionPipeline" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "StartCreationAutonomousDriver" {
				calls++
			}
			return true
		})
	}
	assert.Equal(t, 1, calls)
}
