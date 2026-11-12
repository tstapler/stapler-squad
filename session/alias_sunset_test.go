package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// bridgedDTOAliases is the sunset list for `X = backlogdto.X` aliases. It may only
// shrink; BL-E deletes the bridge file and this list with it.
var bridgedDTOAliases = []string{
	"ActivityNoteData",
	"BacklogItemDependencyEdge",
	"BacklogItemDependencyRef",
	"BacklogStatusEventData",
	"DeletedItemSessionCostEntry",
	"DiagnoseResultData",
	"ItemSourceData",
	"ItemSourceUpdate",
	"OpenStuckStateData",
	"ProgressNoteData",
	"ReviewVerdictData",
	"ReviewVerdictSummary",
	"SourceSyncEventData",
	"StuckStateHistoryData",
}

const backlogdtoImportPath = "github.com/tstapler/stapler-squad/session/domain/backlogdto"

func TestBacklogDTOAliasesOnlyShrink(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{}
	for _, n := range bridgedDTOAliases {
		allowed[n] = true
	}
	count := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, alias := range backlogdtoAliases(f) {
			count++
			if !allowed[alias] {
				t.Errorf("%s: new alias %s = backlogdto.*; reference backlogdto directly", name, alias)
			}
		}
	}
	if count > len(bridgedDTOAliases) {
		t.Errorf("bridge grew: %d aliases, allowed %d", count, len(bridgedDTOAliases))
	}
}

// backlogdtoAliases returns the names of `type X = backlogdto.Y` declarations in f.
func backlogdtoAliases(f *ast.File) []string {
	local := backlogdtoLocalName(f)
	if local == "" {
		return nil
	}
	var names []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Assign.IsValid() && isSelectorOn(ts.Type, local) {
				names = append(names, ts.Name.Name)
			}
		}
	}
	return names
}

func isSelectorOn(e ast.Expr, pkg string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == pkg
}

func backlogdtoLocalName(f *ast.File) string {
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) != backlogdtoImportPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "backlogdto"
	}
	return ""
}
