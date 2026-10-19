package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseClientActivity_ShouldSplitModeActivityAndSession_WhenOutputIsWellFormed(t *testing.T) {
	t.Parallel()
	got := ParseClientActivity("1 1700000000 ssq_a\n0 1700000100 my session\nbroken\n0 notanumber x\n")

	assert.Equal(t, []ClientActivity{
		{Session: "ssq_a", ControlMode: true, Activity: time.Unix(1700000000, 0)},
		{Session: "my session", ControlMode: false, Activity: time.Unix(1700000100, 0)},
	}, got)
}

func TestParseClientActivity_ShouldReturnNothing_WhenOutputIsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, ParseClientActivity(""))
}
