package tmux

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseClientActivity_ShouldSplitModeActivityAndSession_WhenOutputIsWellFormed(t *testing.T) {
	t.Parallel()
	got := ParseClientActivity("1 1700000000 4242 ssq_a\n0 1700000100 4343 my session\nbroken\n0 notanumber 1 x\n")

	assert.Equal(t, []ClientActivity{
		{Session: "ssq_a", ControlMode: true, Activity: time.Unix(1700000000, 0), PID: 4242},
		{Session: "my session", ControlMode: false, Activity: time.Unix(1700000100, 0), PID: 4343},
	}, got)
}

func TestClientActivityIsOurs_ShouldBeTrueOnlyForAChildOfThisProcess(t *testing.T) {
	t.Parallel()
	child := exec.Command("sleep", "30") //nolint:norawexec the test needs a real child process of this binary
	require.NoError(t, child.Start())
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })

	assert.True(t, ClientActivity{PID: int32(child.Process.Pid)}.IsOurs(), "ssq's own pty attach is a child of ssq") //nolint:gosec // test pid
	assert.False(t, ClientActivity{PID: int32(os.Getppid())}.IsOurs(), "a person's attach has a shell for a parent") //nolint:gosec // test pid
	assert.False(t, ClientActivity{}.IsOurs(), "unknown pid is not ours")
}

func TestParseClientActivity_ShouldReturnNothing_WhenOutputIsEmpty(t *testing.T) {
	t.Parallel()
	assert.Empty(t, ParseClientActivity(""))
}
