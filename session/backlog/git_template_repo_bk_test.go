package backlog

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/backlog/internal/blktest"
)

func initialCommitRepo(t *testing.T, dir, branch string) { blktest.InitialCommitRepo(t, dir, branch) }

func initConfiguredRepo(t *testing.T, dir, branch string) {
	blktest.InitConfiguredRepo(t, dir, branch)
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	blktest.CommitFile(t, dir, name, content, msg)
}

func headCommitSHA(t *testing.T, dir string) string {
	t.Helper()
	return blktest.HeadCommitSHA(t, dir)
}
