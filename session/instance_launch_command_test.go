package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session/launchcmd"
)

func TestLaunchCommand_ClaudeResumeIsShellFreeArgv(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "claude", UUID: "uuid-argv", AutoYes: true}
	const id = "a364f374-4c68-4dd0-a836-770a191973e5"

	cmd := inst.launchCommand(id)

	assert.False(t, cmd.RequiresShell())
	prog, ok := cmd.Program()
	require.True(t, ok)
	assert.Equal(t, "claude", prog)
	args, err := cmd.Args(os.ReadFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"--resume", id, "--permission-mode", PermissionModeBypassPermissions}, args)
}

func TestLaunchCommand_HostileResumeIDStaysOneArgument(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "claude", UUID: "uuid-hostile"}
	const hostile = "x'; touch /tmp/pwned; echo '"

	cmd := inst.launchCommand(hostile)

	args, err := cmd.Args(os.ReadFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"--resume", hostile}, args)
	assert.Equal(t, `claude --resume 'x'\''; touch /tmp/pwned; echo '\'''`, cmd.ShellString())
}

func TestLaunchCommand_ProgramNeedingAShellStaysRawShell(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := &Instance{Program: "$HOME/bin/claude --model sonnet"}

	cmd := inst.launchCommand("")

	assert.True(t, cmd.RequiresShell())
	assert.Equal(t, "$HOME/bin/claude --model sonnet", cmd.ShellString())
	_, err := cmd.Args(os.ReadFile)
	assert.ErrorIs(t, err, launchcmd.ErrRequiresShell)
}

func TestLaunchCommand_PiStderrIsStructuredNotARedirectString(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	inst := &Instance{Title: "pi-stderr", Program: "pi"}

	cmd := inst.launchCommand("")

	assert.NotEmpty(t, cmd.StderrAppend)
	args, err := cmd.Args(os.ReadFile)
	require.NoError(t, err)
	assert.Empty(t, args, "the redirect must not leak into argv")
}
