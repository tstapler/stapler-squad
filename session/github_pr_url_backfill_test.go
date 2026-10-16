package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunGitHubPRURLBackfill_should_PopulateURL_When_HostRecoverableFromPath
// is the regression test for the bug this migration exists to fix: rows
// created before server/services/session_service.go's CreateSession handler
// started building github_pr_url at creation time have a known PR
// number/owner/repo but an empty URL. Recovering the host from the session's
// persisted path (following the GOPATH-style <host>/<owner>/<repo>
// convention) lets the backfill build the URL without guessing.
func TestRunGitHubPRURLBackfill_should_PopulateURL_When_HostRecoverableFromPath(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, InstanceData{
		Title:          "pr-463-traffic-capacitron",
		Path:           "/Users/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/pr-463",
		Status:         Stopped,
		Program:        "claude",
		CreatedAt:      now,
		UpdatedAt:      now,
		GitHubOwner:    "tstapler",
		GitHubRepo:     "stapler-squad",
		GitHubPRNumber: 463,
	}))

	require.NoError(t, runGitHubPRURLBackfill(ctx, repo))

	migrated, err := repo.Get(ctx, "pr-463-traffic-capacitron")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/463", migrated.GitHubPRURL)
}

// TestRunGitHubPRURLBackfill_should_SkipRow_When_HostNotRecoverableFromPath
// guards against guessing "github.com" for rows whose path carries no
// host-shaped segment (e.g. a plain local-directory session) — a wrong guess
// would silently produce an incorrect URL for GitHub Enterprise rows.
func TestRunGitHubPRURLBackfill_should_SkipRow_When_HostNotRecoverableFromPath(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, InstanceData{
		Title:          "local-dir-with-pr-metadata",
		Path:           "/Users/tstapler/dotfiles",
		Status:         Stopped,
		Program:        "claude",
		CreatedAt:      now,
		UpdatedAt:      now,
		GitHubOwner:    "tstapler",
		GitHubRepo:     "dotfiles",
		GitHubPRNumber: 7,
	}))

	require.NoError(t, runGitHubPRURLBackfill(ctx, repo))

	unmigrated, err := repo.Get(ctx, "local-dir-with-pr-metadata")
	require.NoError(t, err)
	assert.Empty(t, unmigrated.GitHubPRURL, "must not guess a host for a path with no recoverable host segment")
}

// TestRunGitHubPRURLBackfill_should_BeIdempotent_When_RunTwice proves a
// second run is a safe no-op — the query itself excludes rows that already
// have a github_pr_url, including ones the first run just populated.
func TestRunGitHubPRURLBackfill_should_BeIdempotent_When_RunTwice(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, InstanceData{
		Title:          "pr-99-idempotent-check",
		Path:           "/Users/tstapler/.stapler-squad/repos/github.com/tstapler/stapler-squad/pr-99",
		Status:         Stopped,
		Program:        "claude",
		CreatedAt:      now,
		UpdatedAt:      now,
		GitHubOwner:    "tstapler",
		GitHubRepo:     "stapler-squad",
		GitHubPRNumber: 99,
	}))

	require.NoError(t, runGitHubPRURLBackfill(ctx, repo))
	require.NoError(t, runGitHubPRURLBackfill(ctx, repo))

	final, err := repo.Get(ctx, "pr-99-idempotent-check")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/99", final.GitHubPRURL)
}
