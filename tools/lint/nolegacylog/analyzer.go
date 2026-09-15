// Package nolegacylog defines a go/analysis pass that forbids the legacy
// log.InfoLog()/WarningLog()/ErrorLog()/DebugLog().Printf(...) logging API in
// a fixed set of files that have already been migrated to the structured
// log.Info/Warn/Error/Debug(msg string, args ...any) API (slog-style
// key/value args, log/log.go:681-692).
//
// Background: the legacy API returns a *log.Logger (stdlib-shaped) and
// writes a plain-text line — `[pid-...] INFO:2026/... file.go:333: [Tag]
// message key=val: %v` — that is NOT valid JSON, so `jq` silently skips it.
// The modern API writes a real JSON-lines record instead, filterable with
// `jq -r 'select(.item=="...")'` (see docs/how-to/debug-with-logs.md). We
// migrated server/services/backlog_service_triage.go,
// server/services/backlog_service_trigger_triage.go,
// session/backlog_lifecycle.go, and session/backlog_lifecycle_triage.go
// wholesale (2026-09-15) after a live incident where the only way to find a
// failed triage session's diagnostic output was a manual grep across the
// wrong log file. This analyzer exists so a future edit to one of those four
// files can't quietly reintroduce a legacy Printf call and regress that
// investigation story back to un-filterable text.
//
// Deliberately scoped to protectedFiles rather than repo-wide: the legacy
// Printf-style API is still the norm in many other, unmigrated files across
// this codebase, and an unconditional repo-wide ban would fail the build
// there. As more files are migrated, add their basename to protectedFiles.
package nolegacylog

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the nolegacylog check.
var Analyzer = &analysis.Analyzer{
	Name:     "nolegacylog",
	Doc:      "forbids log.InfoLog()/WarningLog()/ErrorLog()/DebugLog().Printf(...) (the legacy, non-JSON logging API) in files already migrated to the structured log.Info/Warn/Error/Debug(msg, \"key\", value, ...) API; use the modern API so lines stay jq-filterable, or add //nolint:nolegacylog with a justification",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// logPackagePath is the real import path of the package that owns both the
// legacy and modern logging APIs.
const logPackagePath = "github.com/tstapler/stapler-squad/log"

// legacyAccessorNames are the log.<Level>Log() functions whose returned
// *log.Logger's .Printf method is the forbidden call — see log/log.go:216-228.
var legacyAccessorNames = map[string]bool{
	"WarningLog": true,
	"InfoLog":    true,
	"ErrorLog":   true,
	"DebugLog":   true,
}

// protectedFiles are the basenames of files that have already been migrated
// off the legacy logging API and must stay that way. Matched by basename
// (not full path) — deliberately simple, since these are distinctively named
// files within a single codebase, not a public library where collisions are
// a real risk. Add an entry here whenever another file finishes the same
// migration.
var protectedFiles = map[string]bool{
	"backlog_service_triage.go":         true,
	"backlog_service_trigger_triage.go": true,
	"backlog_lifecycle.go":              true,
	"backlog_lifecycle_triage.go":       true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		if !isProtectedFile(pass, call.Pos()) {
			return
		}
		if !isLegacyPrintfCall(pass, call) {
			return
		}
		if nolintcomment.Contains(pass, call.Pos(), "nolegacylog") {
			return
		}
		pass.Reportf(call.Pos(),
			"legacy log.<Level>Log().Printf(...) call in a file already migrated to structured logging — use log.Info/Warn/Error/Debug(msg, \"key\", value, ...) instead so this line stays jq-filterable JSON (docs/how-to/debug-with-logs.md); add //nolint:nolegacylog with a justification if this genuinely can't use the modern API")
	})

	return nil, nil
}

// isProtectedFile reports whether pos falls in a file whose basename is in
// protectedFiles.
func isProtectedFile(pass *analysis.Pass, pos token.Pos) bool {
	file := pass.Fset.File(pos)
	if file == nil {
		return false
	}
	return protectedFiles[filepath.Base(file.Name())]
}

// isLegacyPrintfCall reports whether call is `<expr>.Printf(...)` where
// <expr> is itself a call to one of legacyAccessorNames resolved (via type
// info) to logPackagePath — i.e. log.InfoLog().Printf(...) and its siblings,
// however the log package is imported/aliased.
func isLegacyPrintfCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Printf" {
		return false
	}
	accessorCall, ok := sel.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	accessorSel, ok := accessorCall.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if !legacyAccessorNames[accessorSel.Sel.Name] {
		return false
	}
	obj, ok := pass.TypesInfo.Uses[accessorSel.Sel]
	if !ok {
		return false
	}
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil {
		return false
	}
	return fn.Pkg().Path() == logPackagePath
}
