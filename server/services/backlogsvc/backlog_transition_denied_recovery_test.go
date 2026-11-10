package backlogsvc

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

// --- Story 1.5.8: spawn points park behind an open denial --------------------------

// --- completeness: remediation reasons and the shared guard ----------------------

// denRemediationReasons is every reason remediationActionByReason maps, with the
// spawn point it reaches; a new mapping fails the test until it is classified.
var denRemediationReasons = map[string]string{
	"StuckReasonBouncing":        "AutoReopenAfterFailedReview",
	"StuckReasonAbandonedReview": "AutoRespawnReview",
	"StuckReasonAutonomousStuck": "AutoRespawnAutonomousWork",
	"StuckReasonStaleWork":       "RemediateStaleWorkSession",
	"StuckReasonOrphanedTriage":  "AutoRespawnTriage",
	"StuckReasonPRPendingNoPR":   "AutoReopenForPRFix",
	"StuckReasonPRNeedsFix":      "AutoReopenForPRFix",
}

func denParse(t *testing.T, file string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	return parsed
}

func TestEveryRemediationReasonIsClassified(t *testing.T) {
	var got []string
	for _, decl := range denParse(t, "backlog_service_stuck.go").Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "remediationActionByReason" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					if sel, ok := e.(*ast.SelectorExpr); ok {
						got = append(got, sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
	want := make([]string, 0, len(denRemediationReasons))
	for reason := range denRemediationReasons {
		want = append(want, reason)
	}
	sort.Strings(got)
	sort.Strings(want)
	assert.Equal(t, want, got)
}

// denFuncsCalling lists the BacklogService methods (and plain functions) in this
// package's non-test files whose body calls a method named one of names.
func denFuncsCalling(t *testing.T, names ...string) map[string]bool {
	t.Helper()
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		for _, decl := range denParse(t, f).Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && want[sel.Sel.Name] {
						found[fn.Name.Name+"->"+sel.Sel.Name] = true
					}
				}
				return true
			})
		}
	}
	return found
}

func TestSpawnPointGuards_should_ShareOnePredicateAndCoverEveryRemediationAction(t *testing.T) {
	predicate := denFuncsCalling(t, "HasOpenTransitionDenied", "OpenTransitionDenial", "TransitionDenialHolds")
	assert.Equal(t, map[string]bool{
		"heldByTransitionDenial->TransitionDenialHolds":    true,
		"transitionDenialHoldError->OpenTransitionDenial":  true,
		"transitionDenialHoldError->TransitionDenialHolds": true,
	}, predicate, "the predicate is reached only through the two guard helpers")

	guarded := denFuncsCalling(t, "heldByTransitionDenial", "triageHeldByDenial", "transitionDenialHoldError")
	spawnPoints := map[string]string{ // function -> the helper it must call
		"AutoReopenAfterFailedReview": "heldByTransitionDenial",
		"AutoRespawnReview":           "heldByTransitionDenial",
		"AutoRespawnAutonomousWork":   "heldByTransitionDenial",
		"RemediateStaleWorkSession":   "heldByTransitionDenial",
		"respawnTriage":               "heldByTransitionDenial",
		"autoReopenForPRFix":          "heldByTransitionDenial",
		"DequeueNextQueuedItems":      "heldByTransitionDenial",
		"triageHeldByDenial":          "heldByTransitionDenial",
		"MaybeTriggerTriage":          "triageHeldByDenial",
		"TriggerRemediationNow":       "transitionDenialHoldError",
	}
	for fn, helper := range spawnPoints {
		assert.Truef(t, guarded[fn+"->"+helper], "%s must call %s", fn, helper)
	}
}

// --- end to end: gate + real reporter + sweep --------------------------------------
