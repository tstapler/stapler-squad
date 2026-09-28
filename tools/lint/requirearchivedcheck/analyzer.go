// Package requirearchivedcheck defines a go/analysis pass, scoped only to
// files under server/services/diagnose_*.go or session/diagnose/*.go, that
// flags a function calling a session archive/kill/revive primitive
// (ArchiveSessionByUUID, KillTmuxPaneOnly, or ResumeSession) without a
// same-function, textually preceding call to .IsArchived() on the same
// receiver expression that the primitive call's own receiver or a
// SessionUUID-style argument is rooted in.
//
// Background: ADR-001 (superseded-rework-session-retirement) requires that
// archived sessions never be auto-started, auto-revived, or auto-retried
// (session/instance_state.go's IsArchived doc comment). The repo already has
// 8 call sites that perform an archive/kill/revive without that guard — a
// pre-existing bypass pool the backlog-diagnose-and-nudge plan's Tech Debt
// Disposition table deliberately leaves out of scope for a repo-wide
// retrofit. This analyzer instead ratchets the check for this feature's own
// new automated-lifecycle code only, so the diagnose/nudge feature can't
// silently add a 9th bypass — the same narrow-scope precedent as
// norawghrequest (see tools/lint/norawghrequest), which polices its own
// GitHub-host request-construction rule for one package rather than
// retrofitting every existing caller.
package requirearchivedcheck

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// analyzerName is the linter name used both for Analyzer.Name and for
// matching //nolint:requirearchivedcheck directives — kept as a constant
// (rather than reading Analyzer.Name back) to avoid an initialization cycle
// between Analyzer and checkFunc.
const analyzerName = "requirearchivedcheck"

// Analyzer is the exported analysis.Analyzer for the requirearchivedcheck check.
var Analyzer = &analysis.Analyzer{
	Name:     analyzerName,
	Doc:      "in server/services/diagnose_*.go and session/diagnose/*.go only: flags a function calling ArchiveSessionByUUID/KillTmuxPaneOnly/ResumeSession without a preceding same-function .IsArchived() check on the same receiver — add the check, or //nolint:requirearchivedcheck with a justification",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// guardedPrimitiveNames are the session archive/kill/revive primitives that
// must not be called without a preceding IsArchived() guard in the same
// function. Matched by name only (not by resolved package/type) because this
// analyzer's scope is the two file-path prefixes below, not a specific
// package — unlike norawghrequest, which polices a single real package and
// so can afford (and needs) type-resolved package matching.
var guardedPrimitiveNames = map[string]bool{
	"ArchiveSessionByUUID": true,
	"KillTmuxPaneOnly":     true,
	"ResumeSession":        true,
}

// isArchivedMethodName is the guard method a preceding call must invoke.
const isArchivedMethodName = "IsArchived"

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.FuncLit)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		body := funcBody(n)
		if body == nil {
			return
		}
		if !isMatchedFile(pass.Fset.Position(n.Pos()).Filename) {
			return
		}
		checkFunc(pass, body)
	})

	return nil, nil
}

func funcBody(n ast.Node) *ast.BlockStmt {
	switch fn := n.(type) {
	case *ast.FuncDecl:
		return fn.Body
	case *ast.FuncLit:
		return fn.Body
	}
	return nil
}

// diagnoseDirSuffix and servicesDirSuffix are OS-slash-independent (matched
// against a filepath.ToSlash'd directory) so the analyzer behaves the same
// whether Fset reports paths with '/' or '\'.
const (
	diagnoseDirSuffix  = "session/diagnose"
	servicesDirSuffix  = "server/services"
	servicesFilePrefix = "diagnose_"
)

// isMatchedFile reports whether filename falls under one of the two path
// prefixes this analyzer is scoped to: session/diagnose/*.go, or
// server/services/diagnose_*.go. Deliberately excludes every other file
// (e.g. server/services/superseded_session_sweeper.go, which has a
// structurally identical unguarded-archive pattern) — that's the point: the
// plan's Tech Debt Disposition table treats those as pre-existing, explicitly
// out-of-scope debt, not a regression this analyzer should flag.
func isMatchedFile(filename string) bool {
	if filename == "" {
		return false
	}
	slashPath := filepath.ToSlash(filename)
	dir := filepath.ToSlash(filepath.Dir(slashPath))
	base := filepath.Base(slashPath)

	if dir == diagnoseDirSuffix || strings.HasSuffix(dir, "/"+diagnoseDirSuffix) {
		return true
	}
	if dir == servicesDirSuffix || strings.HasSuffix(dir, "/"+servicesDirSuffix) {
		return strings.HasPrefix(base, servicesFilePrefix) && strings.HasSuffix(base, ".go")
	}
	return false
}

// checkFunc walks body's statements in source order, tracking which
// receiver objects (types.Object of the root identifier of an expression
// like inst in inst.IsArchived() or inst.SessionUUID) have been verified via
// a preceding IsArchived() call, and reporting any guardedPrimitiveNames call
// whose receiver — or a SessionUUID-shaped argument rooted in the same
// receiver — isn't yet covered.
func checkFunc(pass *analysis.Pass, body *ast.BlockStmt) {
	checked := map[types.Object]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if sel.Sel.Name == isArchivedMethodName {
			if obj := rootObject(pass, sel.X); obj != nil {
				checked[obj] = true
			}
			return true
		}

		if !guardedPrimitiveNames[sel.Sel.Name] {
			return true
		}
		if nolintcomment.Contains(pass, call.Pos(), analyzerName) {
			return true
		}
		if isGuarded(pass, call, sel, checked) {
			return true
		}
		pass.Reportf(call.Pos(),
			"%s called without a preceding same-function .IsArchived() check on the same session — archived sessions must never be auto-started/auto-revived/auto-retried (ADR-001); add the check, or //nolint:requirearchivedcheck with a justification",
			sel.Sel.Name)
		return true
	})
}

// isGuarded reports whether call — a resolved guardedPrimitiveNames call with
// selector sel — is covered by an IsArchived() check already recorded in
// checked, either via the call's own receiver (sel.X) or via a same-rooted
// SessionUUID-shaped argument (e.g. ArchiveSessionByUUID(ctx, inst.SessionUUID)
// guarded by an earlier inst.IsArchived()).
func isGuarded(pass *analysis.Pass, call *ast.CallExpr, sel *ast.SelectorExpr, checked map[types.Object]bool) bool {
	if obj := rootObject(pass, sel.X); obj != nil && checked[obj] {
		return true
	}
	for _, arg := range call.Args {
		argSel, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if obj := rootObject(pass, argSel.X); obj != nil && checked[obj] {
			return true
		}
	}
	return false
}

// rootObject resolves expr down to its root *ast.Ident (unwrapping
// SelectorExpr chains like a.b.c down to a) and returns the types.Object it
// refers to, so two different expressions of the same underlying variable
// (inst.IsArchived() and inst.SessionUUID) can be recognized as the same
// receiver.
func rootObject(pass *analysis.Pass, expr ast.Expr) types.Object {
	for {
		switch e := expr.(type) {
		case *ast.SelectorExpr:
			expr = e.X
		case *ast.ParenExpr:
			expr = e.X
		case *ast.StarExpr:
			expr = e.X
		case *ast.Ident:
			return pass.TypesInfo.Uses[e]
		default:
			return nil
		}
	}
}
