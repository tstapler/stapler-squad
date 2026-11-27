package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services/access"
)

func TestNewBacklogSteerWriter_ShouldReturnErrReadOnly_WhenLinkIsZeroOrRunnerIsNil(t *testing.T) {
	t.Parallel()
	inst := hiddenInstance(t, true)
	link := BacklogReviewLink{inst: inst, itemID: "item-1"}
	runner := &SessionService{}

	w, err := newBacklogSteerWriter(access.TerminalReadOnly, BacklogReviewLink{}, runner)
	assert.Nil(t, w)
	assert.ErrorIs(t, err, access.ErrReadOnly, "a zero link (nobody resolved it) yields no writer")

	w, err = newBacklogSteerWriter(access.TerminalReadOnly, link, nil)
	assert.Nil(t, w)
	assert.ErrorIs(t, err, access.ErrReadOnly, "no runner yields no writer")

	w, err = newBacklogSteerWriter(access.TerminalReadOnly, link, runner)
	require.NoError(t, err)
	assert.NotNil(t, w)
}
