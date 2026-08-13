package session

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetWithOptions verifies that GetWithOptions loads session data correctly.
// EntRepository always loads all data regardless of LoadOptions.
func TestSelectiveLoading(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	testData := InstanceData{
		Title:      "test-session",
		Path:       "/tmp/test",
		WorkingDir: "/tmp/test",
		Branch:     "main",
		Status:     Ready,
		Program:    "claude",
		Worktree: GitWorktreeData{
			RepoPath:      "/tmp/repo",
			WorktreePath:  "/tmp/worktree",
			SessionName:   "test-session",
			BranchName:    "feature",
			BaseCommitSHA: "abc123",
		},
		DiffStats: DiffStatsData{
			Added:   100,
			Removed: 50,
			Content: "diff content for test",
		},
		Tags: []string{"Frontend", "Urgent"},
		ClaudeSession: ClaudeSessionData{
			ConversationUUID: "claude-123",
			SquadSessionID:   "conv-456",
			ProjectName:      "my-project",
		},
	}

	err = repo.Create(ctx, testData)
	require.NoError(t, err)

	// EntRepository always loads full data regardless of LoadOptions
	for _, tc := range []struct {
		name string
		opts LoadOptions
	}{
		{"LoadMinimal", LoadMinimal},
		{"LoadSummary", LoadSummary},
		{"LoadFull", LoadFull},
		{"LoadDiffOnly", LoadDiffOnly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := repo.GetWithOptions(ctx, "test-session", tc.opts)
			require.NoError(t, err)

			assert.Equal(t, "test-session", session.Title)
			assert.Equal(t, "/tmp/test", session.Path)
			assert.Equal(t, Ready, session.Status)
			assert.Equal(t, "/tmp/repo", session.Worktree.RepoPath)
			assert.Equal(t, 100, session.DiffStats.Added)
			assert.ElementsMatch(t, []string{"Frontend", "Urgent"}, session.Tags)
			assert.Equal(t, "claude-123", session.ClaudeSession.ConversationUUID)
		})
	}

	// Verify Get and List also return correct data
	t.Run("Get returns full data", func(t *testing.T) {
		session, err := repo.Get(ctx, "test-session")
		require.NoError(t, err)
		assert.Equal(t, "test-session", session.Title)
		assert.Equal(t, 100, session.DiffStats.Added)
		assert.Contains(t, session.DiffStats.Content, "diff content for test")
	})

	t.Run("List returns all sessions", func(t *testing.T) {
		sessions, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "test-session", sessions[0].Title)
	})
}

// TestBuilderMethods verifies the fluent builder methods work correctly
func TestBuilderMethods(t *testing.T) {
	// Start with LoadMinimal and add what we need
	options := LoadMinimal.WithTags().WithDiffContent()

	assert.True(t, options.LoadTags, "WithTags should enable tag loading")
	assert.True(t, options.LoadDiffContent, "WithDiffContent should enable diff content loading")
	assert.True(t, options.LoadDiffStats, "WithDiffContent should imply LoadDiffStats")
	assert.False(t, options.LoadWorktree, "Should not enable worktree unless explicitly set")
	assert.False(t, options.LoadClaudeSession, "Should not enable Claude session unless explicitly set")

	// Remove what we don't need
	options = LoadFull.WithoutDiffContent()

	assert.True(t, options.LoadWorktree, "Should preserve worktree setting")
	assert.True(t, options.LoadTags, "Should preserve tags setting")
	assert.False(t, options.LoadDiffContent, "WithoutDiffContent should disable diff content")
}

// BenchmarkSelectiveLoading measures performance impact of selective loading
func BenchmarkSelectiveLoading(b *testing.B) {
	// Create temporary database
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	repo, err := NewEntRepository(WithDatabasePath(dbPath))
	if err != nil {
		b.Fatal(err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Create a session with large diff content (simulate 1MB diff)
	largeDiff := make([]byte, 1024*1024) // 1 MB
	for i := range largeDiff {
		largeDiff[i] = 'x'
	}

	testData := InstanceData{
		Title:      "bench-session",
		Path:       "/tmp/test",
		WorkingDir: "/tmp/test",
		Branch:     "main",
		Status:     Ready,
		Program:    "claude",
		DiffStats: DiffStatsData{
			Added:   5000,
			Removed: 3000,
			Content: string(largeDiff),
		},
		Tags: []string{"Frontend", "Backend", "DevOps"},
	}

	err = repo.Create(ctx, testData)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("LoadMinimal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadMinimal)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LoadSummary", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadSummary)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("LoadFull", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := repo.GetWithOptions(ctx, "bench-session", LoadFull)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
