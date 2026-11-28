package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// ---------------------------------------------------------------------------
// CreateDebugSnapshot
// ---------------------------------------------------------------------------

// TestCreateDebugSnapshot_Succeeds verifies that CreateDebugSnapshot writes a
// snapshot file and returns a non-empty file path. The poller is nil so
// instances defaults to an empty list; the call must still succeed.
func TestCreateDebugSnapshot_Succeeds(t *testing.T) {
	t.Parallel()
	svc := setupUtilityService()

	resp, err := svc.CreateDebugSnapshot(context.Background(), connect.NewRequest(&sessionv1.CreateDebugSnapshotRequest{}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Msg.FilePath, "FilePath must be non-empty after a successful snapshot")
	assert.Greater(t, resp.Msg.FileSizeBytes, int64(0), "snapshot file must have non-zero size")
}

// TestCreateDebugSnapshot_WithNote verifies that CreateDebugSnapshot accepts
// an optional note without error.
func TestCreateDebugSnapshot_WithNote(t *testing.T) {
	t.Parallel()
	svc := setupUtilityService()

	note := "test note for snapshot"
	resp, err := svc.CreateDebugSnapshot(context.Background(), connect.NewRequest(&sessionv1.CreateDebugSnapshotRequest{
		Note: &note,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Msg.FilePath)
}
