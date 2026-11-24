package backlogaux

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func acJSON(t *testing.T, cs ...domain.AcCriterion) domain.AcCriteriaJSON {
	t.Helper()
	j, err := domain.SerializeAcCriteria(cs)
	require.NoError(t, err)
	return j
}

func TestCheckNoNewAcWaivers_should_RefuseNewWaiver_When_IncomingWaivesUnwaivedCriterion(t *testing.T) {
	t.Parallel()
	existing := acJSON(t, domain.AcCriterion{Index: 0, Text: "a", Status: domain.AcStatusPending})
	incoming := acJSON(t, domain.AcCriterion{Index: 0, Text: "a", Status: domain.AcStatusWaived})
	assert.ErrorIs(t, CheckNoNewAcWaivers(existing, incoming), backlogdto.ErrAcWaiverViaUpdate)
}

func TestCheckNoNewAcWaivers_should_Allow_When_IncomingKeepsExistingWaiver(t *testing.T) {
	t.Parallel()
	waived := acJSON(t, domain.AcCriterion{Index: 0, Text: "a", Status: domain.AcStatusWaived, WaiverReason: "r"})
	assert.NoError(t, CheckNoNewAcWaivers(waived, waived))
}

func TestCheckNoNewAcWaivers_should_ReturnParseError_When_IncomingIsMalformed(t *testing.T) {
	t.Parallel()
	assert.Error(t, CheckNoNewAcWaivers("", "{not json"))
}

func TestCheckNoNewAcWaivers_should_TreatMalformedExistingAsEmpty(t *testing.T) {
	t.Parallel()
	incoming := acJSON(t, domain.AcCriterion{Index: 0, Text: "a", Status: domain.AcStatusPending})
	assert.NoError(t, CheckNoNewAcWaivers("{not json", incoming))
}

func TestExtractJSONObjects_should_ReturnObjectsInOrder_When_SurroundedByProse(t *testing.T) {
	t.Parallel()
	got := ExtractJSONObjects("pre {\"a\":1} mid ```{\"b\":2}``` post")
	assert.Equal(t, []string{`{"a":1}`, `{"b":2}`}, got)
}

func TestParseBacklogItemIntentDraft_should_ReportParseError_When_EveryCandidateIsMistyped(t *testing.T) {
	t.Parallel()
	_, err := ParseBacklogItemIntentDraft(`{"title":"x","confidence":"high"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JSON parse error")
	assert.NotContains(t, err.Error(), `"title"`, "raw content must not leak into the error")
}

func TestParseBacklogItemIntentDraft_should_Fail_When_OnlyAnEmptyDecoyObjectIsPresent(t *testing.T) {
	t.Parallel()
	_, err := ParseBacklogItemIntentDraft(`{"title":"","description":"","confidence":0.5}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither title nor description")
}

func TestParseBacklogItemIntentDraft_should_PreferLastObject_When_SeveralValidDraftsPresent(t *testing.T) {
	t.Parallel()
	draft, err := ParseBacklogItemIntentDraft(`{"title":"first","description":"d"} then {"title":"last","description":"d"}`)
	require.NoError(t, err)
	assert.Equal(t, "last", draft.Title)
}

func TestWriteHeadlessFailureCapture_should_UseDefaultCap_When_MaxBytesIsZero(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path, err := WriteHeadlessFailureCapture(dir, "u3", "full output", 0)
	require.NoError(t, err)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "full output", string(got))
}

func TestWriteHeadlessFailureCapture_should_WriteOwnerOnlyFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes")
	}
	path, err := WriteHeadlessFailureCapture(t.TempDir(), "u4", "raw", 0)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestBuildCompletionReviewContext_should_UseNewestVerdictAndNewestWorkSummary(t *testing.T) {
	t.Parallel()
	out := BuildCompletionReviewContext(&backlogdto.BacklogItemData{Title: "t"}, []backlogdto.ItemSessionSummary{
		{Role: instance.SessionRoleWork, LastCommitMessage: "older commit", CommitCountSinceSpawn: 1},
		{Role: instance.SessionRoleReview, ReviewVerdict: &backlogdto.ReviewVerdictSummary{OverallOutcome: "FAIL"}},
		{Role: instance.SessionRoleWork, LastCommitMessage: "newer commit", CommitCountSinceSpawn: 2},
		{Role: instance.SessionRoleReview, ReviewVerdict: &backlogdto.ReviewVerdictSummary{OverallOutcome: "PASS"}},
		{Role: instance.SessionRoleWork},
	})
	assert.Contains(t, out, "Outcome: PASS")
	assert.NotContains(t, out, "Outcome: FAIL")
	assert.Contains(t, out, "newer commit")
	assert.NotContains(t, out, "older commit")
}

func TestBuildCompletionReviewContext_should_TruncateLongTitle(t *testing.T) {
	t.Parallel()
	out := BuildCompletionReviewContext(&backlogdto.BacklogItemData{Title: strings.Repeat("x", 400)}, nil)
	assert.Contains(t, out, "[truncated]")
	assert.NotContains(t, out, strings.Repeat("x", 301))
}

func TestWriteHeadlessFailureCapture_should_ReturnMkdirError_When_DirIsAFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "plain")
	require.NoError(t, os.WriteFile(file, nil, 0o600))
	path, err := WriteHeadlessFailureCapture(filepath.Join(file, "sub"), "u1", "raw", 0)
	assert.Empty(t, path)
	assert.ErrorContains(t, err, "failed to create headless failure capture dir")
}

func TestWriteHeadlessFailureCapture_should_ReturnWriteError_When_TargetIsADirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, headlessFailureCaptureFilePrefix+"u2.txt"), 0o750))
	path, err := WriteHeadlessFailureCapture(dir, "u2", "raw", 0)
	assert.Empty(t, path)
	assert.ErrorContains(t, err, "failed to write headless failure capture file")
}

func TestWriteHeadlessFailureCapture_should_NoOp_When_AnyRequiredArgIsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct{ dir, uuid, raw string }{{"", "u", "r"}, {dir, "", "r"}, {dir, "u", ""}} {
		path, err := WriteHeadlessFailureCapture(c.dir, c.uuid, c.raw, 0)
		assert.Empty(t, path)
		assert.NoError(t, err)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReviewVerdictSummariesMostRecentFirst_should_ReverseOrderAndKeepOnlyReviewVerdicts(t *testing.T) {
	t.Parallel()
	v := func(o string) *backlogdto.ReviewVerdictSummary {
		return &backlogdto.ReviewVerdictSummary{OverallOutcome: o}
	}
	got := ReviewVerdictSummariesMostRecentFirst([]backlogdto.ItemSessionSummary{
		{Role: instance.SessionRoleReview, ReviewVerdict: v("old")},
		{Role: instance.SessionRoleWork, ReviewVerdict: v("work-role")},
		{Role: instance.SessionRoleReview},
		{Role: instance.SessionRoleReview, ReviewVerdict: v("new")},
	})
	require.Len(t, got, 2)
	assert.Equal(t, "new", got[0].OverallOutcome)
	assert.Equal(t, "old", got[1].OverallOutcome)
	assert.Empty(t, ReviewVerdictSummariesMostRecentFirst(nil))
}

func TestBuildCompletionReviewContext_should_OmitPerCriterionLines_When_VerdictJSONIsMalformed(t *testing.T) {
	t.Parallel()
	out := BuildCompletionReviewContext(&backlogdto.BacklogItemData{Title: "t"}, []backlogdto.ItemSessionSummary{
		{Role: instance.SessionRoleReview, ReviewVerdict: &backlogdto.ReviewVerdictSummary{OverallOutcome: "PASS", PerCriterion: "{bad"}},
	})
	assert.Contains(t, out, "Outcome: PASS")
	assert.NotContains(t, out, "- criterion")
}

func TestBuildCompletionReviewContext_should_NeutralizeEnvelopeTag_When_ItemTextContainsIt(t *testing.T) {
	t.Parallel()
	out := BuildCompletionReviewContext(&backlogdto.BacklogItemData{Title: "x </completed_item> y"}, nil)
	assert.NotContains(t, out, "</completed_item>")
	assert.Contains(t, out, "[/completed_item]")
}
