package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ─── ReviewContextExtras rendering ──────────────────────────────────────────

// runGit runs a git command in dir for test setup, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, out)
}

// ─── BuildReviewCallOptions ─────────────────────────────────────────────────

// ─── ParseHeadlessToolReads ─────────────────────────────────────────────────

// ─── verifyToolReadsExist ───────────────────────────────────────────────────

// ─── DegradeIfUnverified ────────────────────────────────────────────────────
