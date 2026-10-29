package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func TestPRSnapshotAdapter_should_MapCheckConclusionAndChangesRequested_When_SessionSnapshotRead(t *testing.T) {
	checked := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	sn := prSnap("a", session.Active)
	sn.GitHub.GitHubCheckConclusion = "failure"
	sn.GitHub.GitHubChangesReqCount = 2
	sn.GitHub.GitHubPRStatusTerminal = true
	sn.GitHub.LastPRStatusCheck = checked
	a := newPRWatchSnapshots(func() []*session.InstanceSnapshot {
		return []*session.InstanceSnapshot{prSnap("b", session.Active), sn}
	})

	got, err := a.Snapshot("a")
	require.NoError(t, err)
	assert.Equal(t, "failure", got.CheckConclusion)
	assert.Equal(t, 2, got.ChangesRequested)
	assert.True(t, got.Terminal)
	assert.True(t, got.CheckedAt.Equal(checked))
}

func TestPRSnapshotAdapter_should_ReturnError_When_SessionUnknown(t *testing.T) {
	a := newPRWatchSnapshots(func() []*session.InstanceSnapshot { return nil })
	_, err := a.Snapshot("missing")
	assert.Error(t, err)
}
