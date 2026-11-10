package services

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The websocket input sites are guarded in wsconn; this guards the Connect-stream site that stayed here.
var inputSiteAllowList = map[string]string{
	"session_service_stream_terminal.go:StreamTerminal": "Connect stream path (path stream)",
}

// scanInputSites finds, in non-test sources, every inputReadLoopParams literal (which must set
// onInput), every GetInput() call and every TerminalData_Input type-switch case.
func scanInputSites(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if funcHasInputSite(t, fn) {
				found[f+":"+fn.Name.Name] = true
			}
		}
	}
	return found
}

func funcHasInputSite(t *testing.T, fn *ast.FuncDecl) bool {
	t.Helper()
	hit := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CompositeLit:
			if id, ok := x.Type.(*ast.Ident); ok && id.Name == "inputReadLoopParams" {
				hit = true
				assert.True(t, litSetsKey(x, "onInput"), "%s builds inputReadLoopParams without onInput", fn.Name.Name)
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "GetInput" {
				hit = true
			}
		case *ast.CaseClause:
			for _, e := range x.List {
				if star, ok := e.(*ast.StarExpr); ok {
					if sel, ok := star.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "TerminalData_Input" {
						hit = true
					}
				}
			}
		}
		return true
	})
	return hit
}

func litSetsKey(lit *ast.CompositeLit, key string) bool {
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
				return true
			}
		}
	}
	return false
}

func TestInputSites_ShouldFail_When_ConnectStreamInputWiringNotInAllowList(t *testing.T) {
	t.Parallel()
	found := scanInputSites(t)
	var unlisted, stale []string
	for site := range found {
		if _, ok := inputSiteAllowList[site]; !ok {
			unlisted = append(unlisted, site)
		}
	}
	for site := range inputSiteAllowList {
		if !found[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(unlisted)
	sort.Strings(stale)
	assert.Empty(t, unlisted, "new input entry point: wire it to recordTerminalInput (received before gates, forwarded after the write) and add it to inputSiteAllowList")
	assert.Empty(t, stale, "allow-list entry no longer matches an input site; remove it")
}
