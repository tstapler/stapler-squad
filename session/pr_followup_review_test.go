package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deterministically bad link (zero ref) must not roll back the rest of a session save.
func TestUpsertSessionPRSetTx_ShouldKeepGoodLinks_WhenOneLinkIsDeterministicallyInvalid(t *testing.T) {
	t.Parallel()
	repo := newPRRepo(t)
	ctx := context.Background()
	data := InstanceData{UUID: "u-skip", PRs: []PRView{
		{Ref: mustPRRef(t, "", "acme", "app", 1), AttachSource: AttachSourceManual},
		{AttachSource: AttachSourceManual}, // zero ref
		{Ref: mustPRRef(t, "", "acme", "app", 3), AttachSource: AttachSourceManual},
	}}
	tx, err := repo.Client.Tx(ctx)
	require.NoError(t, err)
	require.NoError(t, upsertSessionPRSetTx(ctx, tx, data))
	require.NoError(t, tx.Commit())
	assert.Equal(t, 2, countLinks(t, repo))
}
