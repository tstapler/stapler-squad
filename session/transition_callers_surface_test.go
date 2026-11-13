package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// A caller that moves an item into review must say how a human learns of it:
// "swallowed status transition" (an escalation with only a log line) recurred
// because nothing forced the choice. See docs/tasks/backlog-feature-improvement.md.
func TestTransitionCallerAudit_EveryAgentAndSystemCallerEnteringReviewDeclaresASurface(t *testing.T) {
	t.Parallel()
	for _, c := range domain.TransitionCallers() {
		if c.Forwarding || c.Class == domain.ClassOperator {
			continue
		}
		entersReview := false
		for _, e := range c.Edges {
			entersReview = entersReview || e.To == domain.BacklogStatusReview
		}
		if !entersReview {
			require.Emptyf(t, c.Surface, "%s never enters review, so it must not declare a surface", c.Name)
			continue
		}
		require.Containsf(t, []domain.SurfacePolicy{domain.SurfaceReviewFlow, domain.SurfaceNotifies}, c.Surface,
			"%s enters review without declaring how a human learns of it (set Surface in transition_callers.go)", c.Name)
	}
}

func TestTransitionCallerAudit_SurfaceNotifiesSitesActuallyPublishANotification(t *testing.T) {
	t.Parallel()
	checked := 0
	for _, c := range domain.TransitionCallers() {
		if c.Surface != domain.SurfaceNotifies {
			continue
		}
		for _, s := range c.Sites {
			checked++
			require.Truef(t, funcCallsNotifier(t, filepath.Join("..", s.File), s.Func),
				"%s declares SurfaceNotifies but %s in %s calls no notify* function or NewNotificationEvent", c.Name, s.Func, s.File)
		}
	}
	require.Positive(t, checked, "no SurfaceNotifies caller found; the declaration or this check is broken")
}

// funcCallsNotifier reports whether the "Recv.Name" function in file calls
// NewNotificationEvent or a method/function whose name starts with "notify".
func funcCallsNotifier(t *testing.T, file, qualified string) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	name := qualified[strings.LastIndex(qualified, ".")+1:]
	found, notifies := false, false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || fn.Body == nil {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var callee string
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				callee = fun.Sel.Name
			case *ast.Ident:
				callee = fun.Name
			}
			notifies = notifies || callee == "NewNotificationEvent" || strings.HasPrefix(callee, "notify")
			return true
		})
	}
	require.Truef(t, found, "function %s not found in %s", qualified, file)
	return notifies
}
