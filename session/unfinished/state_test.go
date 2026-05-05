package unfinished

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// newTestStateStore creates a StateStore backed by a temp file.
func newTestStateStore(t *testing.T) (*StateStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, err := NewStateStore(path)
	require.NoError(t, err)
	return store, path
}

// ---- UT-021: Dismiss persists across reload --------------------------------

func TestStateDismissPersistsAcrossReload(t *testing.T) {
	// Use a real directory so cleanupStaleEntries doesn't remove the entry.
	repoDir := t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, err := NewStateStore(path)
	require.NoError(t, err)

	err = store.Dismiss(repoDir, "main")
	require.NoError(t, err)
	assert.True(t, store.IsDismissed(repoDir, "main"))
	assert.False(t, store.IsDismissed(repoDir, "feature"))

	// Reload from disk.
	store2, err := NewStateStore(path)
	require.NoError(t, err)
	assert.True(t, store2.IsDismissed(repoDir, "main"), "dismiss should survive reload")
	assert.False(t, store2.IsDismissed(repoDir, "feature"))
}

func TestStateDismissIdempotent(t *testing.T) {
	repoDir := t.TempDir()
	store, _ := newTestStateStore(t)
	require.NoError(t, store.Dismiss(repoDir, "main"))
	require.NoError(t, store.Dismiss(repoDir, "main")) // second call is no-op
	store.mu.RLock()
	count := 0
	for _, d := range store.state.Dismissed {
		if d.RepoPath == repoDir && d.Branch == "main" {
			count++
		}
	}
	store.mu.RUnlock()
	assert.Equal(t, 1, count, "duplicate dismiss must not create two entries")
}

func TestStateUndismiss(t *testing.T) {
	repoDir := t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, err := NewStateStore(path)
	require.NoError(t, err)

	require.NoError(t, store.Dismiss(repoDir, "main"))
	require.NoError(t, store.Undismiss(repoDir, "main"))
	assert.False(t, store.IsDismissed(repoDir, "main"))

	// Reload confirms removal.
	store2, err := NewStateStore(path)
	require.NoError(t, err)
	assert.False(t, store2.IsDismissed(repoDir, "main"))
}

// ---- UT-022: Snooze auto-clears when SHA changes --------------------------

func TestStateSnoozeAutoClears(t *testing.T) {
	store, _ := newTestStateStore(t)

	const sha1 = "abc123"
	const sha2 = "def456"

	require.NoError(t, store.Snooze("/repo/b", "feature", sha1))
	assert.True(t, store.IsSnoozed("/repo/b", "feature", sha1))

	// Same SHA — still snoozed.
	assert.True(t, store.IsSnoozed("/repo/b", "feature", sha1))

	// Different SHA — auto-clears.
	assert.False(t, store.IsSnoozed("/repo/b", "feature", sha2))

	// Now the snooze is gone.
	assert.False(t, store.IsSnoozed("/repo/b", "feature", sha1))
}

func TestStateUnsnoozePersists(t *testing.T) {
	repoDir := t.TempDir()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, err := NewStateStore(path)
	require.NoError(t, err)

	require.NoError(t, store.Snooze(repoDir, "main", "sha-xyz"))
	require.NoError(t, store.Unsnooze(repoDir, "main"))
	assert.False(t, store.IsSnoozed(repoDir, "main", "sha-xyz"))

	store2, err := NewStateStore(path)
	require.NoError(t, err)
	assert.False(t, store2.IsSnoozed(repoDir, "main", "sha-xyz"))
}

// ---- UT-023: Cleanup removes stale dismissed/snoozed entries ---------------

func TestStateCleanupRemovesStaleEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write a state file with both a real and a non-existent repo.
	realRepo := dir // dir always exists
	ghostRepo := filepath.Join(dir, "ghost-that-does-not-exist")

	store, err := NewStateStore(path)
	require.NoError(t, err)
	// Inject entries directly to bypass OS stat check in Dismiss.
	store.mu.Lock()
	store.state.Dismissed = append(store.state.Dismissed,
		dismissEntry{RepoPath: realRepo, Branch: "main", DismissedAt: time.Now()},
		dismissEntry{RepoPath: ghostRepo, Branch: "feature", DismissedAt: time.Now()},
	)
	store.state.Snoozed = append(store.state.Snoozed,
		snoozeEntry{RepoPath: ghostRepo, Branch: "main", SnoozeSinceSHA: "abc"},
	)
	require.NoError(t, store.save())
	store.mu.Unlock()

	// Reload triggers cleanupStaleEntries.
	store2, err := NewStateStore(path)
	require.NoError(t, err)
	assert.True(t, store2.IsDismissed(realRepo, "main"), "real repo dismissed entry should remain")
	assert.False(t, store2.IsDismissed(ghostRepo, "feature"), "ghost dismissed entry should be cleaned up")
	assert.False(t, store2.IsSnoozed(ghostRepo, "main", "abc"), "ghost snoozed entry should be cleaned up")
}

// ---- UT-024: Atomic write (temp file + rename) ----------------------------

func TestStateAtomicWrite(t *testing.T) {
	store, path := newTestStateStore(t)
	require.NoError(t, store.Dismiss("/repo/a", "main"))

	// Verify the .tmp file is gone after save completes.
	_, err := os.Stat(path + ".tmp")
	assert.True(t, os.IsNotExist(err), "temp file should be cleaned up after atomic rename")

	// Verify the final file is valid JSON.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "/repo/a")
}

// ---- UT-025: AI summary cache round-trip and TTL --------------------------

func TestStateAISummaryCacheRoundTrip(t *testing.T) {
	store, path := newTestStateStore(t)

	// Miss on empty cache.
	_, ok := store.GetCachedSummary("/repo/c", "main", "hash1")
	assert.False(t, ok, "cache miss expected on empty store")

	// Store a summary.
	require.NoError(t, store.CacheSummary("/repo/c", "main", "hash1", "This is a summary."))
	summary, ok := store.GetCachedSummary("/repo/c", "main", "hash1")
	assert.True(t, ok)
	assert.Equal(t, "This is a summary.", summary)

	// Different hash — miss.
	_, ok = store.GetCachedSummary("/repo/c", "main", "hash2")
	assert.False(t, ok)

	// Reload verifies persistence.
	store2, err := NewStateStore(path)
	require.NoError(t, err)
	summary2, ok2 := store2.GetCachedSummary("/repo/c", "main", "hash1")
	assert.True(t, ok2)
	assert.Equal(t, "This is a summary.", summary2)
}

func TestStateAISummaryCacheEvictsExpired(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewStateStore(path)
	require.NoError(t, err)

	// Inject an expired entry directly.
	store.mu.Lock()
	store.state.AICache = []aiCacheEntry{
		{
			RepoPath:    "/repo/d",
			Branch:      "main",
			DiffHash:    "hash-old",
			Summary:     "old summary",
			GeneratedAt: time.Now().Add(-25 * time.Hour),
		},
	}
	require.NoError(t, store.save())
	store.mu.Unlock()

	// Reload: expired entry should be stripped on Load.
	store2, err := NewStateStore(path)
	require.NoError(t, err)
	_, ok := store2.GetCachedSummary("/repo/d", "main", "hash-old")
	assert.False(t, ok, "expired AI cache entry should be evicted on reload")
}

// ---- UT-027: ComputeDiffHash — same diff produces same hash ----------------

func TestComputeDiffHashSameOutput(t *testing.T) {
	// Create a minimal git repo so `git diff HEAD` works.
	dir := t.TempDir()

	gitCmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@example.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range gitCmds {
		out, err := runCmd(args[0], args[1:]...)
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}

	// Create and commit a file.
	filePath := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello\n"), 0644))
	_, err := runCmd("git", "-C", dir, "add", ".")
	require.NoError(t, err)
	_, err = runCmd("git", "-C", dir, "commit", "-m", "init")
	require.NoError(t, err)

	// Modify the file (unstaged).
	require.NoError(t, os.WriteFile(filePath, []byte("hello world\n"), 0644))

	hash1, err := ComputeDiffHash(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)

	hash2, err := ComputeDiffHash(dir)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2, "identical diff should produce identical hash")

	// Modify further — hash must differ.
	require.NoError(t, os.WriteFile(filePath, []byte("completely different\n"), 0644))
	hash3, err := ComputeDiffHash(dir)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3, "different diff should produce different hash")
}

// runCmd is a test helper that runs a command and returns combined output.
func runCmd(name string, args ...string) (string, error) {
	cmd := safeexec.CommandContext(context.Background(), name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
