package instance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type namedPM struct {
	noopProcessManager
	name string
}

func (p namedPM) GetSessionIdentifier() string { return p.name }

// The persisted shell name must be the one tmux.NewShellTmuxHandle creates.
func TestShellTmuxSessionName_should_ReplaceTargetSyntax_When_ParentWasAdopted(t *testing.T) {
	t.Parallel()
	inst := &Instance{}
	inst.SetProcessManagerForTest(namedPM{name: "ext.v1:2"})

	require.Equal(t, "ext_v1_2_shell_id", inst.shellTmuxSessionName("id"))
}
