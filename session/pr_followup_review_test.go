package session

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/internal/sessiontest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// A deterministically bad link (zero ref) must not roll back the rest of a session save.
func TestUpsertSessionPRSetTx_ShouldKeepGoodLinks_WhenOneLinkIsDeterministicallyInvalid(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	data := domain.InstanceData{UUID: "u-skip", PRs: []domain.PRView{
		{Ref: sessiontest.MustPRRef(t, "", "acme", "app", 1), AttachSource: domain.AttachSourceManual},
		{AttachSource: domain.AttachSourceManual}, // zero ref
		{Ref: sessiontest.MustPRRef(t, "", "acme", "app", 3), AttachSource: domain.AttachSourceManual},
	}}
	tx, err := repo.Client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, upsertSessionPRSetTx(ctx, tx, data))
	require.NoError(t, tx.Commit())
	assert.Equal(t, 2, countLinks(t, repo))
}
