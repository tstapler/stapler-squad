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
	t.Parallel()
	resp := firstCallJSON("s1", `{"summary":"A summary","tags":["feature"]}`)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	summary, err := SummarizeBacklogItem(context.Background(), pool, "Title", "Description")
	require.NoError(t, err)
	assert.Equal(t, "A summary", summary)
}

// TestSummarizeBacklogItem_Error_WhenPoolFails verifies error propagation.
func TestSummarizeBacklogItem_Error_WhenPoolFails(t *testing.T) {
	t.Parallel()
	runner := &FakeRunner{
		errors: []error{assert.AnError},
	}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := SummarizeBacklogItem(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestGenerateAcceptanceCriteria_ParsesJSONArray verifies AC parsing.
func TestGenerateAcceptanceCriteria_ParsesJSONArray(t *testing.T) {
	t.Parallel()
	resp := firstCallJSON("s1", `["AC1","AC2","AC3"]`)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	criteria, err := GenerateAcceptanceCriteria(context.Background(), pool, "Title", "Description")
	require.NoError(t, err)
	assert.Equal(t, []string{"AC1", "AC2", "AC3"}, criteria)
}

// TestGenerateAcceptanceCriteria_Error_WhenJSONInvalid verifies parse error.
func TestGenerateAcceptanceCriteria_Error_WhenJSONInvalid(t *testing.T) {
	t.Parallel()
	resp := firstCallJSON("s1", "not json")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := GenerateAcceptanceCriteria(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestGenerateAcceptanceCriteria_EmptyResponse_ReturnsError verifies empty-array error.
func TestGenerateAcceptanceCriteria_EmptyResponse_ReturnsError(t *testing.T) {
	t.Parallel()
	resp := firstCallJSON("s1", "[]")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := GenerateAcceptanceCriteria(context.Background(), pool, "T", "D")
	assert.Error(t, err)
}

// TestDraftPRDescription_ReturnsText_WhenFakeRunnerResponds verifies PR description return.
func TestDraftPRDescription_ReturnsText_WhenFakeRunnerResponds(t *testing.T) {
	t.Parallel()
	prText := "## Summary\n- Added feature\n\n## Test plan\n- Unit tests added"
	resp := firstCallJSON("s1", prText)
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	result, _, err := DraftPRDescription(context.Background(), pool, "Fix flaky test", "The test flaked under CI load.", "diff content", "feat/my-feature")
	require.NoError(t, err)
	assert.Equal(t, prText, result)
}

// TestDraftPRDescription_TruncatesDiff_WhenOver40000Bytes verifies truncation.
func TestDraftPRDescription_TruncatesDiff_WhenOver40000Bytes(t *testing.T) {
	t.Parallel()
	bigDiff := strings.Repeat("x", 50_000)
	resp := firstCallJSON("s1", "PR description")
	runner := NewFakeRunner(resp)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, _, err := DraftPRDescription(context.Background(), pool, "Title", "Description", bigDiff, "branch")
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
	t.Parallel()
	runner := NewFakeRunner()
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, _, err := DraftPRDescription(context.Background(), pool, "Title", "Description", "   ", "branch")
	assert.Error(t, err)
	assert.Equal(t, 0, runner.CallCount(), "LLM should never be called for an empty diff")
}

// TestSuggestCommitMessage_ReturnsText_WhenFakeRunnerResponds verifies commit message return.
func TestSuggestCommitMessage_ReturnsText_WhenFakeRunnerResponds(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// fakePoolClientRecorder is a minimal PoolClient test double that records the
// exact arguments it was called with, for assertions that need to verify which
// FeatureKey/prompts a caller used (something FakeRunner/Pool doesn't expose
// directly, since FeatureKey only affects internal session-affinity bookkeeping).
type fakePoolClientRecorder struct {
	response string
	err      error

	calls int
	key   FeatureKey
	sys   string
	user  string
}

func (f *fakePoolClientRecorder) CallBlocking(_ context.Context, key FeatureKey, systemPrompt, userPrompt string, _ CallOptions, sink CostSink) (string, error) {
	f.calls++
	f.key = key
	f.sys = systemPrompt
	f.user = userPrompt
	sink(0)
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

// TestGenerateSessionCompletionNarrative_should_ReturnProseAndCallPoolWithFeatureKey_When_TitleGoalDiffAndDecisionsProvided
// verifies the happy path: prose is returned and pool.CallBlocking was invoked with
// FeatureKeySessionCompletionSummary.
func TestGenerateSessionCompletionNarrative_should_ReturnProseAndCallPoolWithFeatureKey_When_TitleGoalDiffAndDecisionsProvided(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{response: "The session fixed the login redirect loop."}

	text, _, err := GenerateSessionCompletionNarrative(context.Background(), fake, "fix-login-redirect", "Investigate why login redirects loop under SSO", "diff content", "5 auto-approved, 1 manual")
	require.NoError(t, err)
	assert.Equal(t, "The session fixed the login redirect loop.", text)
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, FeatureKeySessionCompletionSummary, fake.key)
	assert.Contains(t, fake.user, "fix-login-redirect")
	assert.Contains(t, fake.user, "Investigate why login redirects loop under SSO")
}

// TestGenerateSessionCompletionNarrative_should_OmitGoalLine_When_SessionGoalIsEmptyString
// verifies that an empty sessionGoal (never set) is simply omitted from the prompt,
// not rendered as an empty/placeholder line, and the call still succeeds.
func TestGenerateSessionCompletionNarrative_should_OmitGoalLine_When_SessionGoalIsEmptyString(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{response: "The session made some changes."}

	text, _, err := GenerateSessionCompletionNarrative(context.Background(), fake, "some-session", "", "diff content", "no decisions")
	require.NoError(t, err)
	assert.Equal(t, "The session made some changes.", text)
	assert.NotContains(t, fake.user, "Session goal:")
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_DistinctFromNormalPrompt verifies
// the codebase-access prompt (used on the empty-diff path) is a genuinely different
// string from the normal no-tool-access prompt, not an accidental alias.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_DistinctFromNormalPrompt(t *testing.T) {
	t.Parallel()
	assert.NotEqual(t, HeadlessReviewSystemPrompt(), HeadlessReviewSystemPromptWithCodebaseAccess())
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresOwnCitation verifies the
// falsification framing instructs the model to cite evidence it found itself, not
// merely restate the work session's claim.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresOwnCitation(t *testing.T) {
	t.Parallel()
	assert.Contains(t, strings.ToLower(HeadlessReviewSystemPromptWithCodebaseAccess()), "your own")
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresToolReadsField verifies the
// prompt requires the model to report which files it actually opened, so the caller
// can detect and degrade a verdict backed by no real tool use.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_RequiresToolReadsField(t *testing.T) {
	t.Parallel()
	assert.Contains(t, HeadlessReviewSystemPromptWithCodebaseAccess(), "tool_reads")
}

// TestHeadlessReviewSystemPrompt_NoteOnNonEmptyDiffIsInformationalOnly verifies the
// plain (diff != "") review prompt bounds the evidentiary weight of a criterion's
// self-reported Note: it must not be sufficient by itself for a PASS.
func TestHeadlessReviewSystemPrompt_NoteOnNonEmptyDiffIsInformationalOnly(t *testing.T) {
	t.Parallel()
	prompt := HeadlessReviewSystemPrompt()
	assert.Contains(t, prompt, "informational")
	assert.Contains(t, prompt, "Note")
}

// TestHeadlessReviewSystemPromptWithCodebaseAccess_UnaffectedByEvidentiaryWeightChange
// is a regression guard: the Note-evidentiary-weight sentence added to the plain
// (diff != "") prompt must not leak into the codebase-access (diff == "") prompt,
// which has its own distinct falsification instructions.
func TestHeadlessReviewSystemPromptWithCodebaseAccess_UnaffectedByEvidentiaryWeightChange(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	assert.Contains(t, HeadlessTriageSystemPrompt(), "single, non-interactive call")
	assert.Contains(t, HeadlessTriageSystemPrompt(), "no later turn")
}

// TestGenerateHandoffSummary_PrependsReferenceOnlyPrefixVerbatim verifies the
// returned string's first line is exactly referenceOnlyPrefix, reproduced
// verbatim (not paraphrased), for a non-empty TranscriptWindow and a
// successful pool call.
func TestGenerateHandoffSummary_PrependsReferenceOnlyPrefixVerbatim(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{response: "The session refactored the auth middleware."}
	head := []HandoffTranscriptMessage{{Role: "user", Content: "Let's refactor auth"}}
	middle := []HandoffTranscriptMessage{{Role: "assistant", Content: "Refactored middleware.go"}}
	tail := []HandoffTranscriptMessage{{Role: "assistant", Content: "Running tests next"}}

	text, err := GenerateHandoffSummary(context.Background(), fake, "auth-refactor", head, middle, tail)
	require.NoError(t, err)

	firstLine := strings.SplitN(text, "\n", 2)[0]
	assert.Equal(t, referenceOnlyPrefix, firstLine)
	assert.Equal(t, 1, fake.calls)
}

// TestGenerateHandoffSummary_PromptInstructsActiveTaskSection verifies the
// constructed userPrompt (sent to the LLM, before the pool call returns)
// contains the literal "## Active Task" instruction, plus separately
// labeled Head:/Middle (to summarize):/Tail: sections built from the
// window's three slices.
func TestGenerateHandoffSummary_PromptInstructsActiveTaskSection(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{response: "summary text"}
	head := []HandoffTranscriptMessage{{Role: "user", Content: "start the task"}}
	middle := []HandoffTranscriptMessage{{Role: "assistant", Content: "did some middle work"}}
	tail := []HandoffTranscriptMessage{{Role: "assistant", Content: "about to run tests"}}

	_, err := GenerateHandoffSummary(context.Background(), fake, "some-session", head, middle, tail)
	require.NoError(t, err)

	assert.Equal(t, FeatureKeyHandoffSummary, fake.key)
	assert.Contains(t, fake.user, "## Active Task")
	assert.Contains(t, fake.user, "Head:")
	assert.Contains(t, fake.user, "Middle (to summarize):")
	assert.Contains(t, fake.user, "Tail:")
	assert.Contains(t, fake.user, "start the task")
	assert.Contains(t, fake.user, "did some middle work")
	assert.Contains(t, fake.user, "about to run tests")
}

// TestGenerateHandoffSummary_EmptyMiddlePlaceholderText verifies that a
// TranscriptWindow with an empty Middle (short conversation) still calls the
// pool, but the prompt's middle section reads the explicit placeholder
// rather than an empty block.
func TestGenerateHandoffSummary_EmptyMiddlePlaceholderText(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{response: "summary text"}
	head := []HandoffTranscriptMessage{{Role: "user", Content: "short chat"}}
	var middle []HandoffTranscriptMessage
	tail := []HandoffTranscriptMessage{{Role: "assistant", Content: "done"}}

	_, err := GenerateHandoffSummary(context.Background(), fake, "short-session", head, middle, tail)
	require.NoError(t, err)

	assert.Equal(t, 1, fake.calls)
	assert.Contains(t, fake.user, "(nothing to summarize — conversation was short)")
}

// TestGenerateHandoffSummary_PropagatesPoolClientError_When_CallBlockingFails
// verifies that a pool.CallBlocking failure is propagated as-is with no
// partial/garbled text returned.
func TestGenerateHandoffSummary_PropagatesPoolClientError_When_CallBlockingFails(t *testing.T) {
	t.Parallel()
	fake := &fakePoolClientRecorder{err: assert.AnError}
	head := []HandoffTranscriptMessage{{Role: "user", Content: "start"}}
	tail := []HandoffTranscriptMessage{{Role: "assistant", Content: "end"}}

	text, err := GenerateHandoffSummary(context.Background(), fake, "failing-session", head, nil, tail)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Empty(t, text)
}
