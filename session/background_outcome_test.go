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
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func TestComputeBackgroundOutcome(t *testing.T) {
	t.Parallel()
	verdict := &backlogdto.ReviewVerdictSummary{OverallOutcome: "PASS"}
	tests := []struct {
		name   string
		status domain.Status
		is     *backlogdto.ItemSessionSummary
		want   BackgroundOutcome
	}{
		{"ComputeBackgroundOutcome_should_returnCompleted_When_reviewVerdictRecorded", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview, ReviewVerdict: verdict}, BackgroundOutcomeCompleted},
		{"completed_When_triageResultRecorded", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleTriage, TriageResult: `{"summary":"x"}`}, BackgroundOutcomeCompleted},
		{"ComputeBackgroundOutcome_should_returnFailed_When_statusCrashed", domain.Crashed,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview}, BackgroundOutcomeFailed},
		{"failed_When_crashedAndNoItemSession", domain.Crashed, nil, BackgroundOutcomeFailed},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_endReasonShutdown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview, EndReason: "shutdown"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_diagnoseRole", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleDiagnose}, BackgroundOutcomeUnknown},
		{"unknown_When_diagnoseRoleEvenWithFailureReason", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleDiagnose, EndReason: "timeout"}, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_returnUnknown_When_noItemSession", domain.Stopped, nil, BackgroundOutcomeUnknown},
		{"ComputeBackgroundOutcome_should_treatTimeoutAsFailed", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview, EndReason: "timeout"}, BackgroundOutcomeFailed},
		{"_JulesCompletedAsUnknown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleJulesWork, EndReason: "jules_completed"}, BackgroundOutcomeUnknown},
		{"_UnlistedReasonAsUnknown", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview, EndReason: "some_future_reason"}, BackgroundOutcomeUnknown},
		{"_VerdictBeatsFailureReason", domain.Stopped,
			&backlogdto.ItemSessionSummary{Role: SessionRoleReview, ReviewVerdict: verdict, EndReason: "timeout"}, BackgroundOutcomeCompleted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ComputeBackgroundOutcome(tc.status, tc.is))
		})
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
			case "gen", "node_modules", "ent", ".git", ".claude", "web-app", "dist": // .claude holds gitignored agent worktree checkouts
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
		_, fail := FailureEndReasons[r]
		_, non := NonFailureEndReasons[r]
		assert.Truef(t, fail != non, "literal end_reason %q (%s) must be classified exactly once", r, path)
	}
}

func TestListItemSessionsBySessionUUIDs_should_returnNewestPerUUID_And_loadVerdict(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "batch"})
	require.NoError(t, err)

	review, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "u-review", SessionRole: SessionRoleReview})
	require.NoError(t, err)
	require.NoError(t, storage.Repo.SaveReviewVerdict(ctx, review.ID, backlogdto.ReviewVerdictData{OverallOutcome: "PASS"}))
	_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "u-other", SessionRole: SessionRoleDiagnose})
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
