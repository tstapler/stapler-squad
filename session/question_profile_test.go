package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/detection/dtypes"
	"github.com/tstapler/stapler-squad/session/domain"
)

func paneFixture(t *testing.T, harness, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "panes", harness, name))
	require.NoError(t, err)
	return string(b)
}

func TestClaudeProfile_Should_WrapExistingDialogMatch_When_RealCaptureFixture(t *testing.T) {
	p := ClaudeQuestionProfile{}
	for fixture, want := range map[string]QuestionDialog{
		"question_single_select_3opts.txt": colorQuestion,
		"question_wrapped_2opts.txt":       migrationQuestion,
	} {
		capture := loadDialogFixture(t, fixture)
		d, ok := p.Extract(capture)
		require.True(t, ok, fixture)
		assert.Equal(t, dtypes.ProfileDialog{Profile: "claude", Header: want.Header, Question: want.Question, Options: want.Labels}, d)
		assert.True(t, p.Verify(capture, d))
		assert.Equal(t, DialogMatch(capture, want), p.Verify(capture, d), "Verify is DialogMatch")
		assert.False(t, p.Verify(capture+"\n  1. Red\n", d), "Verify fails closed exactly where DialogMatch does")
	}
}

func TestClaudeProfile_Should_NotExtract_When_NotASingleSelectDialog(t *testing.T) {
	p := ClaudeQuestionProfile{}
	for _, f := range []string{
		"permission_bash.txt", "permission_edit_diff.txt", "claude_auq_twoq_step1_single_select.txt",
		"claude_auq_twoq_step2_multiselect_initial.txt", "claude_auq_twoq_review_submit.txt",
	} {
		_, ok := p.Extract(loadDialogFixture(t, f))
		assert.False(t, ok, f)
	}
	forged := loadDialogFixture(t, "question_single_select_3opts.txt") + "\n  1. Allow once\n"
	_, ok := p.Extract(forged)
	assert.False(t, ok, "a row after the footer is no dialog")
}

func TestClaudeProfile_Should_AnswerWithTheOptionDigit(t *testing.T) {
	p := ClaudeQuestionProfile{}
	d, ok := p.Extract(loadDialogFixture(t, "question_single_select_3opts.txt"))
	require.True(t, ok)
	for i, want := range []string{"1", "2", "3"} {
		keys, ok := p.AnswerKeys(i, d)
		assert.True(t, ok)
		assert.Equal(t, want, keys)
	}
	_, ok = p.AnswerKeys(3, d)
	assert.False(t, ok)
	_, ok = p.AnswerKeys(-1, d)
	assert.False(t, ok)
}

func TestRegistry_Should_ContainOnlyMeasuredHarnesses(t *testing.T) {
	r := DefaultQuestionProfiles()
	assert.Equal(t, []string{"claude", "opencode"}, r.Names())
	// Spike S5: Aider's reply keystroke was never captured; pi has no TUI permission
	// prompts; Gemini does not start; agy never prompted. All stay read_only.
	for _, h := range []string{"aider", "pi", "gemini", "agy", "antigravity", "codex", ""} {
		assert.False(t, r.Has(h), h)
		_, ok := r.Lookup(h)
		assert.False(t, ok, h)
	}
}

func TestRegistry_Should_NotExtractUnmeasuredHarnessScreens(t *testing.T) {
	r := DefaultQuestionProfiles()
	for harness, name := range map[string]string{
		"aider": "confirm_run_output.txt", "pi": "extension_select.txt", "agy": "trust_folder.txt",
	} {
		_, ok := r.Extract(paneFixture(t, harness, name))
		assert.False(t, ok, harness)
	}
	d, ok := r.Extract(paneFixture(t, "opencode", "permission_bash.txt"))
	require.True(t, ok)
	assert.Equal(t, "opencode", d.Profile)
	d, ok = r.Extract(loadDialogFixture(t, "question_single_select_3opts.txt"))
	require.True(t, ok)
	assert.Equal(t, "claude", d.Profile)
}

func TestReplyableProfile_Should_CreateRowWithSourceAskUserQuestionHookAndProfileName_When_OpenCodeFixtureDetected(t *testing.T) {
	d, ok := DefaultQuestionProfiles().Extract(paneFixture(t, "opencode", "permission_bash.txt"))
	require.True(t, ok)
	params, ok := ProfileGuidanceParams("sess-1", d)
	require.True(t, ok)
	gr, err := domain.NewGuidanceRequest(params)
	require.NoError(t, err)
	p := gr.Params()
	assert.Equal(t, domain.QuestionSourceHook, p.Source)
	json, err := p.Payload.JSON()
	require.NoError(t, err)
	assert.Contains(t, json, `"profile":"opencode"`)
	assert.Contains(t, json, `"Allow once"`)
	assert.Equal(t, "sess-1", p.AskerSessionUUID)

	// Claude's hook payload keeps its serialization: no profile key.
	cd, ok := DefaultQuestionProfiles().Extract(loadDialogFixture(t, "question_single_select_3opts.txt"))
	require.True(t, ok)
	cp, ok := ProfileGuidanceParams("sess-1", cd)
	require.True(t, ok)
	claudeJSON, err := cp.Payload.JSON()
	require.NoError(t, err)
	assert.True(t, strings.Contains(claudeJSON, `"profile":"claude"`))
}

func TestReplyableProfile_Should_CreateNoRow_When_HarnessIsUnmeasured(t *testing.T) {
	_, ok := ProfileGuidanceParams("sess-1", dtypes.ProfileDialog{
		Profile: "aider", Question: "Add output to the chat?", Options: []string{"Yes", "No"},
	})
	assert.False(t, ok, "Aider's (Y)es/(N)o prompt has no measured reply: notification carries only OPEN")
	_, ok = ProfileGuidanceParams("sess-1", dtypes.ProfileDialog{Profile: "opencode"})
	assert.False(t, ok, "no options, no row")
}

func TestOpenCodeProfile_Should_AnswerThroughTheSequenceDriver_When_AllowOnce(t *testing.T) {
	p := DefaultQuestionProfiles()
	capture := paneFixture(t, "opencode", "permission_bash.txt")
	d, ok := p.Extract(capture)
	require.True(t, ok)
	prof, _ := p.Lookup("opencode")
	keys, ok := prof.AnswerKeys(0, d)
	require.True(t, ok)

	pane := &fakeReplyPane{owner: replyOwner, captures: []string{capture, "\n  ┃  Ask anything\n"}}
	_, lease := replyInstanceAndLease(t)
	res := SubmitReplySequence(t.Context(), pane, lease, []ReplyStep{{
		Keys: keys, Expect: func(c string) bool { return prof.Verify(c, d) },
	}}, SequenceOptions{StillOpen: func(c string) bool { _, ok := prof.Extract(c); return ok }, After: seqAfter})
	require.NoError(t, res.Err)
	assert.Equal(t, StageDialogClosed, res.Stage)
	assert.Equal(t, []string{"\r"}, pane.writtenBytes())
}
