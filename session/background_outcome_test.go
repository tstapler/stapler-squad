package session

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBackgroundOutcome(t *testing.T) {
	t.Parallel()
	verdict := &ReviewVerdictSummary{OverallOutcome: "PASS"}
	tests := []struct {
		name   string
		status Status
		is     *ItemSessionSummary
		want   BackgroundOutcome
	}{
		{"ComputeBackgroundOutcome_should_returnCompleted_When_reviewVerdictRecorded", Stopped,
			&ItemSessionSummary{Role: SessionRoleReview, ReviewVerdict: verdict}, BackgroundOutcomeCompleted},
		{"completed_When_triageResultRecorded", Stopped,
			&ItemSessionSummary{Role: SessionRoleTriage, TriageResult: `{"summary":"x"}`}, BackgroundOutcomeCompleted},
		{"ComputeBackgroundOutcome_should_returnFailed_When_statusCrashed", Crashed,
			&ItemSessionSummary{Role: SessionRoleReview}, BackgroundOutcomeFailed},
		{"failed_When_crashedAndNoItemSession", Crashed, nil, BackgroundOutcomeFailed},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_endReasonShutdown", Stopped,
			&ItemSessionSummary{Role: SessionRoleReview, EndReason: "shutdown"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_diagnoseRole", Stopped,
			&ItemSessionSummary{Role: SessionRoleDiagnose}, BackgroundOutcomeUnknown},
		{"unknown_When_diagnoseRoleEvenWithFailureReason", Stopped,
			&ItemSessionSummary{Role: SessionRoleDiagnose, EndReason: "timeout"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_noItemSession", Stopped, nil, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_treatTimeoutAsFailed", Stopped,
			&ItemSessionSummary{Role: SessionRoleReview, EndReason: "timeout"}, BackgroundOutcomeFailed},
		{"_JulesCompletedAsUnknown", Stopped,
			&ItemSessionSummary{Role: SessionRoleJulesWork, EndReason: "jules_completed"}, BackgroundOutcomeUnknown},
		{"_UnlistedReasonAsUnknown", Stopped,
			&ItemSessionSummary{Role: SessionRoleReview, EndReason: "some_future_reason"}, BackgroundOutcomeUnknown},
		{"_VerdictBeatsFailureReason", Stopped,
			&ItemSessionSummary{Role: SessionRoleReview, ReviewVerdict: verdict, EndReason: "timeout"}, BackgroundOutcomeCompleted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ComputeBackgroundOutcome(tc.status, tc.is))
		})
	}
}

// knownEndReasonProducers lists every end_reason value passed to
// UpdateItemSessionEndedWithReason / UpdateItemSessionEndedWithDetail
// (directly or via failJulesSession / classifyHeadlessCallError /
// stopGuardrailSession), found with:
//
//	git grep -n "UpdateItemSessionEndedWith\|failJulesSession(" -- '*.go' ':!*_test.go'
var knownEndReasonProducers = []string{
	"", "shutdown", "pool_saturated", "idle", "timeout", "claude_not_found",
	"subprocess_start_error", TriageEndReasonFanoutCeiling, TriageEndReasonCostCeiling, TriageEndReasonOther,
	"parse_error", "dispatch_failed", "dispatch_incomplete", "jules_timed_out",
	"jules_session_missing", "jules_failed", "jules_completed", "jules_completed_no_pr",
	"cost_budget_exceeded", "idle_wait_loop_escalation",
}

func TestEndReasonClassification_ShouldCoverEveryKnownProducer(t *testing.T) {
	t.Parallel()
	for _, r := range knownEndReasonProducers {
		_, fail := failureEndReasons[r]
		_, non := nonFailureEndReasons[r]
		assert.Truef(t, fail != non, "end_reason %q must be in exactly one of the failure/non-failure maps", r)
	}
	for r := range failureEndReasons {
		assert.NotContains(t, nonFailureEndReasons, r)
	}
}

// Scans non-test sources so a new string-literal producer fails here until it is classified.
func TestEndReasonClassification_ShouldCoverEveryLiteralInSource(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..")
	reasonArg := map[string]int{"UpdateItemSessionEndedWithReason": 3, "UpdateItemSessionEndedWithDetail": 3, "failJulesSession": 3}
	found := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "gen", "node_modules", "ent", ".git", "web-app", "dist":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			case *ast.Ident:
				name = fn.Name
			}
			idx, ok := reasonArg[name]
			if !ok || idx >= len(call.Args) {
				return true
			}
			if lit, ok := call.Args[idx].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				found[v] = path
			}
			return true
		})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, found, "scan found no producers; the walker is broken")
	for r, path := range found {
		_, fail := failureEndReasons[r]
		_, non := nonFailureEndReasons[r]
		assert.Truef(t, fail != non, "literal end_reason %q (%s) must be classified exactly once", r, path)
	}
}

func TestListItemSessionsBySessionUUIDs_should_returnNewestPerUUID_And_loadVerdict(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "batch"})
	require.NoError(t, err)

	review, err := storage.CreateItemSession(ctx, ItemSessionData{ItemID: item.ID, SessionUUID: "u-review", SessionRole: SessionRoleReview})
	require.NoError(t, err)
	require.NoError(t, storage.repo.SaveReviewVerdict(ctx, review.ID, ReviewVerdictData{OverallOutcome: "PASS"}))
	_, err = storage.CreateItemSession(ctx, ItemSessionData{ItemID: item.ID, SessionUUID: "u-other", SessionRole: SessionRoleDiagnose})
	require.NoError(t, err)

	got, err := storage.ListItemSessionsBySessionUUIDs(ctx, []string{"u-review", "u-other", "u-missing"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.NotNil(t, got["u-review"].ReviewVerdict)
	assert.NotContains(t, got, "u-missing")

	empty, err := storage.ListItemSessionsBySessionUUIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
