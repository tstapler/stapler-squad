package git

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllocateAdminDirName_FreshName covers Epic 1.2's Story 1.2.1 first acceptance
// criterion: a name with no existing collision gets that exact name, no suffix.
func TestAllocateAdminDirName_FreshName(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	got, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x"), got)
	info, statErr := os.Stat(got)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())
}

// TestAllocateAdminDirName_CollidingName_GetsSuffix covers Story 1.2.1's second
// acceptance criterion: a colliding name gets the numeric suffix real git's own
// add_worktree would produce ("<name>1"), confirmed against builtin/worktree.c in
// Task 1.2.1a.
func TestAllocateAdminDirName_CollidingName_GetsSuffix(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	first, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	second, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x1"), second)
	info, statErr := os.Stat(second)
	require.NoError(t, statErr)
	assert.True(t, info.IsDir())

	third, err := AllocateAdminDirName(repoPath, "feature-x")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(repoPath, ".git", "worktrees", "feature-x2"), third)
}

// TestAllocateAdminDirName_should_ReturnError_When_RetryCapExceeded covers
// validation.md's P1 error-path row: pre-creating "feature-x" and every numeric-suffix
// name up through the retry cap forces AllocateAdminDirName to return a bounded error
// instead of looping forever.
func TestAllocateAdminDirName_should_ReturnError_When_RetryCapExceeded(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()
	worktreesDir := filepath.Join(repoPath, ".git", "worktrees")
	require.NoError(t, os.MkdirAll(worktreesDir, 0o777))

	require.NoError(t, os.Mkdir(filepath.Join(worktreesDir, "feature-x"), 0o777))
	for i := 1; i <= maxAllocateAdminDirNameRetries; i++ {
		name := fmt.Sprintf("feature-x%d", i)
		require.NoError(t, os.Mkdir(filepath.Join(worktreesDir, name), 0o777))
	}

	_, err := AllocateAdminDirName(repoPath, "feature-x")
	require.Error(t, err)
}

// TestAllocateAdminDirName_ConcurrentCallers_NeverCollide covers Task 1.2.1c's
// concurrency requirement: N goroutines racing AllocateAdminDirName with the same base
// name against one repo must all get distinct, existing paths — proof the os.Mkdir +
// EEXIST-retry loop is actually race-free, not just correct single-threaded.
func TestAllocateAdminDirName_ConcurrentCallers_NeverCollide(t *testing.T) {
	t.Parallel()
	repoPath := t.TempDir()

	const goroutines = 20
	results := make([]string, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = AllocateAdminDirName(repoPath, "feature-x")
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, goroutines)
	for i := range goroutines {
		require.NoError(t, errs[i])
		require.False(t, seen[results[i]], "path %q returned to more than one caller", results[i])
		seen[results[i]] = true

		info, statErr := os.Stat(results[i])
		require.NoError(t, statErr)
		assert.True(t, info.IsDir())
	}
	assert.Len(t, seen, goroutines)
}
