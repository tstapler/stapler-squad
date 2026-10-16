package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A new session sharing a directory with a sibling must not adopt the sibling's
// (newer) conversation via the path fallback; it falls back to the next unclaimed
// one, or stays unlinked when none exists.
func TestHistoryLinker_CorrelateSession_SkipsConversationOwnedBySibling(t *testing.T) {
	t.Parallel()
	tempHome := t.TempDir()
	detector := NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, tempHome)

	repoPath := "/repo/shared"
	dir := filepath.Join(tempHome, ".claude", "projects", ClaudeProjectDirName(repoPath))
	require.NoError(t, os.MkdirAll(dir, 0755))

	siblingUUID := "cccccccc-0000-0000-0000-000000000000"
	unclaimedUUID := "dddddddd-0000-0000-0000-000000000000"
	unclaimedFile := filepath.Join(dir, unclaimedUUID+".jsonl")
	require.NoError(t, os.WriteFile(unclaimedFile, []byte("{}"), 0644))
	require.NoError(t, os.Chtimes(unclaimedFile, time.Now().Add(-time.Second), time.Now().Add(-time.Second)))
	require.NoError(t, os.WriteFile(filepath.Join(dir, siblingUUID+".jsonl"), []byte("{}"), 0644)) // newest

	sibling := &Instance{Title: "sibling", Path: repoPath, Status: Running}
	sibling.SetHistoryInfo(siblingUUID, filepath.Join(dir, siblingUUID+".jsonl"))
	fresh := &Instance{Title: "fresh", Path: repoPath, Status: Running}

	linker := NewHistoryLinker(detector, nil)
	linker.SetInstances([]*Instance{sibling, fresh})

	linker.correlateSession(fresh, false)
	require.True(t, fresh.HasClaudeSession())
	assert.Equal(t, unclaimedUUID, fresh.GetClaudeSession().ConversationUUID,
		"must skip the sibling's newer conversation")

	// With only the sibling's conversation left, the session stays unlinked.
	lonely := &Instance{Title: "lonely", Path: repoPath, Status: Running}
	require.NoError(t, os.Remove(unclaimedFile))
	linker.SetInstances([]*Instance{sibling, lonely})
	linker.correlateSession(lonely, false)
	assert.False(t, lonely.HasClaudeSession(), "must not adopt the sibling's conversation")
}

// A brand-new session must not adopt a conversation that began before it was
// created, even when nobody has linked that conversation yet and it is the newest
// file on disk (the original incident: a review session's resumed transcript).
func TestHistoryLinker_CorrelateSession_SkipsConversationPredatingSession(t *testing.T) {
	t.Parallel()
	tempHome := t.TempDir()
	detector := NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{files: []string{}}, tempHome)

	repoPath := "/repo/predate"
	dir := filepath.Join(tempHome, ".claude", "projects", ClaudeProjectDirName(repoPath))
	require.NoError(t, os.MkdirAll(dir, 0755))

	created := time.Now().Add(-time.Minute)
	entry := func(ts time.Time) []byte {
		return []byte(`{"type":"queue-operation","timestamp":"` + ts.UTC().Format(time.RFC3339Nano) + `"}` + "\n")
	}
	oldUUID := "eeeeeeee-0000-0000-0000-000000000000"
	ownUUID := "ffffffff-0000-0000-0000-000000000000"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ownUUID+".jsonl"), entry(created.Add(10*time.Second)), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, oldUUID+".jsonl"), entry(created.Add(-time.Hour)), 0644)) // newest mtime, started long ago

	fresh := &Instance{Title: "fresh", Path: repoPath, Status: Running, CreatedAt: created}
	linker := NewHistoryLinker(detector, nil)
	linker.SetInstances([]*Instance{fresh})

	linker.correlateSession(fresh, false)
	require.True(t, fresh.HasClaudeSession())
	assert.Equal(t, ownUUID, fresh.GetClaudeSession().ConversationUUID)

	// A session that previously had a conversation may still recover older files.
	veteran := &Instance{Title: "veteran", Path: repoPath, Status: Running, CreatedAt: created, EverHadConversationHistory: true}
	linker.SetInstances([]*Instance{veteran})
	linker.correlateSession(veteran, false)
	assert.Equal(t, oldUUID, veteran.GetClaudeSession().ConversationUUID)
}
