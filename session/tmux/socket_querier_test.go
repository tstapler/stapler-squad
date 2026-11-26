package tmux

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealSocketQuerier_ListSessions_DelegatesToTmuxBinary(t *testing.T) {
	writeFakeTmux(t, `printf 'alpha\nbeta\n'
`)

	got, err := NewRealSocketQuerier().ListSessions("")

	require.NoError(t, err)
	require.Equal(t, map[string]bool{"alpha": true, "beta": true}, got)
}

func TestRealSocketQuerier_ListSessions_ReportsServerDown(t *testing.T) {
	writeFakeTmux(t, `echo "no server running on /tmp/tmux-1000/default" >&2
exit 1
`)

	_, err := NewRealSocketQuerier().ListSessions("")

	require.ErrorIs(t, err, ErrServerDown)
}

func TestRealSocketQuerier_IsServerDown_DelegatesToTmuxBinary(t *testing.T) {
	writeFakeTmux(t, `echo "no server running on /tmp/tmux-1000/default" >&2
exit 1
`)
	require.True(t, NewRealSocketQuerier().IsServerDown(""))

	writeFakeTmux(t, "exit 0\n")
	require.False(t, NewRealSocketQuerier().IsServerDown(""))
}
