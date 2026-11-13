package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func attachMCPPR(t *testing.T, inst *session.Instance, n int, state domain.PRState) {
	t.Helper()
	ref, err := domain.NewPRRef("github.com", "acme", "app", n)
	require.NoError(t, err)
	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceManual, session.PRInitial{State: state, Terminal: state.Terminal()}))
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
	attachMCPPR(t, inst, 11, domain.PRStateMerged)
	attachMCPPR(t, inst, 12, domain.PRStateOpen)
	attachMCPPR(t, inst, 13, domain.PRStateUnknown)
	got := openPRWarnings(inst, "stop_session")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "acme/app#12")
	assert.Contains(t, got[0], "acme/app#13")
	assert.NotContains(t, got[0], "#11", "a merged PR is not a reason to warn")
	assert.Contains(t, got[0], "stop_session")

	done := newInst("terminal-target")
	attachMCPPR(t, done, 11, domain.PRStateMerged)
	attachMCPPR(t, done, 14, domain.PRStateClosed)
	assert.Empty(t, openPRWarnings(done, "pause_session"))
}
