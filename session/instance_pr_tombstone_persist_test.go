package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A tombstoned attach is dropped from memory, which can zero the legacy scalars;
// the stored columns must follow or the stale number reloads as the active PR.
func TestPersistPRAttach_ShouldClearStoredLegacyNumber_WhenTombstonedAttachLeavesNoLinks(t *testing.T) {
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	ctx := context.Background()
	require.NoError(t, storage.PersistPRMirror(inst.UUID, "https://github.com/acme/app/pull/7", 7, false))
	inst.GitHubPRNumber, inst.GitHubPRURL = 7, "https://github.com/acme/app/pull/7"

	ref := mustPRRef(t, "", "acme", "app", 13)
	_, err := storage.repo.UpsertSessionPR(ctx, SessionPROwner(inst.UUID), ref, PRLink{Source: AttachSourceManual})
	require.NoError(t, err)
	_, err = storage.repo.DeleteSessionPR(ctx, SessionPROwner(inst.UUID), ref)
	require.NoError(t, err)

	require.NoError(t, inst.AttachPR(ref, AttachSourceBranchDiscovery, PRInitial{State: PRStateOpen}))

	require.Zero(t, inst.Snapshot().GitHub.GitHubPRNumber, "memory cleared by the tombstone drop")
	got := sessionMirror(t, storage, "pr-session")
	assert.Zero(t, got.GitHubPRNumber, "stored legacy number must follow the in-memory clear")
	assert.Empty(t, got.GitHubPRURL)
}
