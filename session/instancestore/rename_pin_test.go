package instancestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

// The tmux session keeps the name it was created with, however often the row is renamed.
func TestInstances_should_PinTheFirstTitle_When_Renamed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	str := func(s string) *string { return &s }
	c := entdbtest.NewClient(t)
	seedBare(t, c, "first")
	store := New(c, Ports{})

	require.NoError(t, store.UpdateSessionMetadata(ctx, "first", str("second"), nil, nil, nil))
	require.NoError(t, store.UpdateSessionMetadata(ctx, "second", str("third"), nil, nil, nil))

	assert.Equal(t, "first", row(t, c, "third").TmuxTitle)
}
