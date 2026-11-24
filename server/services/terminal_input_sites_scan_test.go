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

// dirsWithOwnInputScanner maps a leaf directory to the file holding its own input-site scanner,
// so scanInputSites skips it. The file's existence is asserted: removing the scanner re-exposes the dir.
var dirsWithOwnInputScanner = map[string]string{
	"wsconn": "wsconn/terminal_input_sites_ws_test.go",
}

// scanInputSites finds, in non-test sources of this package and every leaf under it (a site moved
// into a new package stays visible), every inputReadLoopParams literal (which must set onInput),
// every GetInput() call and every TerminalData_Input type-switch case. Keys are slash paths
// relative to server/services plus the function name.
func scanInputSites(t *testing.T) map[string]bool {
	t.Helper()
	var files []string
	for _, dir := range servicesSourceDirs(t) {
		if _, own := dirsWithOwnInputScanner[filepath.ToSlash(dir)]; own {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		files = append(files, matches...)
	}
	found := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		f = filepath.ToSlash(f)
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

func TestInputSites_ShouldFail_When_OwnScannerFileMissing(t *testing.T) {
	t.Parallel()
	for dir, scanner := range dirsWithOwnInputScanner {
		assert.FileExists(t, scanner, "dir %s is skipped because this scanner guards it", dir)
	}
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
