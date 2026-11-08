package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A mass move-with-edit bigger than the rename limit is reported as delete+add
// pairs instead of paying go-git's O(added x deleted) blob-similarity matrix
// (28% of the live service's CPU in a 20s pprof window when this ran unbounded).
func TestDiffStatBetween_ShouldSkipContentRenameDetection_WhenMoreFilesThanRenameLimit(t *testing.T) {
	t.Parallel()
	origin := setupTestRepo(t)
	work := cloneTestRepo(t, origin)

	const n = 120 // above diffStatRenameLimit
	require.NoError(t, os.MkdirAll(filepath.Join(work, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(work, "b"), 0o755))
	for i := 0; i < n; i++ {
		body := strings.Repeat(fmt.Sprintf("file %d unique line\n", i), 8)
		require.NoError(t, os.WriteFile(filepath.Join(work, "a", fmt.Sprintf("f%03d.txt", i)), []byte(body), 0o644))
	}
	runGit(t, work, "add", "a")
	runGit(t, work, "commit", "-m", "add files")
	baseSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	for i := 0; i < n; i++ {
		src := filepath.Join(work, "a", fmt.Sprintf("f%03d.txt", i))
		dst := filepath.Join(work, "b", fmt.Sprintf("g%03d.txt", i))
		body, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.Remove(src))
		require.NoError(t, os.WriteFile(dst, append(body, []byte("one edited line\n")...), 0o644))
	}
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-m", "move and edit")
	headSHA := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	start := time.Now()
	stat, err := DiffStatBetween(context.Background(), work, baseSHA, headSHA)
	require.NoError(t, err)
	t.Logf("DiffStatBetween over %d moved+edited files took %s", n, time.Since(start))
	assert.Equal(t, 2*n, stat.FilesChanged, "beyond the rename limit every move is a delete plus an add")
}
