package escapedb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/ent"
)

// openForTest opens a fresh escape-events database under dir and closes it with the test.
func openForTest(ctx context.Context, t *testing.T, dir string) *ent.Client {
	t.Helper()
	client, _, err := Open(ctx, dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}
