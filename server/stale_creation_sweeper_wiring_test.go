package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// The stale-creation sweeper lives in server/services/sessioncreate; only this
// start keeps a session whose pipeline died from staying Creating.
func TestServerWiring_ShouldStartTheStaleCreationSweeper(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	require.NoError(t, err)

	var built, started int
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "NewStaleCreationSweeper":
			built++
		case "goBackground":
			started += countStarts(call, "staleCreationSweeper")
		}
		return true
	})
	require.Equal(t, 1, built, "server.go must build the sweeper once")
	require.Equal(t, 1, started, "server.go must start it with srv.goBackground")
}

// countStarts counts name.Start selectors under n.
func countStarts(n ast.Node, name string) int {
	var count int
	ast.Inspect(n, func(inner ast.Node) bool {
		sel, ok := inner.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Start" {
			return true
		}
		if recv, ok := sel.X.(*ast.Ident); ok && recv.Name == name {
			count++
		}
		return true
	})
	return count
}
