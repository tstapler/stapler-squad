package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
)

// A nil Ports.DerivePriority only logs and publishes at unspecified priority, which no
// compiler or leaf test sees; this pins the production wiring in server.go.
func TestHealthNotifiers_should_BeWiredWithDerivePriority_When_ServerConstructsThem(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	require.NoError(t, err)
	got := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			return true
		}
		name := types.ExprString(call.Fun)
		if name != "healthnotify.NewStaleSessionNotifier" && name != "healthnotify.NewMemoryPressureNotifier" {
			return true
		}
		ports, isLit := call.Args[2].(*ast.CompositeLit)
		require.True(t, isLit, "%s: ports must be a literal", name)
		require.Len(t, ports.Elts, 1, name)
		kv, isKV := ports.Elts[0].(*ast.KeyValueExpr)
		require.True(t, isKV, name)
		got[name] = types.ExprString(kv.Key) + ":" + types.ExprString(kv.Value)
		return true
	})
	require.Equal(t, map[string]string{
		"healthnotify.NewStaleSessionNotifier":   "DerivePriority:services.DerivePriority",
		"healthnotify.NewMemoryPressureNotifier": "DerivePriority:services.DerivePriority",
	}, got)
}
