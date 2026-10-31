// Package nowrongdbentity defines a go/analysis pass that stops code from
// reaching an entity that lives in another database through an *ent.Client
// (ADR-036).
//
// Every ent client has a field for every entity, so the type system cannot say
// which file a table is in. This analyzer is the ratchet, not a proof: it
// encodes the ownership by package.
//
//   - sessions.db code (package session and its subpackages) must not select
//     entities that live elsewhere: AnalyticsEvent, EscapeEvent.
//   - package server/analytics, which owns analytics.db, may select only
//     AnalyticsEvent.
//   - package server/analytics/escapedb, which owns db/escape_events.db, may
//     select only EscapeEvent.
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

// entityOwner maps each entity that lives outside sessions.db to the database
// that holds it. Hand-maintained until per-DB table subsets exist (ADR-036 PR 2
// adds the registry-equals-analyzer check).
var entityOwner = map[string]owner{
	"AnalyticsEvent": ownerAnalytics,
	"EscapeEvent":    ownerEscapeEvents,
}

type owner int

const (
	ownerCore         owner = iota // sessions.db: every entity not in entityOwner
	ownerAnalytics                 // analytics.db (server/analytics)
	ownerEscapeEvents              // db/escape_events.db (server/analytics/escapedb)
)

func (o owner) String() string {
	switch o {
	case ownerAnalytics:
		return "analytics.db"
	case ownerEscapeEvents:
		return "db/escape_events.db"
	}
	return "sessions.db"
}

// packageOwner returns the database a package's client points at, and false for
// packages the check does not cover.
func packageOwner(pkgPath string) (owner, bool) {
	switch {
	case strings.HasSuffix(pkgPath, "/server/analytics/escapedb"):
		return ownerEscapeEvents, true
	case strings.HasSuffix(pkgPath, "/server/analytics"):
		return ownerAnalytics, true
	case strings.HasSuffix(pkgPath, "/session/dbpool"), strings.HasSuffix(pkgPath, "/session/ent"), strings.Contains(pkgPath, "/session/ent/"):
		return ownerCore, false
	case strings.HasSuffix(pkgPath, "/session"), strings.Contains(pkgPath, "/session/"):
		return ownerCore, true
	}
	return ownerCore, false
}

func entityOwnerOf(entity string) owner { return entityOwner[entity] }

func run(pass *analysis.Pass) (interface{}, error) {
	pkgDB, covered := packageOwner(pass.Pkg.Path())
	if !covered {
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
		entityDB := entityOwnerOf(entity)
		if entityDB == pkgDB {
			return
		}
		switch nolintcomment.Lookup(pass, sel.Pos(), "nowrongdbentity") {
		case nolintcomment.OK:
			return
		case nolintcomment.Missing:
			pass.Reportf(sel.Pos(), "//nolint:nowrongdbentity needs a one-line reason")
			return
		}
		pass.Reportf(sel.Pos(), "entity %s lives in %s but this package holds the %s client; use the client from the package that owns that database (or //nolint:nowrongdbentity with a reason)", entity, entityDB, pkgDB)
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
