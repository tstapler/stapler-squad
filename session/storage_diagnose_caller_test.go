package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newItemSessionRoleFixture creates a fresh Storage-backed backlog item and
// links a new session UUID to it with the given role — shared setup for
// IsDiagnoseCaller/ClaimDiagnoseNudgeAttempt tests across this file and
// diagnose_nudge_dispatch_guard_test.go.
func newItemSessionRoleFixture(t *testing.T, role, title string) (storage *Storage, sessUUID string) {
	t.Helper()
	repo, cleanup := createTestEntRepository(t)
	t.Cleanup(cleanup)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: title})
	require.NoError(t, err)
	sessUUID = uuid.New().String()
	_, err = storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: sessUUID,
		SessionRole: role,
	})
	require.NoError(t, err)
	return storage, sessUUID
}

// TestStorage_IsDiagnoseCaller is the direct unit test for gap #1's
// session-identity source of truth: server/mcp's denyIfDiagnoseCaller relies
// on this to decide whether a caller session is a dispatched Diagnose &
// Nudge investigation, so it must correctly distinguish a diagnose-role
// ItemSession from every other role and from "no link at all" — the latter
// must read as false (not diagnose), not an error, since an unidentifiable
// caller must fall through to the pre-existing unrestricted behavior.
func TestStorage_IsDiagnoseCaller(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	diagnoseStorage, diagnoseSessUUID := newItemSessionRoleFixture(t, SessionRoleDiagnose, "diag test item")
	workStorage, workSessUUID := newItemSessionRoleFixture(t, SessionRoleWork, "work test item")

	assert.True(t, diagnoseStorage.IsDiagnoseCaller(ctx, diagnoseSessUUID), "a diagnose-role ItemSession must be identified")
	assert.False(t, workStorage.IsDiagnoseCaller(ctx, workSessUUID), "a work-role ItemSession must not be identified as diagnose")
	assert.False(t, diagnoseStorage.IsDiagnoseCaller(ctx, uuid.New().String()), "a session with no ItemSession link at all must not be identified as diagnose")
}
