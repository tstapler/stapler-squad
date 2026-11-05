package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
)

func attachMCPPR(t *testing.T, inst *session.Instance, n int, state session.PRState) {
	t.Helper()
	ref, err := session.NewPRRef("github.com", "acme", "app", n)
	require.NoError(t, err)
	require.NoError(t, inst.AttachPR(ref, session.AttachSourceManual, session.PRInitial{State: state, Terminal: state.Terminal()}))
}

// pause_session and stop_session stay ungated but name any open or unknown PR of
// the target session in their result.
func TestOpenPRWarnings_ShouldNameOpenAndUnknownPRs_AndStaySilentWhenAllTerminal(t *testing.T) {
	envtest.NewIsolatedStateDir(t)
	newInst := func(title string) *session.Instance {
		inst, err := session.NewInstance(session.InstanceOptions{Title: title, Path: t.TempDir()})
		require.NoError(t, err)
		inst.UUID = "uuid-" + title
		return inst
	}

	inst := newInst("open-pr-target")
	attachMCPPR(t, inst, 11, session.PRStateMerged)
	attachMCPPR(t, inst, 12, session.PRStateOpen)
	attachMCPPR(t, inst, 13, session.PRStateUnknown)
	got := openPRWarnings(inst, "stop_session")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "acme/app#12")
	assert.Contains(t, got[0], "acme/app#13")
	assert.NotContains(t, got[0], "#11", "a merged PR is not a reason to warn")
	assert.Contains(t, got[0], "stop_session")

	done := newInst("terminal-target")
	attachMCPPR(t, done, 11, session.PRStateMerged)
	attachMCPPR(t, done, 14, session.PRStateClosed)
	assert.Empty(t, openPRWarnings(done, "pause_session"))
}
