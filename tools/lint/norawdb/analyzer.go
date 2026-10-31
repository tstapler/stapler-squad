// Package norawdb defines a go/analysis pass that rejects opening a SQLite
// database outside session/dbpool (ADR-036).
//
// Why: every database the registry opens is forced to WAL, gets a single
// writer, a pragma profile for its durability class and dbobs metrics. A raw
// sql.Open("sqlite", ...) gets none of that and is invisible to /actuator/db.
//
// Approved: session/dbpool and internal/sqlitedsn (the DSN builder), anything
// under scripts/, and _test.go files. A call elsewhere needs
// //nolint:norawdb with the reason (for example "writes to a file another tool
// owns"); read-only access to a foreign file goes through
// dbpool.OpenForeignReadOnly.
package norawdb

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the norawdb check.
var Analyzer = &analysis.Analyzer{
	Name:     "norawdb",
	Doc:      "rejects sql.Open of a SQLite driver outside session/dbpool; open databases through the dbpool registry so WAL, pool shape and metrics are enforced",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

var exemptPackageSuffixes = []string{"/session/dbpool", "/internal/sqlitedsn"}

// openFuncs are the driver-name-first Open functions that can open SQLite.
var openFuncs = map[string]bool{
	"database/sql":             true,
	"entgo.io/ent/dialect/sql": true,
	// the generated ent.Open(driverName, dsn)
	"github.com/tstapler/stapler-squad/session/ent": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	pkg := pass.Pkg.Path()
	if strings.Contains(pkg, "/scripts/") {
		return nil, nil
	}
	for _, suffix := range exemptPackageSuffixes {
		if strings.HasSuffix(pkg, suffix) {
			return nil, nil
		}
	}
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if !isSQLiteOpen(pass, call) {
			return
		}
		if strings.HasSuffix(pass.Fset.Position(call.Pos()).Filename, "_test.go") {
			return
		}
		switch nolintcomment.Lookup(pass, call.Pos(), "norawdb") {
		case nolintcomment.OK:
			return
		case nolintcomment.Missing:
			pass.Reportf(call.Pos(), "//nolint:norawdb needs a one-line reason")
			return
		}
		pass.Reportf(call.Pos(), "SQLite opened outside session/dbpool: use dbpool.OpenCore/OpenEscape (or dbpool.OpenForeignReadOnly for another tool's file); add //nolint:norawdb with a reason if the file must be opened directly")
	})
	return nil, nil
}

func isSQLiteOpen(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Open" || len(call.Args) == 0 {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || !openFuncs[fn.Pkg().Path()] {
		return false
	}
	tv, ok := pass.TypesInfo.Types[call.Args[0]]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return false
	}
	driver := constant.StringVal(tv.Value)
	return driver == "sqlite" || driver == "sqlite3"
}
