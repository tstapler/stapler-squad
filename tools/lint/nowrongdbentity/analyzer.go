// Package nowrongdbentity defines a go/analysis pass that stops code from
// reaching an entity that lives in another database through an *ent.Client
// (ADR-036).
//
// Every ent client has a field for every entity, so the type system cannot say
// which file a table is in. This analyzer is the ratchet, not a proof: it
// encodes the ownership by package.
//
//   - sessions.db code (package session and its subpackages) must not select
//     entities that live in analytics.db: EscapeEvent, AnalyticsEvent.
//   - package server/analytics, which owns analytics.db, may select only those
//     entities.
//
// The motivating bug: session.EntRepository.ListAnalyticsBySessionID read
// AnalyticsEvent through the sessions.db client, whose analytics_events table is
// empty; the rows are in analytics.db. A deliberate exception needs
// //nolint:nowrongdbentity with the reason.
package nowrongdbentity

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"

	"github.com/tstapler/stapler-squad/tools/lint/internal/nolintcomment"
)

// Analyzer is the exported analysis.Analyzer for the nowrongdbentity check.
var Analyzer = &analysis.Analyzer{
	Name:     "nowrongdbentity",
	Doc:      "rejects selecting an ent entity that lives in another database (sessions.db code touching analytics.db entities, and the reverse)",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer},
}

// escapeEntities live in analytics.db. Hand-maintained until per-DB table subsets exist (ADR-036 PR 2 adds the registry-equals-analyzer check).
var escapeEntities = map[string]bool{"EscapeEvent": true, "AnalyticsEvent": true}

type side int

const (
	sideNone   side = iota
	sideCore        // sessions.db code
	sideEscape      // analytics.db owner
)

func sideOf(pkgPath string) side {
	switch {
	case strings.HasSuffix(pkgPath, "/server/analytics"):
		return sideEscape
	case strings.HasSuffix(pkgPath, "/session/dbpool"), strings.HasSuffix(pkgPath, "/session/ent"), strings.Contains(pkgPath, "/session/ent/"):
		return sideNone
	case strings.HasSuffix(pkgPath, "/session"), strings.Contains(pkgPath, "/session/"):
		return sideCore
	}
	return sideNone
}

func run(pass *analysis.Pass) (interface{}, error) {
	s := sideOf(pass.Pkg.Path())
	if s == sideNone {
		return nil, nil
	}
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.SelectorExpr)(nil)}, func(n ast.Node) {
		sel := n.(*ast.SelectorExpr)
		entity, ok := entityOnClient(pass, sel)
		if !ok {
			return
		}
		if strings.HasSuffix(pass.Fset.Position(sel.Pos()).Filename, "_test.go") {
			return
		}
		wrong := (s == sideCore && escapeEntities[entity]) || (s == sideEscape && !escapeEntities[entity])
		if !wrong {
			return
		}
		switch nolintcomment.Lookup(pass, sel.Pos(), "nowrongdbentity") {
		case nolintcomment.OK:
			return
		case nolintcomment.Missing:
			pass.Reportf(sel.Pos(), "//nolint:nowrongdbentity needs a one-line reason")
			return
		}
		if s == sideCore {
			pass.Reportf(sel.Pos(), "entity %s lives in analytics.db but this code holds the sessions.db client; use the analytics client from server/analytics (or //nolint:nowrongdbentity with a reason)", entity)
			return
		}
		pass.Reportf(sel.Pos(), "entity %s lives in sessions.db but server/analytics owns analytics.db; this client has no such table in use (or //nolint:nowrongdbentity with a reason)", entity)
	})
	return nil, nil
}

// entityOnClient reports the entity name when sel is `<*ent.Client>.<Entity>` or
// `<*ent.Tx>.<Entity>`, recognised as a struct field of type *<Entity>Client on a
// type named Client or Tx in a package whose path ends in /session/ent.
func entityOnClient(pass *analysis.Pass, sel *ast.SelectorExpr) (string, bool) {
	obj, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Var)
	if !ok || !obj.IsField() {
		return "", false
	}
	tv, ok := pass.TypesInfo.Types[sel.X]
	if !ok {
		return "", false
	}
	t := tv.Type
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || (named.Obj().Name() != "Client" && named.Obj().Name() != "Tx") || named.Obj().Pkg() == nil || !strings.HasSuffix(named.Obj().Pkg().Path(), "/session/ent") {
		return "", false
	}
	ptr, ok := obj.Type().(*types.Pointer)
	if !ok {
		return "", false
	}
	fieldType, ok := ptr.Elem().(*types.Named)
	if !ok || fieldType.Obj().Name() != sel.Sel.Name+"Client" {
		return "", false
	}
	return sel.Sel.Name, true
}
