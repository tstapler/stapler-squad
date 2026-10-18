package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
)

func TestUniqueFailingTestClasses(t *testing.T) {
	// kibitzer:ignore replace-magic-literal -- realistic sample gh log lines (incl. an intentional duplicate, to assert dedup), not magic values
	log := "OutlinerRegressionTest[jvm] > testBasicOutlinerFlow[jvm] FAILED\n" +
		"OutlinerRegressionTest[jvm] > testBasicOutlinerFlow[jvm] FAILED\n" +
		"FooTest[jvm] > bar[jvm] FAILED\n" +
		"BazTest[jvm] > qux[jvm] PASSED\n"

	got := uniqueFailingTestClasses(log)

	require.Equal(t, []string{"FooTest", "OutlinerRegressionTest"}, got)
}

func TestUniqueFailingTestClasses_NoMatches(t *testing.T) {
	require.Empty(t, uniqueFailingTestClasses("BUILD FAILED\nsome unrelated gradle error\n"))
}

func TestDiffContainsClass(t *testing.T) {
	diffFiles := []string{
		"kmp/src/androidUnitTest/kotlin/dev/stapler/stelekit/ui/WikiSubdirFixDialogTest.kt",
		"kmp/src/commonMain/kotlin/dev/stapler/stelekit/ui/ScreenRouter.kt",
	}

	require.True(t, diffContainsClass(diffFiles, "WikiSubdirFixDialogTest"))
	require.False(t, diffContainsClass(diffFiles, "OutlinerRegressionTest"))
}

func TestGetBacklogStuckReport_ReturnsNotFoundError(t *testing.T) {
	storage := newTestBacklogStorage(t)
	handler := &backlogHandlers{storage: storage}

	req := makeToolReq(map[string]interface{}{
		// kibitzer:ignore replace-magic-literal -- "item_id" is the MCP tool's real argument name, matching every sibling test's makeToolReq call in this package
		"item_id": "00000000-0000-0000-0000-000000000999",
	})

	result, err := handler.getBacklogStuckReport(context.Background(), req)
	require.NoError(t, err)

	m := parseResult(t, result)
	require.False(t, m["success"].(bool))
	errObj, ok := m["error"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, ErrItemNotFound, errObj["code"].(string))
}

// TestGetBacklogStuckReport_RendersAcceptanceCriteriaTimelineSessionsAndNotes
// is the core regression for the diagnostic this tool ports from
// scripts/backlog-stuck-report.py: every section (AC checklist, stuck-state
// timeline including a resolved row, session history with cost, activity
// notes) must show up in one call, which is the whole point versus the
// cross-table archaeology get_backlog_item alone requires.
// kibitzer:ignore long-function -- table-setup-then-assert shape matches this file's sibling tests (e.g. TestGetBacklogItem_ReturnsItemWithEnvelope); splitting it would just move the setup into an unshared helper
func TestGetBacklogStuckReport_RendersAcceptanceCriteriaTimelineSessionsAndNotes(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:              "test: app shell coverage",
		AcceptanceCriteria: `[{"index":0,"text":"History cap boundary covered","status":"fail"}]`,
		Priority:           3,
		Status:             string(session.BacklogStatusReview),
	})
	require.NoError(t, err)

	_, err = storage.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, session.BacklogStatusReview, "bounced 13 times in 24h")
	require.NoError(t, err)
	resolved, err := storage.ResolveStuck(ctx, item.ID, domain.StuckReasonBouncing)
	require.NoError(t, err)
	require.True(t, resolved)

	_, err = storage.MarkStuck(ctx, item.ID, domain.StuckReasonAutonomousStuck, session.BacklogStatusReview, "stop_reason=max_turns")
	require.NoError(t, err)

	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "11111111-1111-1111-1111-111111111111",
		SessionRole: "work",
	})
	require.NoError(t, err)

	require.NoError(t, storage.AppendActivityNote(ctx, item.ID, "", "diagnose", "Diagnostic: not an infra/product defect"))

	handler := &backlogHandlers{storage: storage}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID})
	result, callErr := handler.getBacklogStuckReport(ctx, req)
	require.NoError(t, callErr)

	text := toolResultText(t, result)
	require.Contains(t, text, "--- STUCK ITEM REPORT")
	require.Contains(t, text, "## Acceptance Criteria")
	require.Contains(t, text, "History cap boundary covered")
	require.Contains(t, text, "## Stuck-state timeline (2 rows, incl. resolved)")
	require.Contains(t, text, string(domain.StuckReasonBouncing))
	require.Contains(t, text, "resolved ")
	require.Contains(t, text, string(domain.StuckReasonAutonomousStuck))
	require.Contains(t, text, "[OPEN]")
	require.Contains(t, text, "## Item sessions (1,")
	require.Contains(t, text, "role=work")
	require.Contains(t, text, "## Activity notes (1)")
	require.Contains(t, text, "Diagnostic: not an infra/product defect")
	// Not pr_pending, so no CI cross-check section.
	require.NotContains(t, text, "Live CI cross-check")
}

// fakeStuckReportPRChecker is a test double for stuckReportPRChecker so
// TestGetBacklogStuckReport_CICrossCheck_* never shells out to the real gh
// CLI or makes a real GitHub API call.
type fakeStuckReportPRChecker struct {
	status    *git.PRStatus
	diffFiles []string
	checkLogs map[git.CheckRunRef]string
}

func (f *fakeStuckReportPRChecker) GetPRStatus(ctx context.Context, prNumber int) (*git.PRStatus, error) {
	return f.status, nil
}

func (f *fakeStuckReportPRChecker) GetPRDiffFiles(ctx context.Context, prNumber int) ([]string, error) {
	return f.diffFiles, nil
}

func (f *fakeStuckReportPRChecker) GetCheckRunLog(ctx context.Context, ref git.CheckRunRef) (string, error) {
	return f.checkLogs[ref], nil
}

// TestGetBacklogStuckReport_CICrossCheck_FlagsOutOfDiffFailureAndSuggestsRerun
// is the live finding from PR tstapler/stelekit#425 turned into a regression:
// a failing check whose test file is outside the PR's diff must be flagged
// as likely pre-existing/flaky with a rerun suggestion, not silently treated
// the same as a real in-diff failure.
// kibitzer:ignore long-function -- same setup-then-assert shape as this file's other tests
func TestGetBacklogStuckReport_CICrossCheck_FlagsOutOfDiffFailureAndSuggestsRerun(t *testing.T) {
	storage := newTestBacklogStorage(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:    "test: app shell coverage",
		Priority: 3,
		Status:   string(session.BacklogStatusPRPending),
		RepoPath: "/repo",
	})
	require.NoError(t, err)
	_, err = storage.UpdateBacklogItem(ctx, item.ID, session.BacklogItemUpdate{PrNumber: intPtr(425), PrURL: strPtr("https://github.com/tstapler/stelekit/pull/425")}, nil)
	require.NoError(t, err)

	ref := git.CheckRunRef{RunID: "38730353308", JobID: "116420909053"}
	status, parseErr := git.ParsePRStatusPayload([]byte(`{
		"statusCheckRollup": [{
			"name": "Incremental coverage + mutation (changed files)",
			"conclusion": "FAILURE",
			"detailsUrl": "https://github.com/tstapler/stelekit/actions/runs/38730353308/job/116420909053"
		}],
		"mergeable": "MERGEABLE",
		"state": "OPEN"
	}`))
	require.NoError(t, parseErr)

	checker := &fakeStuckReportPRChecker{
		status: status,
		diffFiles: []string{
			"kmp/src/androidUnitTest/kotlin/dev/stapler/stelekit/ui/WikiSubdirFixDialogTest.kt",
		},
		checkLogs: map[git.CheckRunRef]string{
			ref: "OutlinerRegressionTest[jvm] > testBasicOutlinerFlow[jvm] FAILED\n",
		},
	}

	handler := &backlogHandlers{
		storage:          storage,
		prCheckerFactory: func(string) stuckReportPRChecker { return checker },
	}
	req := makeToolReq(map[string]interface{}{"item_id": item.ID})
	result, callErr := handler.getBacklogStuckReport(ctx, req)
	require.NoError(t, callErr)

	text := toolResultText(t, result)
	require.Contains(t, text, "## Live CI cross-check against this PR's own diff")
	require.Contains(t, text, "OutlinerRegressionTest: NOT in this PR's diff — likely pre-existing/flaky")
	require.Contains(t, text, "suggestion: `gh run rerun 38730353308 --failed`")
}

func toolResultText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(mcpgo.TextContent)
	require.True(t, ok)
	return tc.Text
}

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }
