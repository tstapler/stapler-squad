package safeexec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTmuxSubcommand(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"capture-pane":    {"-L", "sock", "capture-pane", "-p", "-t", "s"},
		"list-panes":      {"-S", "/tmp/x", "-f", "/dev/null", "list-panes", "-a"},
		"has-session":     {"has-session", "-t", "x"},
		"none":            {"-V"},
		"other":           {"-L", "s", "not-a-command"},
		"display-message": {"-2", "-L", "s", "display-message", "-p", "x"},
	}
	for want, args := range cases {
		assert.Equal(t, want, TmuxSubcommand(args), fmt.Sprint(args))
	}
}

func TestIsTmux(t *testing.T) {
	t.Parallel()
	assert.True(t, isTmux("/opt/homebrew/bin/tmux"))
	assert.True(t, isTmux("tmux"))
	assert.False(t, isTmux("git"))
	assert.False(t, isTmux("/usr/bin/ps"))
}

func TestLabelSetCapsDistinctValues(t *testing.T) {
	t.Parallel()
	s := newLabelSet(2)
	assert.Equal(t, "a", s.admit("a"))
	assert.Equal(t, "b", s.admit("b"))
	assert.Equal(t, spawnLabelOverflow, s.admit("c"))
	assert.Equal(t, "a", s.admit("a"), "an admitted value stays admitted")
}

func TestShortFuncStripsClosures(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "session.(*ReviewQueuePoller).Start", shortFunc("github.com/tstapler/stapler-squad/session.(*ReviewQueuePoller).Start.func1.2"))
	assert.Equal(t, "session.StartSessionDriver", shortFunc("github.com/tstapler/stapler-squad/session.StartSessionDriver.gowrap1"))
	assert.Equal(t, "session.run", shortFunc("github.com/tstapler/stapler-squad/session.run.func1.gowrap1"))
	assert.Equal(t, "session.(*T).M", shortFunc("github.com/tstapler/stapler-squad/session.(*T).M-fm"))
}
