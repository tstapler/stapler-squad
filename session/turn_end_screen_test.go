package session

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/detection"
)

const claudeIdleCtx = "Claude Code idle prompt showing ? for shortcuts"

func TestDetectScreenTurnEnd_Should_RejectStatusReadyAndUnsafeContexts(t *testing.T) {
	t.Parallel()
	pane := "Which approach would you like me to take?\n\n> \n? for shortcuts\n"
	for name, tc := range map[string]struct {
		status detection.DetectedStatus
		ctx    string
	}{
		"ready is dead code":    {detection.StatusReady, claudeIdleCtx},
		"idle but vim prompt":   {detection.StatusIdle, "Command prompt"},
		"idle empty context":    {detection.StatusIdle, ""},
		"active spinner":        {detection.StatusProcessing, claudeIdleCtx},
		"claude insert mode":    {detection.StatusIdle, "Claude Code insert mode"},
		"success other context": {detection.StatusSuccess, "All tests passed"},
	} {
		_, ok := DetectScreenTurnEnd("claude", tc.status, tc.ctx, pane)
		assert.False(t, ok, name)
	}
}

func TestDetectScreenTurnEnd_Should_TakeBottomMostParagraphAboveThePrompt(t *testing.T) {
	t.Parallel()
	pane := "Earlier tool output line one\n\n● Summary of what changed in the cache.\n\n  Want me to also update the docs?\n\n╭────╮\n│ >  │\n╰────╯\n  ? for shortcuts\n"
	got, ok := DetectScreenTurnEnd("claude", detection.StatusIdle, claudeIdleCtx, pane)
	assert.True(t, ok)
	assert.Equal(t, "Want me to also update the docs?", got.Text)
	assert.Equal(t, TurnEndKindQuestion, got.Kind)
}

func TestDetectScreenTurnEnd_Should_YieldFinishedForAiderEvenWithAQuestionTail(t *testing.T) {
	t.Parallel()
	pane := "I can move the helper into utils.py or keep it in main.py.\nDo you want me to move it?\n\n> \n"
	got, ok := DetectScreenTurnEnd("aider", detection.StatusUnknown, "", pane)
	assert.True(t, ok)
	assert.Equal(t, TurnEndKindFinished, got.Kind)
}

func TestDetectScreenTurnEnd_Should_ReturnNothing_When_NoProseAbovePrompt(t *testing.T) {
	t.Parallel()
	_, ok := DetectScreenTurnEnd("aider", detection.StatusUnknown, "", "────\n> \n")
	assert.False(t, ok)
}

func TestDetectScreenTurnEnd_Should_NotReadNoneRows(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"gemini", "opencode", "pi", "unknown-agent"} {
		_, ok := DetectScreenTurnEnd(p, detection.StatusIdle, claudeIdleCtx, "Did it work out for you today?\n\n> \n")
		assert.False(t, ok, p)
	}
}

func TestDetectScreenTurnEnd_Should_FlagAmbiguousClaudeFinishedForTiebreak(t *testing.T) {
	t.Parallel()
	pane := "Why does this matter?\n\n> \n? for shortcuts\n"
	got, ok := DetectScreenTurnEnd("claude", detection.StatusIdle, claudeIdleCtx, pane)
	assert.True(t, ok)
	assert.Equal(t, TurnEndKindFinished, got.Kind)
}
