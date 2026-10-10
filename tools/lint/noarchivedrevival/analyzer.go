// Package noarchivedrevival defines a go/analysis pass that requires every
// automated start/revive/retry call in the session-lifecycle file set to be
// preceded, within its function, by an archival check (IsArchived() or an
// ArchivedAt read).
//
// Background: PR #810 found six auto-revival sites that never read ArchivedAt, so
// superseded rework sessions resurrected themselves; the site count grew 2 → 4 →
// 6 → 7 across review rounds because nothing flagged a missed one. See
// .claude/rules/noarchivedrevival.md.
//
// "Guarded" approximates dominance without a CFG: an archival check must appear
// earlier in a statement list that encloses the call, or in every in-package
// caller chain of an unexported function. Name-based, so same-named methods on
// different types share call sites.
package noarchivedrevival

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const name = "noarchivedrevival"

// Analyzer is the exported analysis.Analyzer for the noarchivedrevival check.
var Analyzer = &analysis.Analyzer{
	Name: name,
	Doc:  "requires Start(false)/RecoverFromStopped()/restartForRetry/transitionToLocked(..., Active) in the automated-lifecycle files to follow an IsArchived()/ArchivedAt check, or carry //nolint:noarchivedrevival <reason>",
	Run:  run,
}

// lifecycleFiles are the path suffixes of files holding automated (non-user)
// revival paths. Explicit user-action files (instance_hibernate.go,
// instance_crash.go, import_commit.go, retry_state.go, MCP hydration) are
// deliberately absent.
var lifecycleFiles = []string{
	"session/health.go",
	"session/review_queue_poller.go",
	"session/session_driver.go",
	"session/instance_claude.go",
	"session/instance_serialization.go",
	"server/dependencies.go",
}

func isLifecycleFile(filename string) bool {
	filename = strings.ReplaceAll(filename, "\\", "/")
	for _, s := range lifecycleFiles {
		if strings.HasSuffix(filename, "/"+s) || filename == s {
			return true
		}
	}
	return false
}

// archivalHelpers returns names of predicate-shaped functions (last result is
// bool) whose body itself performs an archival check, e.g. a skip-reason helper
// that reads ArchivedAt; a call to one counts as an archival check. IsArchived is
// always included. Derived from bodies so a helper that does not check archival
// (health.go's healthCheckSkipReason before PR #810) is not trusted.
func archivalHelpers(files []*ast.File) map[string]bool {
	h := map[string]bool{"IsArchived": true}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Type.Results == nil || len(fd.Type.Results.List) == 0 {
				continue
			}
			last := fd.Type.Results.List[len(fd.Type.Results.List)-1].Type
			if !isIdent(last, "bool") {
				continue
			}
			if len(collectGuards(fd.Body, h)) > 0 {
				h[fd.Name.Name] = true
			}
		}
	}
	return h
}

// guard is an archival check and the statement list it dominates: any later
// position inside [pos, scopeEnd).
type guard struct{ pos, scopeEnd token.Pos }

// fnInfo is one top-level function with its guards.
type fnInfo struct {
	decl   *ast.FuncDecl
	file   *ast.File
	guards []guard
}

func (fi *fnInfo) guarded(pos token.Pos) bool {
	for _, g := range fi.guards {
		if g.pos < pos && pos < g.scopeEnd {
			return true
		}
	}
	return false
}

type site struct {
	caller *fnInfo
	pos    token.Pos
}

func run(pass *analysis.Pass) (interface{}, error) {
	helpers := archivalHelpers(pass.Files)
	var fns []*fnInfo
	sites := map[string][]site{} // callee simple name -> call sites (non-test files)
	byName := map[string][]*fnInfo{}
	for _, f := range pass.Files {
		filename := pass.Fset.Position(f.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fi := &fnInfo{decl: fd, file: f, guards: collectGuards(fd.Body, helpers)}
			fns = append(fns, fi)
			byName[fd.Name.Name] = append(byName[fd.Name.Name], fi)
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if name := calleeName(call); name != "" {
						sites[name] = append(sites[name], site{fi, call.Pos()})
					}
				}
				return true
			})
		}
	}

	// covered(fn): fn is unexported, has callers, and every call site is guarded
	// in its caller or the caller is itself covered. Handles guards that live in
	// an outer function (health.go's checkSingleSession -> recoverMissingSession).
	memo := map[*fnInfo]bool{}
	visiting := map[*fnInfo]bool{}
	var covered func(fi *fnInfo) bool
	covered = func(fi *fnInfo) bool {
		if v, ok := memo[fi]; ok {
			return v
		}
		name := fi.decl.Name.Name
		if ast.IsExported(name) || visiting[fi] || len(sites[name]) == 0 {
			return false
		}
		visiting[fi] = true
		ok := true
		for _, s := range sites[name] {
			if !s.caller.guarded(s.pos) && !covered(s.caller) {
				ok = false
				break
			}
		}
		visiting[fi] = false
		memo[fi] = ok
		return ok
	}

	for _, fi := range fns {
		if !isLifecycleFile(pass.Fset.Position(fi.file.Pos()).Filename) {
			continue
		}
		ast.Inspect(fi.decl.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			what, ok := revivalCall(call)
			if !ok || fi.guarded(call.Pos()) || covered(fi) {
				return true
			}
			switch nolintFor(pass, fi.file, call.Pos()) {
			case nolintOK:
			case nolintNoReason:
				pass.Reportf(call.Pos(), "//nolint:%s requires a one-line justification, e.g. //nolint:%s explicit user resume of an archived session", name, name)
			default:
				pass.Reportf(call.Pos(), "%s revives a session without a preceding IsArchived()/ArchivedAt check; archived sessions must never be revived automatically (see .claude/rules/noarchivedrevival.md). Guard it, or add //nolint:%s <reason>", what, name)
			}
			return true
		})
	}
	return nil, nil
}

// collectGuards finds archival checks in body, each scoped to the innermost
// statement list that contains it — a check in an unrelated nested block does
// not guard code outside that block.
func collectGuards(body *ast.BlockStmt, helpers map[string]bool) []guard {
	var out []guard
	var scopes []ast.Node // enclosing statement-list containers
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		ast.Inspect(n, func(c ast.Node) bool {
			if c == nil {
				return false
			}
			switch x := c.(type) {
			case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
				if c != n {
					scopes = append(scopes, c)
					walk(c)
					scopes = scopes[:len(scopes)-1]
					return false
				}
			case *ast.CallExpr:
				if name := calleeName(x); helpers[name] {
					out = append(out, guard{x.Pos(), scopes[len(scopes)-1].End()})
				}
			case *ast.BinaryExpr:
				// Only a nil comparison is a check; copying the field (e.g.
				// `ArchivedAt: data.ArchivedAt`) is not.
				if (x.Op == token.EQL || x.Op == token.NEQ) && (isArchivedAt(x.X) && isIdent(x.Y, "nil") || isArchivedAt(x.Y) && isIdent(x.X, "nil")) {
					out = append(out, guard{x.Pos(), scopes[len(scopes)-1].End()})
				}
			}
			return true
		})
	}
	scopes = append(scopes, body)
	walk(body)
	return out
}

// calleeName returns the simple name of a call's target (f(...) or x.f(...)).
func calleeName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	}
	return ""
}

// revivalCall reports whether call is one of the revival entry points.
func revivalCall(call *ast.CallExpr) (string, bool) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		switch fun.Sel.Name {
		case "Start":
			if len(call.Args) == 1 && isIdent(call.Args[0], "false") {
				return "Start(false)", true
			}
		case "RecoverFromStopped":
			return "RecoverFromStopped()", true
		}
	case *ast.Ident:
		switch fun.Name {
		case "restartForRetry":
			return "restartForRetry()", true
		case "transitionToLocked":
			if len(call.Args) == 3 && isIdent(call.Args[2], "Active") {
				return "transitionToLocked(..., Active)", true
			}
		}
	}
	return "", false
}

func isArchivedAt(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "ArchivedAt"
}

func isIdent(e ast.Expr, n string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == n
}

type nolintResult int

const (
	nolintNone nolintResult = iota
	nolintNoReason
	nolintOK
)

// nolintFor looks for //nolint:<list> <reason> on pos's line or the line above.
func nolintFor(pass *analysis.Pass, f *ast.File, pos token.Pos) nolintResult {
	line := pass.Fset.Position(pos).Line
	res := nolintNone
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			cl := pass.Fset.Position(c.Pos()).Line
			if cl != line && cl != line-1 {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, "nolint:") {
				continue
			}
			list, reason, _ := strings.Cut(strings.TrimPrefix(text, "nolint:"), " ")
			if !strings.Contains(list, name) {
				continue
			}
			if strings.TrimSpace(reason) == "" {
				res = nolintNoReason
			} else {
				return nolintOK
			}
		}
	}
	return res
}
