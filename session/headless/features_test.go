package headless

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSummarizeBacklogItem_ReturnsText_WhenFakeRunnerResponds verifies summary parsing.
func TestSummarizeBacklogItem_ReturnsText_WhenFakeRunnerResponds(t *testing.T) {
	resp := firstCallJSON("s1", `{"summary":"A summary","tags":["feature"]}`)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	summary, err := SummarizeBacklogItem(context.Background(), pool, "Title", "Description")
	require.NoError(t, err)
	assert.Equal(t, "A summary", summary)
}

// TestSummarizeBacklogItem_Error_WhenPoolFails verifies error propagation.
func TestSummarizeBacklogItem_Error_WhenPoolFails(t *testing.T) {
	runner := &FakeRunner{
		errors: []error{assert.AnError},
	}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := SummarizeBacklogItem(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestGenerateAcceptanceCriteria_ParsesJSONArray verifies AC parsing.
func TestGenerateAcceptanceCriteria_ParsesJSONArray(t *testing.T) {
	resp := firstCallJSON("s1", `["AC1","AC2","AC3"]`)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	criteria, err := GenerateAcceptanceCriteria(context.Background(), pool, "Title", "Description")
	require.NoError(t, err)
	assert.Equal(t, []string{"AC1", "AC2", "AC3"}, criteria)
}

// TestGenerateAcceptanceCriteria_Error_WhenJSONInvalid verifies parse error.
func TestGenerateAcceptanceCriteria_Error_WhenJSONInvalid(t *testing.T) {
	resp := firstCallJSON("s1", "not json")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := GenerateAcceptanceCriteria(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestGenerateAcceptanceCriteria_EmptyResponse_ReturnsError verifies empty-array error.
func TestGenerateAcceptanceCriteria_EmptyResponse_ReturnsError(t *testing.T) {
	resp := firstCallJSON("s1", "[]")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := GenerateAcceptanceCriteria(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestDraftPRDescription_ReturnsText_WhenFakeRunnerResponds verifies PR description return.
func TestDraftPRDescription_ReturnsText_WhenFakeRunnerResponds(t *testing.T) {
	prText := "## Summary\n- Added feature\n\n## Test plan\n- Unit tests added"
	resp := firstCallJSON("s1", prText)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	result, err := DraftPRDescription(context.Background(), pool, "Fix flaky test", "The test flaked under CI load.", "diff content", "feat/my-feature")
	require.NoError(t, err)
	assert.Equal(t, prText, result)
}

// TestDraftPRDescription_TruncatesDiff_WhenOver40000Bytes verifies truncation.
func TestDraftPRDescription_TruncatesDiff_WhenOver40000Bytes(t *testing.T) {
	bigDiff := strings.Repeat("x", 50_000)
	resp := firstCallJSON("s1", "PR description")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := DraftPRDescription(context.Background(), pool, "Title", "Description", bigDiff, "branch")
	require.NoError(t, err)

	// Inspect what was passed to the runner.
	args := runner.ArgsForCall(0)
	require.NotNil(t, args)
	// The user prompt is the last arg; it should contain a truncated diff.
	userPrompt := args[len(args)-1]
	// The prompt contains the "Backlog item / Problem statement / Branch / Diff"
	// prefix (~60 chars) + diff. Total should be <= maxDiffSizePR + prefix length.
	assert.LessOrEqual(t, len(userPrompt), maxDiffSizePR+150,
		"user prompt should not be much larger than maxDiffSizePR; got %d bytes", len(userPrompt))
}

// TestDraftPRDescription_Error_WhenDiffEmpty verifies the empty-diff guard —
// found live (PR #174 on this repo) that sending an empty diff to the LLM
// produced a conversational non-answer instead of a usable PR body, so
// DraftPRDescription now short-circuits before calling the LLM at all.
func TestDraftPRDescription_Error_WhenDiffEmpty(t *testing.T) {
	runner := NewFakeRunner()
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := DraftPRDescription(context.Background(), pool, "Title", "Description", "   ", "branch")
	assert.Error(t, err)
	assert.Equal(t, 0, runner.CallCount(), "LLM should never be called for an empty diff")
}

// TestSuggestCommitMessage_ReturnsText_WhenFakeRunnerResponds verifies commit message return.
func TestSuggestCommitMessage_ReturnsText_WhenFakeRunnerResponds(t *testing.T) {
	msg := "feat(auth): add OAuth2 login"
	resp := firstCallJSON("s1", msg)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	result, err := SuggestCommitMessage(context.Background(), pool, "diff content")
	require.NoError(t, err)
	assert.Equal(t, msg, result)
}

// TestSuggestCommitMessage_TruncatesDiff_WhenOver20000Bytes verifies truncation.
func TestSuggestCommitMessage_TruncatesDiff_WhenOver20000Bytes(t *testing.T) {
	bigDiff := strings.Repeat("y", 30_000)
	resp := firstCallJSON("s1", "fix(core): a fix")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := SuggestCommitMessage(context.Background(), pool, bigDiff)
	require.NoError(t, err)

	args := runner.ArgsForCall(0)
	require.NotNil(t, args)
	userPrompt := args[len(args)-1]
	assert.LessOrEqual(t, len(userPrompt), maxDiffSizeCommit+10,
		"user prompt should be <= maxDiffSizeCommit bytes; got %d", len(userPrompt))
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_DistinctFromNormalPrompt verifies
// the codebase-access prompt (used on the empty-diff path) is a genuinely different
// string from the normal no-tool-access prompt, not an accidental alias.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_DistinctFromNormalPrompt(t *testing.T) {
	assert.NotEqual(t, HeadlessReviewSystemPrompt(), HeadlessReviewSystemPromptWithCodebaseAccess())
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresOwnCitation verifies the
// falsification framing instructs the model to cite evidence it found itself, not
// merely restate the work session's claim.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresOwnCitation(t *testing.T) {
	assert.Contains(t, strings.ToLower(HeadlessReviewSystemPromptWithCodebaseAccess()), "your own")
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresToolReadsField verifies the
// prompt requires the model to report which files it actually opened, so the caller
// can detect and degrade a verdict backed by no real tool use.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresToolReadsField(t *testing.T) {
	assert.Contains(t, HeadlessReviewSystemPromptWithCodebaseAccess(), "tool_reads")
}

// TestHeadlessReviewSystemPrompt_NoteOnNonEmptyDiffIsInformationalOnly verifies the
// plain (diff != "") review prompt bounds the evidentiary weight of a criterion's
// self-reported Note: it must not be sufficient by itself for a PASS.
func TestHeadlessReviewSystemPrompt_NoteOnNonEmptyDiffIsInformationalOnly(t *testing.T) {
	prompt := HeadlessReviewSystemPrompt()
	assert.Contains(t, prompt, "informational")
	assert.Contains(t, prompt, "Note")
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_UnaffectedByEvidentiaryWeightChange
// is a regression guard: the Note-evidentiary-weight sentence added to the plain
// (diff != "") prompt must not leak into the codebase-access (diff == "") prompt,
// which has its own distinct falsification instructions.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_UnaffectedByEvidentiaryWeightChange(t *testing.T) {
	assert.NotContains(t, HeadlessReviewSystemPromptWithCodebaseAccess(), "informational context only")
}

// TestHeadlessTriageSystemPrompt_WarnsAgainstBackgroundStatusPlaceholder guards the
// "single, non-interactive call" paragraph added after a live incident where a
// headless triage call ended its turn with a status update describing a still-running
// background subagent instead of the final JSON block (see the doc comment above
// headlessTriageSystemPrompt in features.go, and
// TestParseHeadlessTriageResult_PrematureCompletionPlaceholder in
// session/backlog_triage_test.go for the parser-side half of this regression). A
// future edit to this prompt string must not silently drop the guidance with zero
// test failure.
func TestHeadlessTriageSystemPrompt_WarnsAgainstBackgroundStatusPlaceholder(t *testing.T) {
	assert.Contains(t, HeadlessTriageSystemPrompt(), "single, non-interactive call")
	assert.Contains(t, HeadlessTriageSystemPrompt(), "no later turn")
}
