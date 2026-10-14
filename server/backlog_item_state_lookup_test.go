package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestBacklogItemStateLookup_should_ReportTerminalAndOpenStuckState(t *testing.T) {
	ctx := context.Background()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)

	done, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "done", Status: string(session.BacklogStatusDone)})
	require.NoError(t, err)
	stuck, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "stuck", Status: string(session.BacklogStatusInProgress)})
	require.NoError(t, err)
	_, err = storage.MarkStuck(ctx, stuck.ID, domain.StuckReasonPRNeedsFix, session.BacklogStatusInProgress, "ci failing")
	require.NoError(t, err)
	const missing = "00000000-0000-0000-0000-000000000001"

	states, err := backlogItemStateLookup(storage)(ctx, []string{done.ID, stuck.ID, missing})
	require.NoError(t, err)

	assert.True(t, states[done.ID].Terminal)
	assert.False(t, states[stuck.ID].Terminal)
	assert.True(t, states[stuck.ID].HasOpenStuck)
	assert.True(t, states[missing].Terminal, "a deleted item has nothing left to decide")
}
