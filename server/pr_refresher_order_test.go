package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// callPositions returns the source offset of every call named name in fn's body.
func callPositions(fn *ast.FuncDecl, name string) []token.Pos {
	var out []token.Pos
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch f := call.Fun.(type) {
			case *ast.Ident:
				if f.Name == name {
					out = append(out, call.Pos())
				}
			case *ast.SelectorExpr:
				if f.Sel.Name == name {
					out = append(out, call.Pos())
				}
			}
		}
		return true
	})
	return out
}

func parseFunc(t *testing.T, file, name string) *ast.FuncDecl {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("%s not found in %s", name, file)
	return nil
}

// The PR state refresher (PRStatusPoller) is built in BuildServiceDeps and
// started by the server from the deps BuildDependencies returns. PR startup
// maintenance must have finished by then: it runs inside BuildCoreDepsWithOptions,
// the first phase, and the server never starts the poller before BuildDependencies returns.
func TestPRStatusPoller_ShouldStartAfterPRStartupMaintenance(t *testing.T) {
	build := parseFunc(t, "dependencies.go", "BuildDependencies")
	core := callPositions(build, "BuildCoreDepsWithOptions")
	service := callPositions(build, "BuildServiceDeps")
	require.Len(t, core, 1)
	require.Len(t, service, 1)
	require.Less(t, core[0], service[0], "maintenance (phase 1) must precede the poller's construction (phase 2)")

	// Maintenance is requested only by the service bootstrap, and runs inside phase 1.
	coreFn := parseFunc(t, "dependencies.go", "BuildCoreDepsWithOptions")
	require.NotEmpty(t, callPositions(coreFn, "runPRMaintenance"))
	require.Empty(t, callPositions(coreFn, "NewPRStatusPoller"), "the poller is not built before maintenance has run")
	require.NotEmpty(t, callPositions(parseFunc(t, "dependencies.go", "BuildServiceDeps"), "NewPRStatusPoller"))

	// The poller is started in the server, from deps that BuildDependencies already returned.
	srv := parseFunc(t, "server.go", "wireDepsIntoServer")
	require.NotEmpty(t, callPositions(srv, "Start"))
	require.Empty(t, callPositions(srv, "BuildCoreDepsWithOptions"))
	require.Empty(t, callPositions(srv, "runPRMaintenance"))
}
