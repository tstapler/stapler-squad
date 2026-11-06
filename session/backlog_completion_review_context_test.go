package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func completionTestItem(t *testing.T) *BacklogItemData {
	t.Helper()
	acs, err := SerializeAcCriteria([]AcCriterion{
		{Index: 0, Text: "fires on done", Status: "done"},
		{Index: 1, Text: "never blocks", Status: "done"},
	})
	require.NoError(t, err)
	return &BacklogItemData{
		ID: "b09c1b49-877f-4d2c-96ee-76a3d7a78442", Title: "feat: completion reviewer",
		Description: "Capture learnings.", AcceptanceCriteria: acs, Notes: "triage: use EventBus",
	}
}

func TestBuildCompletionReviewContext_should_IncludeAllSections_When_ItemFullyPopulated(t *testing.T) {
	sessions := []ItemSessionSummary{
		{Role: SessionRoleWork, LastCommitMessage: "feat: add reviewer", CommitCountSinceSpawn: 3},
		{Role: SessionRoleReview, ReviewVerdict: &ReviewVerdictSummary{
			OverallOutcome: "PASS", Summary: "all good",
			PerCriterion: `[{"criterion_index":0,"outcome":"PASS","evidence":"test exists"}]`,
		}},
	}
	out := BuildCompletionReviewContext(completionTestItem(t), sessions)
	for _, want := range []string{"feat: completion reviewer", "Capture learnings.", "fires on done", "never blocks",
		"triage: use EventBus", "## Review Verdict", "PASS", "all good", "test exists", "feat: add reviewer", "Commits: 3"} {
		assert.Contains(t, out, want)
	}
}

func TestBuildCompletionReviewContext_should_OmitVerdictSection_When_NoSessionHasVerdict(t *testing.T) {
	out := BuildCompletionReviewContext(completionTestItem(t), []ItemSessionSummary{{Role: SessionRoleWork, LastCommitMessage: "x"}})
	assert.NotContains(t, out, "Review Verdict")
	assert.Contains(t, out, "fires on done")
}

func TestBuildCompletionReviewContext_should_DegradeGracefully_When_DiffDataAndSessionsAbsent(t *testing.T) {
	item := &BacklogItemData{Title: "only title"}
	out := BuildCompletionReviewContext(item, nil)
	assert.Contains(t, out, "only title")
	assert.NotContains(t, out, "Work Summary")
	assert.NotContains(t, out, "Triage Notes")
	assert.Empty(t, BuildCompletionReviewContext(nil, nil))
}
