package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// newTwoPRInstance returns a session with #11 merged (terminal) and #12 open,
// the shared fixture of the session-multi-pr-tracking headline tests (T1.3.4).
func newTwoPRInstance(t *testing.T) *Instance {
	t.Helper()
	inst := newPRTestInstance(t)
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 11), AttachSourceCreatedFromURL,
		PRInitial{State: PRStateMerged, Terminal: true}))
	require.NoError(t, inst.AttachPR(mustPRRef(t, "", "acme", "app", 12), AttachSourceBranchDiscovery,
		PRInitial{State: PRStateOpen}))
	return inst
}

// TestPRActions_DefaultTargetsActivePR is the headline test for PR actions
// (validation I-36): merge, close and comment with no explicit PR reach #12, the
// active PR, never the merged #11. MergePR and ClosePR shell out to `gh`, so the
// default target is asserted at the point every action resolves it.
func TestPRActions_DefaultTargetsActivePR(t *testing.T) {
	t.Skip("red until T4.2.1")
	inst := newTwoPRInstance(t)

	require.Equal(t, 12, inst.GitHub().PRNumber, "actions default to the active PR")
}

func TestResolvePR_ShouldReturnCurrentPRAndRejectNonPRSession(t *testing.T) {
	t.Parallel()
	got, err := newTwoPRInstance(t).resolvePR(nil)
	require.NoError(t, err)
	require.Equal(t, 12, got.Number())

	explicit := mustPRRef(t, "", "acme", "app", 11)
	got, err = newTwoPRInstance(t).resolvePR(&explicit)
	require.NoError(t, err)
	require.Equal(t, explicit, got)

	_, err = (&Instance{Title: "plain"}).resolvePR(nil)
	require.ErrorContains(t, err, "is not a PR session")
}
