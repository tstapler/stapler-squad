package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/testutil/wait"
)

// The failed start must publish Stopped, not only assign the field.
func TestForkSession_should_PublishStopped_When_ForkedSessionFailsToStart(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	src, checkpointID := makeInstanceWithCheckpoint("fork-start-failure-src")
	// A regular file as the working directory: tmux cannot start there.
	src.Path = filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(src.Path, nil, 0o600))
	fix.poller.AddInstance(src)

	_, err := fix.svc.ForkSession(context.Background(), connect.NewRequest(&sessionv1.ForkSessionRequest{
		SessionId:    src.Title,
		CheckpointId: checkpointID,
		NewTitle:     "fork-start-failure",
	}))
	require.NoError(t, err)

	forked := fix.svc.findInstance("fork-start-failure")
	require.NotNil(t, forked)
	wait.RequireEventually(t, func() bool {
		return forked.GetLifecycleStatus() == domain.Stopped
	}, 10*time.Second, 10*time.Millisecond, "published status after a failed start")
	assert.False(t, forked.Started())
}
