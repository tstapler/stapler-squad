package binaries

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openCodeFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "panes", "opencode", "permission_bash.txt"))
	require.NoError(t, err)
	return string(b)
}

func TestOpenCodeProfile_Should_ExtractOptionsAndRejectForgedFooter_When_FixtureAndForgedVariant(t *testing.T) {
	p := NewOpenCodeQuestionProfile()
	real := openCodeFixture(t)

	d, ok := p.Extract(real)
	require.True(t, ok)
	assert.Equal(t, []string{"Allow once", "Allow always", "Reject"}, d.Options)
	assert.Equal(t, "opencode", d.Profile)
	assert.Equal(t, "# Shell command $ echo hi", d.Question)
	assert.True(t, p.Verify(real, d))

	buttons := "┃   Allow once   Allow always   Reject"
	transcriptCopy := real[strings.Index(real, "┃  △ Permission required"):]
	composer := "\n  ┃  Ask anything\n  ┃\n"
	for name, forged := range map[string]string{
		"numbered row appended":    real + "\n  ┃   1. Allow once\n",
		"plain text appended":      real + "\n  Allow once is fine\n",
		"copy in scrollback":       real + "\n" + transcriptCopy + composer,
		"buttons without header":   strings.Replace(real, "△ Permission required", "Permission required", 1),
		"header too far above":     strings.Replace(real, "# Shell command", strings.Repeat("┃  x\n", 9)+"  ┃  # Shell command", 1),
		"buttons reordered":        strings.Replace(real, buttons[strings.Index(buttons, "Allow once"):], "Reject   Allow always   Allow once", 1),
		"extra button":             strings.Replace(real, "Reject", "Reject   Always deny", 1),
		"empty capture":            "",
		"buttons in a status line": "  ┃  Allow once   Allow always   Reject   (docs)",
	} {
		_, ok := p.Extract(forged)
		assert.False(t, ok, name)
	}
}

func TestOpenCodeProfile_Should_VerifyOnlyTheSameRequest(t *testing.T) {
	p := NewOpenCodeQuestionProfile()
	real := openCodeFixture(t)
	d, ok := p.Extract(real)
	require.True(t, ok)
	other := strings.ReplaceAll(real, "$ echo hi", "$ rm -rf build")
	assert.False(t, p.Verify(other, d), "a different command is a different dialog")
	assert.False(t, p.Verify("", d))
}

func TestOpenCodeProfile_Should_OfferOnlyTheMeasuredKeystroke(t *testing.T) {
	p := NewOpenCodeQuestionProfile()
	d, ok := p.Extract(openCodeFixture(t))
	require.True(t, ok)
	keys, ok := p.AnswerKeys(0, d)
	assert.True(t, ok)
	assert.Equal(t, "\r", keys, "Enter on the default button; digits do nothing in 1.18.30")
	for _, i := range []int{1, 2, 3, -1} {
		_, ok := p.AnswerKeys(i, d)
		assert.False(t, ok, "option %d keystroke was never measured", i)
	}
}
