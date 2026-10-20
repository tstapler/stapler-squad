package unfinished

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

func TestStartupLog_ShouldEmitOneInfoWithParentsReaderAndFlagValues(t *testing.T) {
	t.Parallel()
	var lines [][]any
	logStartup(nil, func(msg string, args ...any) { lines = append(lines, append([]any{msg}, args...)) })

	require.Len(t, lines, 1, "exactly one INFO at startup")
	fields := map[string]any{}
	for i := 1; i+1 < len(lines[0]); i += 2 {
		fields[lines[0][i].(string)] = lines[0][i+1]
	}
	assert.Equal(t, "stock", fields["commit_parents_reader"])
	for _, k := range []string{"shared_commit_state", "diff_counter", "scan_skip", "watch_worktree_gitdirs", "shadow_verify_every"} {
		assert.Contains(t, fields, k)
	}
}

func TestStockParentsReader_ShouldCountOneDecodePerRead(t *testing.T) {
	// Not parallel: asserts a delta on a process-global counter.
	repo, _ := testrepo.NewRepo(t)
	hashes := testrepo.Linear(t, repo, 3)

	before := DecodedCommitsTotal()
	for _, h := range hashes {
		_, _, err := stockCommitParents.Parents(repo.Storer, h, nil)
		require.NoError(t, err)
	}
	assert.GreaterOrEqual(t, DecodedCommitsTotal()-before, int64(3))
}

func TestMetrics_ShouldExposeReachAndProductCaches_WhenRepoStateHasThem(t *testing.T) {
	t.Parallel()
	rs := repostate.NewRepoState(repostate.Options{}, 1<<20)
	t.Cleanup(rs.Close)
	require.Nil(t, rs.ReachCacheIfBuilt(), "reading must not allocate the reach cache")

	in := newRepostateInputs()
	in.addProducts(rs)
	assert.Nil(t, rs.ReachCacheIfBuilt(), "addProducts must not allocate the reach cache")
	assert.Zero(t, in.caches["reach"].Hits)

	rs.ReachCache()
	in.addProducts(rs)
	assert.NotNil(t, rs.ReachCacheIfBuilt())
}

func TestBuildReport_ShouldMarshalEveryCacheKey_WhenNothingRegistered(t *testing.T) {
	t.Parallel()
	r := BuildReport(nil)
	b, err := json.Marshal(r)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	for _, k := range []string{"scan_cycles_total", "flags", "caches", "parity_mismatch_total",
		"parity_transient_total", "shadow_verify", "decoded_commits_total", "dirty_full_walk_total",
		"scan_skip", "signature_skip", "stat_calls_per_scan"} {
		assert.Contains(t, out, k)
	}
	caches := out["caches"].(map[string]any)
	for _, c := range []string{"graph", "reach", "aheadbehind", "behind", "messages", "diffmemo", "index", "untracked"} {
		assert.Contains(t, caches, c)
	}
}
