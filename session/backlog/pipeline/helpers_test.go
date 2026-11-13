package pipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog/internal/blktest"
)

func createTestStorage(t *testing.T) (*session.Storage, func()) {
	t.Helper()
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	return storage, func() {}
}

func captureWarningLog(t *testing.T) *log.SyncBuffer {
	t.Helper()
	return log.CaptureLogger(t, log.WarningLog(), "WARNING: ")
}

func runGitCapture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
	return string(out)
}

func initConfiguredRepo(t *testing.T, dir, branch string) {
	blktest.InitConfiguredRepo(t, dir, branch)
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	blktest.CommitFile(t, dir, name, content, msg)
}
