package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStorage_IsDiagnoseCaller is the direct unit test for gap #1's
// session-identity source of truth: server/mcp's denyIfDiagnoseCaller relies
// on this to decide whether a caller session is a dispatched Diagnose &
// Nudge investigation, so it must correctly distinguish a diagnose-role
// ItemSession from every other role and from "no link at all" — the latter
// must read as false (not diagnose), not an error, since an unidentifiable
// caller must fall through to the pre-existing unrestricted behavior.
func TestStorage_IsDiagnoseCaller(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	t.Cleanup(cleanup)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "diag test item"})
	require.NoError(t, err)

	diagnoseSessUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: diagnoseSessUUID,
		SessionRole: SessionRoleDiagnose,
	})
	require.NoError(t, err)

	workSessUUID := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: workSessUUID,
		SessionRole: SessionRoleWork,
	})
	require.NoError(t, err)

	assert.True(t, storage.IsDiagnoseCaller(ctx, diagnoseSessUUID), "a diagnose-role ItemSession must be identified")
	assert.False(t, storage.IsDiagnoseCaller(ctx, workSessUUID), "a work-role ItemSession must not be identified as diagnose")
	assert.False(t, storage.IsDiagnoseCaller(ctx, uuid.New().String()), "a session with no ItemSession link at all must not be identified as diagnose")
}
