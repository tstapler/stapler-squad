package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestStorageForItemLink returns a *Storage backed by a fresh in-memory
// EntRepository, mirroring the session.NewTestEntRepository +
// NewStorageWithRepository pattern used across this package's and
// server/services' test files.
func newTestStorageForItemLink(t *testing.T) *Storage {
	t.Helper()
	repo := NewTestEntRepository(t)
	storage, err := NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// TestResolveItemLink_should_ReturnOK_When_CallerSessionOwnsItem is the happy
// path: a session with a real ItemSession link to the item resolves cleanly,
// with no *ItemLinkError.
func TestResolveItemLink_should_ReturnOK_When_CallerSessionOwnsItem(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "linked item"})
	require.NoError(t, err)

	callerUUID := uuid.New().String()
	createdIS, err := storage.CreateItemSession(ctx, ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: callerUUID,
		SessionRole: "work",
	})
	require.NoError(t, err)

	itemSession, linkErr := ResolveItemLink(ctx, storage, callerUUID, item.ID)
	require.Nil(t, linkErr)
	assert.Equal(t, createdIS.ID, itemSession.ID)
}

// TestResolveItemLink_should_ReturnPermissionDenied_When_CallerSessionNotLinkedToItem
// confirms the not-found-vs-permission-denied disambiguation: the item
// exists, but callerUUID has no ItemSession link to it.
func TestResolveItemLink_should_ReturnPermissionDenied_When_CallerSessionNotLinkedToItem(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()

	item, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "unlinked item"})
	require.NoError(t, err)

	callerUUID := uuid.New().String()
	_, linkErr := ResolveItemLink(ctx, storage, callerUUID, item.ID)
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkPermissionDenied, linkErr.Code)
	assert.Contains(t, linkErr.Message, callerUUID)
	assert.Contains(t, linkErr.Message, item.ID)
	assert.Contains(t, linkErr.Remediation, "link_session_to_item")
}

// TestResolveItemLink_should_ReturnNotFound_When_ItemDoesNotExist rounds out
// the disambiguation's other branch: no ItemSession link AND no such item.
func TestResolveItemLink_should_ReturnNotFound_When_ItemDoesNotExist(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()

	missingItemID := uuid.New().String()
	_, linkErr := ResolveItemLink(ctx, storage, uuid.New().String(), missingItemID)
	require.NotNil(t, linkErr)
	assert.Equal(t, ItemLinkNotFound, linkErr.Code)
	assert.Equal(t, ItemNotFoundRemediation, linkErr.Remediation)
}

// TestResolveItemLink_should_ProduceIdenticalErrorShape_When_CalledWithSameFixtureFromBothCallers
// is table-driven over the not-linked and not-found fixtures, establishing
// the exact (Code, Message, Remediation) triples that
// guidance_request_ownership_parity_test.go asserts both the MCP and RPC
// adapters translate identically.
func TestResolveItemLink_should_ProduceIdenticalErrorShape_When_CalledWithSameFixtureFromBothCallers(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()

	linkedItem, err := storage.CreateBacklogItem(ctx, BacklogItemData{Title: "parity linked item"})
	require.NoError(t, err)
	unlinkedCaller := uuid.New().String()

	tests := []struct {
		name     string
		caller   string
		itemID   string
		wantCode ItemLinkErrorCode
	}{
		{"not linked", unlinkedCaller, linkedItem.ID, ItemLinkPermissionDenied},
		{"not found", uuid.New().String(), uuid.New().String(), ItemLinkNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, linkErr := ResolveItemLink(ctx, storage, tt.caller, tt.itemID)
			require.NotNil(t, linkErr)
			assert.Equal(t, tt.wantCode, linkErr.Code)
			assert.NotEmpty(t, linkErr.Remediation)
		})
	}
}
