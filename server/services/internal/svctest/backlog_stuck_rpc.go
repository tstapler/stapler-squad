package svctest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// seedOpenStuckRow creates a backlog item and inserts an open BacklogStuckState
// row directly via the ent client (bypassing MarkStuck/its status precondition,
// since these RPC-level tests only need a row to exist, not the reconciler's
// write path — that path is covered by session/ent_repository_backlog_stuck_test.go).
func SeedOpenStuckRow(t *testing.T, storage *session.Storage, itemID string, reason domain.StuckReason, firstDetectedAt time.Time, stuckContext string) {
	t.Helper()
	client := storage.GetEntClient()
	require.NotNil(t, client)

	parsedID, err := uuid.Parse(itemID)
	require.NoError(t, err)

	now := time.Now()
	err = client.BacklogStuckState.Create().
		SetItemID(parsedID).
		SetReason(string(reason)).
		SetFirstDetectedAt(firstDetectedAt).
		SetLastCheckedAt(now).
		SetContext(stuckContext).
		Exec(context.Background())
	require.NoError(t, err)
}
