package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestCancelGuidanceRequest_should_PersistReason(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	row, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_01", "Which cache?"), 0)
	require.NoError(t, err)
	id := uuid.MustParse(row.ID)

	applied, err := repo.CancelGuidanceRequest(ctx, id, domain.CancelReasonSessionEnded.String())
	require.NoError(t, err)
	assert.True(t, applied)
	got, err := repo.GetGuidanceRequest(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.CancelReasonSessionEnded, got.CancelReason)
}

func TestPurgeTerminalGuidanceQuestions_should_ClearOnlyTerminalRowsOlderThanCutoff(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	open, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_open", "Open?"), 0)
	require.NoError(t, err)
	done, err := repo.CreateMirroredGuidanceRequest(ctx, hookRequest(t, "toolu_done", "Done?"), 0)
	require.NoError(t, err)
	_, err = repo.CancelGuidanceRequest(ctx, uuid.MustParse(done.ID), domain.CancelReasonDialogGone.String())
	require.NoError(t, err)

	n, err := repo.PurgeTerminalGuidanceQuestions(ctx, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n, "a row cancelled just now is inside the retention window")

	n, err = repo.PurgeTerminalGuidanceQuestions(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	gotDone, err := repo.GetGuidanceRequest(ctx, uuid.MustParse(done.ID))
	require.NoError(t, err)
	assert.Empty(t, gotDone.QuestionsJSON)
	assert.Equal(t, "Done?", gotDone.QuestionText)
	gotOpen, err := repo.GetGuidanceRequest(ctx, uuid.MustParse(open.ID))
	require.NoError(t, err)
	assert.NotEmpty(t, gotOpen.QuestionsJSON, "an open row is never purged")
}
